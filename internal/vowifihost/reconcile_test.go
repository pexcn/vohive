package vowifihost

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
)

type switchingRecoveryAdapter struct {
	Adapter
	switching atomic.Bool
}

func (a *switchingRecoveryAdapter) IsSwitching(string) bool { return a.switching.Load() }

func TestManagerDesiredRecoverableIsFalseWhenRuntimeActiveOrStarting(t *testing.T) {
	manager := NewManager()
	manager.RuntimeStore().SetInstance("active", &runtimehost.Instance{})
	manager.RuntimeStore().BeginStart("starting")

	if manager.DesiredRecoverable("active") {
		t.Fatal("DesiredRecoverable() = true for active runtime, want false")
	}
	if manager.DesiredRecoverable("starting") {
		t.Fatal("DesiredRecoverable() = true for starting runtime, want false")
	}
	if !manager.DesiredRecoverable("idle") {
		t.Fatal("DesiredRecoverable() = false for idle device, want true")
	}
}

func TestManagerScheduleDesiredRecoverRunsRecoverAndCallback(t *testing.T) {
	manager := NewManager()
	commands := make(chan LifecycleCommand, 1)
	callbacks := make(chan error, 1)
	manager.SetLifecycleRunForTest(func(ctx context.Context, cmd LifecycleCommand) error {
		commands <- cmd
		return nil
	})

	if !manager.ScheduleDesiredRecover(context.Background(), DesiredRecoverRequest{
		DeviceID: "dev-1",
		Reason:   "desired_reconcile",
		Now:      time.Now(),
		OnResult: func(deviceID, reason string, err error) {
			if deviceID != "dev-1" {
				t.Errorf("callback deviceID = %q, want dev-1", deviceID)
			}
			if reason != "desired_reconcile" {
				t.Errorf("callback reason = %q, want desired_reconcile", reason)
			}
			callbacks <- err
		},
	}) {
		t.Fatal("ScheduleDesiredRecover() = false, want true")
	}

	select {
	case cmd := <-commands:
		if cmd.Kind != LifecycleCommandRecover {
			t.Fatalf("command kind = %s, want recover", cmd.Kind.String())
		}
		if cmd.DeviceID != "dev-1" {
			t.Fatalf("command deviceID = %q, want dev-1", cmd.DeviceID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recover command")
	}

	select {
	case err := <-callbacks:
		if err != nil {
			t.Fatalf("callback err = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recover callback")
	}
}

func TestManagerScheduleDesiredRecoverSkipsRuntimeActivity(t *testing.T) {
	manager := NewManager()
	manager.RuntimeStore().SetInstance("dev-1", &runtimehost.Instance{})
	manager.SetLifecycleRunForTest(func(ctx context.Context, cmd LifecycleCommand) error {
		t.Fatalf("recover should not run for active runtime: %+v", cmd)
		return nil
	})

	if manager.ScheduleDesiredRecover(context.Background(), DesiredRecoverRequest{
		DeviceID: "dev-1",
		Now:      time.Now(),
		OnResult: func(deviceID, reason string, err error) {
			t.Fatalf("callback should not run for skipped recover")
		},
	}) {
		t.Fatal("ScheduleDesiredRecover() = true for active runtime, want false")
	}
}

func TestQueuedRecoverSkipsSwitchBeforeInvalidatingRuntime(t *testing.T) {
	m := NewManager()
	a := &switchingRecoveryAdapter{}
	m.ConfigureAdapter(a)
	deviceID := "dev-1"
	state := runtimehost.State{Phase: runtimehost.PhaseSIMReady, SIMReady: true}
	m.RecordStartupState(deviceID, state)
	epoch := m.CurrentEpoch(deviceID)
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	m.ConfigureLifecycle(LifecycleControllerOptions{Run: func(ctx context.Context, cmd LifecycleCommand) error {
		close(entered)
		<-release
		return m.runLifecycleCommand(ctx, cmd)
	}})
	result := make(chan error, 1)
	go func() { result <- m.Recover(context.Background(), LifecycleRecoverRequest{DeviceID: deviceID}) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("recover command did not enter the controller")
	}
	a.switching.Store(true)
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("switch-owned recovery was reported as a startup failure: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("switch-owned recovery did not exit")
	}
	if m.CurrentEpoch(deviceID) != epoch {
		t.Fatal("queued recover invalidated the runtime during switching")
	}
	if got, ok := m.State(deviceID); !ok || got.Phase != state.Phase || !got.SIMReady {
		t.Fatalf("queued recover cleared startup state during switching: %+v", got)
	}
}
