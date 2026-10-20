package vowifihost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
)

type terminalRecoveryAdapter struct {
	startupFailureAdapter
	restored, started int
	switching         bool
}

func (a *terminalRecoveryAdapter) IsSwitching(string) bool                      { return a.switching }
func (a *terminalRecoveryAdapter) WaitQMICoreReady(string, time.Duration) error { return nil }
func (a *terminalRecoveryAdapter) WaitWorkerReady(string, time.Duration) error  { return nil }
func (a *terminalRecoveryAdapter) RestoreSMSMode(string)                        { a.restored++ }
func (a *terminalRecoveryAdapter) MarkRuntimeStarted(RuntimeStartedRequest)     { a.started++ }

func terminalRecoveryManager(t *testing.T) (*Manager, *terminalRecoveryAdapter, *runtimehost.Instance) {
	t.Helper()
	m := NewManager()
	a := &terminalRecoveryAdapter{}
	a.prepared.Modem = runtimeStartTestModem{}
	m.ConfigureAdapter(a)
	inst := &runtimehost.Instance{}
	claim := m.BeginStart("dev-1")
	if !m.ClaimStarted("dev-1", claim.Epoch, inst) {
		t.Fatal("could not claim original runtime")
	}
	if err := inst.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m, a, inst
}

func TestDesiredRecoveryReplacesStoppedRuntime(t *testing.T) {
	m, a, old := terminalRecoveryManager(t)
	replacement := &runtimehost.Instance{}
	m.SetRuntimeStartForTest(func(_ context.Context, req runtimehost.StartRequest) (*runtimehost.Instance, error) {
		if m.RuntimeStore().Instance("dev-1") != nil || !req.ShouldRun() {
			t.Error("replacement started before old ownership was released")
		}
		return replacement, nil
	})
	result := make(chan error, 1)
	if !m.ScheduleDesiredRecover(context.Background(), DesiredRecoverRequest{
		DeviceID: "dev-1",
		OnResult: func(_, _ string, err error) { result <- err },
	}) {
		t.Fatal("stopped runtime blocked automatic recovery")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("automatic recovery did not finish")
	}
	if m.RuntimeStore().Instance("dev-1") != replacement || a.started != 1 || a.restored != 1 {
		t.Fatal("automatic recovery did not replace the old runtime exactly once")
	}
	// A delayed terminal event must not detach the replacement.
	if m.RuntimeStore().DeleteInstance("dev-1", old) || m.DesiredRecoverable("dev-1") {
		t.Fatal("old runtime displaced a live replacement")
	}
}

func TestTerminalRecoveryWaitsForRetainedCleanup(t *testing.T) {
	m, a, old := terminalRecoveryManager(t)
	pending := &switchCleanupFailure{fail: true}
	m.retainCleanup("dev-1", pending)
	starts := 0
	m.SetRuntimeStartForTest(func(context.Context, runtimehost.StartRequest) (*runtimehost.Instance, error) {
		starts++
		return &runtimehost.Instance{}, nil
	})
	if err := m.Recover(context.Background(), LifecycleRecoverRequest{DeviceID: "dev-1"}); err == nil {
		t.Fatal("recovery ignored unfinished resource cleanup")
	}
	if starts != 0 || a.started != 0 || m.RuntimeStore().Instance("dev-1") != old {
		t.Fatal("recovery released ownership or started before cleanup completed")
	}
	pending.fail = false
	if err := m.Recover(context.Background(), LifecycleRecoverRequest{DeviceID: "dev-1"}); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || a.started != 1 {
		t.Fatal("recovery did not resume after cleanup succeeded")
	}
}

func TestTerminalRecoveryRespectsSwitchOwnership(t *testing.T) {
	m, a, old := terminalRecoveryManager(t)
	a.switching = true
	epoch := m.CurrentEpoch("dev-1")
	m.SetRuntimeStartForTest(func(context.Context, runtimehost.StartRequest) (*runtimehost.Instance, error) {
		return nil, errors.New("switch-owned runtime must not start")
	})
	if m.DesiredRecoverable("dev-1") {
		t.Fatal("terminal recovery competed with eSIM switching")
	}
	if err := m.Recover(context.Background(), LifecycleRecoverRequest{DeviceID: "dev-1"}); err != nil {
		t.Fatal(err)
	}
	if m.CurrentEpoch("dev-1") != epoch || m.RuntimeStore().Instance("dev-1") != old || a.restored != 0 {
		t.Fatal("skipped recovery invalidated switch-owned resources")
	}
}
