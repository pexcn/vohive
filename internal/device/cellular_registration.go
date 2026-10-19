package device

import (
	"context"

	"github.com/iniwex5/vohive/internal/config"
)

// Registration tasks own only their cancellation context, never the shared modem.
type cellularRegistrationRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// Only the switch coordinator may restore an explicitly captured online/data
// snapshot while its switching flag is still held. Background policy work must
// continue to wait for finalization.
type switchSnapshotRegistrationKey struct{}

func (w *Worker) cellularRegistrationBlocked() bool {
	if w == nil {
		return true
	}
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	return w.cellularRegistrationBlockedLocked(false)
}

func (w *Worker) cellularRegistrationBlockedLocked(restoreSwitchSnapshot bool) bool {
	if !restoreSwitchSnapshot && (w.Config.VoWiFiEnabled || w.Config.AirplaneEnabled) {
		return true
	}
	if w.stop != nil {
		select {
		case <-w.stop:
			return true
		default:
		}
	}
	if w.Pool == nil {
		return false
	}
	if !restoreSwitchSnapshot && w.Pool.IsESIMSwitching(w.ID) {
		return true
	}
	host := w.Pool.voWiFiHost()
	return host.Active(w.ID) || host.Starting(w.ID)
}

func (w *Worker) beginCellularRegistration(ctx context.Context) (context.Context, config.DeviceConfig, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, config.DeviceConfig{}, nil, err
	}
	restoreSwitchSnapshot, _ := ctx.Value(switchSnapshotRegistrationKey{}).(bool)
	if w.cellularRegistrationBlockedLocked(restoreSwitchSnapshot) {
		return nil, config.DeviceConfig{}, nil, errQMIRegistrationSkipped
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.registrationNextID++
	id := w.registrationNextID
	done := make(chan struct{})
	if w.registrationRuns == nil {
		w.registrationRuns = make(map[uint64]cellularRegistrationRun)
	}
	w.registrationRuns[id] = cellularRegistrationRun{cancel: cancel, done: done}
	if w.stop != nil {
		go func() {
			select {
			case <-w.stop:
				cancel()
			case <-runCtx.Done():
			}
		}()
	}
	finish := func() {
		cancel()
		w.registrationMu.Lock()
		delete(w.registrationRuns, id)
		close(done)
		w.registrationMu.Unlock()
	}
	return runCtx, w.Config, finish, nil
}

func (w *Worker) cancelCellularRegistration() []<-chan struct{} {
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	done := make([]<-chan struct{}, 0, len(w.registrationRuns))
	for _, run := range w.registrationRuns {
		run.cancel()
		done = append(done, run.done)
	}
	return done
}

// The caller must first establish flight/VoWiFi/switch ownership. Those guards
// exclude new registrations while old controller calls drain. Apply RFOff only
// after this returns so a late Online reply cannot undo it.
func (w *Worker) stopCellularRegistration(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, qmiRegistrationTimeoutBestEffort)
	defer cancel()
	for _, done := range w.cancelCellularRegistration() {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}
