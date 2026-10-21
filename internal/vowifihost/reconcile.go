package vowifihost

import (
	"context"
	"strings"
	"time"

	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

const defaultDesiredRecoverReason = "desired_reconcile"

type DesiredRecoverRequest struct {
	DeviceID     string
	Reason       string
	OverrideEPDG string
	Generation   uint64
	Now          time.Time
	OnResult     func(deviceID, reason string, err error)
}

func (m *Manager) DesiredRecoverable(deviceID string) bool {
	if m == nil {
		return false
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}
	// SwitchEnd owns restoration until the switch coordinator finalizes.
	// Policy projection may happen earlier and must not queue a second owner.
	if adapter := m.hostAdapter(); adapter != nil && adapter.IsSwitching(deviceID) {
		return false
	}
	store := m.RuntimeStore()
	if store.Starting(deviceID) {
		return false
	}
	if inst := store.Instance(deviceID); inst != nil {
		// An IMS registration recovery still owns a live tunnel. Only terminal
		// instances may be replaced, after serialized lifecycle cleanup.
		phase := inst.State().Phase
		return phase == runtimehost.PhaseFailed || phase == runtimehost.PhaseStopped
	}
	return true
}

func (m *Manager) ScheduleDesiredRecover(ctx context.Context, req DesiredRecoverRequest) bool {
	if m == nil {
		return false
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		return false
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = defaultDesiredRecoverReason
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	if !m.DesiredRecoverable(deviceID) {
		return false
	}
	if !m.BeginDesiredRecover(deviceID, now) {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Info("VoWiFi 目标态恢复开始", "event", "VOWIFI_DESIRED_RECOVER", "device", deviceID, "reason", reason)
	go func() {
		err := m.Recover(ctx, LifecycleRecoverRequest{
			DeviceID:     deviceID,
			Reason:       reason,
			OverrideEPDG: req.OverrideEPDG,
			Generation:   req.Generation,
		})
		if req.OnResult != nil {
			req.OnResult(deviceID, reason, err)
		}
	}()
	return true
}
