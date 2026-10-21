package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/cardpolicy"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/vowifihost"
)

func TestRadioPolicyRejectsUnknownProtectedAndStaleCards(t *testing.T) {
	for _, state := range []string{"startup", "stale", "switch_snapshot"} {
		t.Run(state, func(t *testing.T) {
			w := &Worker{}
			w.Config.NetworkEnabled = true
			ctx := t.Context()
			switch state {
			case "startup":
				w.radioPolicyPending = true
			case "stale":
				w.networkPolicyICCID = "old"
				w.state.Identity.ICCID = "new"
			case "switch_snapshot":
				w.Config.AirplaneEnabled = true
				ctx = context.WithValue(ctx, switchSnapshotRegistrationKey{}, true)
			}
			if release, err := w.BeginRadioOnline(ctx); err == nil {
				release()
				t.Fatal("RF activation permitted")
			}
			if w.autoConnectDataAllowed() {
				t.Fatal("automatic data activation permitted")
			}
			if state != "stale" {
				if _, _, finish, err := w.beginCellularRegistration(ctx); err == nil {
					finish()
					t.Fatal("registration permitted")
				}
			}
		})
	}
}

func TestRadioPolicyOnlyConfirmedCurrentCardReleasesStartupHold(t *testing.T) {
	w := &Worker{radioPolicyPending: true}
	w.state.Identity.ICCID = "new"
	if applyPolicyToWorker(w, cardpolicy.Policy{ICCID: "old"}) {
		t.Fatal("stale resolver result accepted")
	}
	if !w.radioPolicyPending {
		t.Fatal("stale policy released hold")
	}
	if !applyPolicyToWorker(w, cardpolicy.Policy{ICCID: "new"}) {
		t.Fatal("current policy rejected")
	}
	if applyPolicyToWorker(w, cardpolicy.Policy{ICCID: "old", AirplaneEnabled: true}) {
		t.Fatal("late policy for the old profile replaced the confirmed policy")
	}
	release, err := w.BeginRadioOnline(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release()
	w.BeginSIMIdentityTransition("next", "test")
	if release, err = w.BeginRadioOnline(t.Context()); err == nil {
		release()
		t.Fatal("identity transition retained old RF permission")
	}
}

func TestRadioPolicyRequiresRFOffReadback(t *testing.T) {
	for _, failure := range []string{"write", "read", "ignored", "already_off", "success"} {
		t.Run(failure, func(t *testing.T) {
			b := &workerStatusBackendStub{opMode: backend.ModeOnline}
			switch failure {
			case "write":
				b.setOpModeErr = errors.New("control failed")
			case "read":
				b.opModeErr = errors.New("read failed")
			case "ignored":
				b.ignoreOpModeWrite = true
			case "already_off":
				b.opMode = backend.ModeRFOff
			}
			w := &Worker{Backend: b}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			err := w.ensureRadioOff(ctx)
			wantError := failure == "write" || failure == "read" || failure == "ignored"
			if (err != nil) != wantError {
				t.Fatalf("RF-off verification: %v", err)
			}
			if failure == "already_off" && len(b.setOpModeCalls) != 0 {
				t.Fatal("redundant RF off could invalidate SIM auth channels")
			}
		})
	}
}

func TestRadioPolicySwitchAndStartupAbortOnRFOffFailure(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	b := &workerStatusBackendStub{setOpModeErr: errors.New("RF command rejected"), opMode: backend.ModeOnline}
	p.workers["dev"] = &Worker{ID: "dev", Backend: b, Pool: p}
	p.voWiFiHost().LifecycleControllerForTest().TestRun = func(context.Context, vowifihost.LifecycleCommand) error { return nil }
	if _, err := p.handleESIMSwitchBefore("dev", "new"); !errors.Is(err, b.setOpModeErr) {
		t.Fatal("profile switch preparation succeeded with RF on")
	}
	if _, err := p.prepareVoWiFiStartContext("dev", "trace", ""); !errors.Is(err, b.setOpModeErr) {
		t.Fatal("VoWiFi preparation succeeded with RF on")
	}
}

func TestRadioPolicyVoWiFiFailureNeverRestoresCellular(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	b := &workerStatusBackendStub{opMode: backend.ModeRFOff}
	ctrl := &fakeController{}
	w := &Worker{ID: "dev", Backend: b, netOverride: ctrl, restoreNetworkAfterVoWiFi: true,
		Config: config.DeviceConfig{VoWiFiEnabled: true, NetworkEnabled: true}}
	p.restoreNetworkAfterVoWiFiStartupFailure("trace", "dev", w)
	if len(b.setOpModeCalls) != 0 || ctrl.connected || w.restoreNetworkAfterVoWiFi {
		t.Fatal("failed VoWiFi reopened cellular fallback")
	}
}

func TestRadioPolicyChangeDrainsLateOnlineWithoutBlockingCancellation(t *testing.T) {
	b := &workerStatusBackendStub{opMode: backend.ModeOnline}
	w := &Worker{Backend: b}
	release, err := w.BeginRadioOnline(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an already-issued controller operation, ignoring cancellation.
	applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
	done := make(chan error, 1)
	go func() { done <- w.EnterAirplaneMode(t.Context()) }()
	select {
	case err := <-done:
		release()
		t.Fatalf("flight mode finished ahead of late Online: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil || b.opMode != backend.ModeRFOff {
			t.Fatalf("late Online undid flight mode: %v %v", err, b.opMode)
		}
	case <-time.After(time.Second):
		t.Fatal("RF operation did not drain")
	}
	if unlock, err := w.BeginRadioOnline(t.Context()); err == nil {
		unlock()
		t.Fatal("policy change permitted another Online operation")
	}
}

func TestRadioPolicyStillBlocksAutomaticRebootAfterPolicyChange(t *testing.T) {
	for _, protect := range []bool{false, true} {
		b := &workerStatusBackendStub{}
		w := &Worker{ID: "dev", Backend: b, consecutiveFailures: 4}
		c := &fakeController{connected: true, rotateHook: func() error {
			if protect {
				applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
			}
			return errors.New("rotation failed")
		}}
		w.netOverride = c
		_, _, _ = w.RotateWithNotify()
		if protect && b.rebootCalls != 0 || !protect && b.rebootCalls != 1 {
			t.Fatalf("protected=%v, automatic reboots=%d", protect, b.rebootCalls)
		}
	}
}
