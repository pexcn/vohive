package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/cardpolicy"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

func TestCellularRegistrationBlockedDuringVoWiFiFlightAndSwitch(t *testing.T) {
	for _, mode := range []string{"desired", "flight", "starting", "active", "switching", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			p := newDesiredVoWiFiTestPool(t, "dev", false, "001010000000001")
			defer p.cancel()
			w := p.GetWorker("dev")
			switch mode {
			case "desired":
				applyPolicyToWorker(w, cardpolicy.Policy{VoWiFiEnabled: true})
			case "flight":
				applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
			case "starting":
				if !p.voWiFiHost().BeginStart("dev").Accepted {
					t.Fatal("start claim rejected")
				}
			case "active":
				p.voWiFiRuntimeStore().SetInstance("dev", &runtimehost.Instance{})
				t.Cleanup(func() { p.voWiFiRuntimeStore().DeleteInstance("dev", nil) })
			case "switching":
				p.switchMu.Lock()
				if p.switchingDevices == nil {
					p.switchingDevices = make(map[string]bool)
				}
				p.switchingDevices["dev"] = true
				p.switchMu.Unlock()
			case "stopped":
				close(w.stop)
			}
			if _, _, _, err := w.beginCellularRegistration(context.Background()); !errors.Is(err, errQMIRegistrationSkipped) {
				t.Fatalf("registration allowed: %v", err)
			}
			if w.startQMIRegistrationReconcile(context.Background(), "background", func(context.Context) error { t.Error("blocked registration ran"); return nil }) {
				t.Fatal("background registration queued")
			}
		})
	}
}

type cancelledQMIRead struct {
	*qmiRegistrationTestController
	entered, release chan struct{}
}

func (c *cancelledQMIRead) GetOperatingMode(context.Context) (backend.OperatingMode, error) {
	close(c.entered)
	<-c.release
	return backend.ModeRFOff, nil // Simulate a late backend reply ignoring cancellation.
}

type cancelledMBIMRead struct {
	*mbimRegistrationTestController
	entered, release chan struct{}
}

func (c *cancelledMBIMRead) GetOperatingMode(context.Context) (backend.OperatingMode, error) {
	close(c.entered)
	<-c.release
	return backend.ModeRFOff, nil
}

func TestCellularTakeoverCancelsAndDrainsLateRadioQueries(t *testing.T) {
	for _, kind := range []string{"qmi", "mbim"} {
		t.Run(kind, func(t *testing.T) {
			w := &Worker{ID: "dev"}
			ctx, cfg, finish, err := w.beginCellularRegistration(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			q := &cancelledQMIRead{qmiRegistrationTestController: &qmiRegistrationTestController{}, entered: entered, release: release}
			m := &cancelledMBIMRead{mbimRegistrationTestController: &mbimRegistrationTestController{}, entered: entered, release: release}
			result := make(chan error, 1)
			go func() {
				defer finish()
				if kind == "qmi" {
					result <- ensureQMIRegistration(ctx, w.ID, cfg, q, q, qmiRegistrationOptions{PollInterval: time.Millisecond})
				} else {
					result <- ensureMBIMRegistration(ctx, w.ID, cfg, m, mbimRegistrationOptions{PollInterval: time.Millisecond})
				}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("registration did not start")
			}
			applyPolicyToWorker(w, cardpolicy.Policy{VoWiFiEnabled: true})
			drained := make(chan error, 1)
			go func() { drained <- w.stopCellularRegistration(context.Background()) }()
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("registration was not cancelled")
			}
			select {
			case err := <-drained:
				t.Fatalf("takeover did not wait for backend reply: %v", err)
			default:
			}
			close(release)
			select {
			case err := <-drained:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("registration drain stuck")
			}
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("late query: %v", err)
			}
			if len(q.setModeCalls) != 0 || len(m.setModes) != 0 {
				t.Fatal("late query re-enabled radio")
			}
			applyPolicyToWorker(w, cardpolicy.Policy{})
			next, _, end, err := w.beginCellularRegistration(context.Background())
			if err != nil {
				t.Fatalf("normal registration did not resume: %v", err)
			}
			defer end()
			ctrl := &qmiRegistrationTestController{opMode: backend.ModeRFOff}
			if err = ensureQMIRegistration(next, w.ID, config.DeviceConfig{}, ctrl, ctrl, qmiRegistrationOptions{PollInterval: time.Millisecond}); err != nil {
				t.Fatal(err)
			}
			if len(ctrl.setModeCalls) != 1 || ctrl.setModeCalls[0] != backend.ModeOnline {
				t.Fatal("radio did not resume normal Online recovery")
			}
		})
	}
}

func TestCellularTakeoverTimeoutRetainsPendingRegistration(t *testing.T) {
	w := &Worker{ID: "dev"}
	_, _, finish, err := w.beginCellularRegistration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	applyPolicyToWorker(w, cardpolicy.Policy{VoWiFiEnabled: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = w.stopCellularRegistration(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("drain timeout: %v", err)
	}
	w.registrationMu.Lock()
	pending := len(w.registrationRuns)
	w.registrationMu.Unlock()
	if pending != 1 {
		t.Fatal("lost pending registration ownership")
	}
}

func TestNetworkPreferenceDoesNotConnectDuringVoWiFiStartup(t *testing.T) {
	p := newDesiredVoWiFiTestPool(t, "dev", false, "001010000000001")
	defer p.cancel()
	w := p.GetWorker("dev")
	controller := &fakeController{connected: true}
	w.netOverride = controller
	for _, enabled := range []bool{false, true} {
		applyPolicyToWorker(w, cardpolicy.Policy{VoWiFiEnabled: true, NetworkEnabled: enabled})
		if err := p.applyNetworkPreference(w); err != nil {
			t.Fatal(err)
		}
		if controller.connected {
			t.Fatal("cellular data connected under VoWiFi policy")
		}
		if err := w.StartNetwork(); !errors.Is(err, errQMIRegistrationSkipped) {
			t.Fatalf("manual data start: %v", err)
		}
	}
	applyPolicyToWorker(w, cardpolicy.Policy{NetworkEnabled: true})
	if err := w.StartNetwork(); err != nil {
		t.Fatal(err)
	}
	if !controller.connected {
		t.Fatal("data preference did not resume after VoWiFi disabled")
	}
}

func TestCellularSwitchSnapshotRestoreDoesNotUnblockBackgroundWork(t *testing.T) {
	p := newDesiredVoWiFiTestPool(t, "dev", true, "001010000000001")
	defer p.cancel()
	w := p.GetWorker("dev")
	controller := &fakeController{}
	w.netOverride = controller
	p.switchMu.Lock()
	p.switchingDevices = map[string]bool{"dev": true}
	p.switchMu.Unlock()
	if err := w.StartNetwork(); !errors.Is(err, errQMIRegistrationSkipped) {
		t.Fatalf("ordinary network request bypassed switching gate: %v", err)
	}
	w.Config.AutoConnectData = true
	snapshot := esimSwitchContext{NetworkEnabledBefore: true}
	if err := p.applyNetworkPreferenceForSwitchSnapshot(w, snapshot); err != nil {
		t.Fatalf("coordinator could not restore its online/data snapshot: %v", err)
	}
	if !controller.connected || !w.cellularRegistrationBlocked() || !p.IsESIMSwitching("dev") {
		t.Fatal("snapshot restoration changed the background registration guard")
	}
	controller.connected = false
	p.voWiFiRuntimeStore().SetInstance("dev", &runtimehost.Instance{})
	t.Cleanup(func() { p.voWiFiRuntimeStore().DeleteInstance("dev", nil) })
	if err := p.applyNetworkPreferenceForSwitchSnapshot(w, snapshot); !errors.Is(err, errQMIRegistrationSkipped) {
		t.Fatalf("snapshot restoration bypassed an active VoWiFi session: %v", err)
	}
	if controller.connected {
		t.Fatal("snapshot restoration connected data during active VoWiFi")
	}
}

type lateOnlineBackend struct {
	*workerStatusBackendStub
	entered, release chan struct{}
}

func (b *lateOnlineBackend) SetOperatingMode(ctx context.Context, mode backend.OperatingMode) error {
	if mode == backend.ModeOnline {
		close(b.entered)
		<-b.release // A command already sent to hardware may finish after cancellation.
	}
	b.opMode = mode
	return b.workerStatusBackendStub.SetOperatingMode(ctx, mode)
}

func TestCellularTakeoverAppliesRFOffAfterLateOnlineWrite(t *testing.T) {
	p := &Pool{ctx: context.Background()}
	b := &lateOnlineBackend{
		workerStatusBackendStub: &workerStatusBackendStub{opMode: backend.ModeRFOff},
		entered:                 make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(func() {
		select {
		case <-b.release:
		default:
			close(b.release)
		}
	})
	w := &Worker{ID: "dev", Backend: b}
	ctx, _, finish, err := w.beginCellularRegistration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer finish()
		_ = b.SetOperatingMode(ctx, backend.ModeOnline)
	}()
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("Online command did not start")
	}
	applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
	applied := make(chan struct{})
	go func() {
		p.enterAirplaneModeFromPolicy(w, "test")
		close(applied)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("registration was not cancelled")
	}
	select {
	case <-applied:
		t.Fatal("RFOff did not wait for in-flight Online command")
	default:
	}
	close(b.release)
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("RFOff takeover stuck")
	}
	if b.opMode != backend.ModeRFOff || len(b.setOpModeCalls) != 2 || b.setOpModeCalls[0] != backend.ModeOnline || b.setOpModeCalls[1] != backend.ModeRFOff {
		t.Fatalf("late command undid RFOff: %v", b.setOpModeCalls)
	}
}
