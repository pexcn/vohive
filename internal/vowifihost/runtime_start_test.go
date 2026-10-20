package vowifihost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
)

func TestManagerStartRuntimePublishesProgressBeforeReturn(t *testing.T) {
	m := NewManager()
	claim := m.BeginStart("dev-1")
	changes, unsub := m.SubscribeState("dev-1")
	defer unsub()
	m.SetRuntimeStartForTest(func(ctx context.Context, req runtimehost.StartRequest) (*runtimehost.Instance, error) {
		if req.Observer == nil {
			t.Fatal("missing observer during startup")
		}
		for _, state := range []runtimehost.State{
			{Phase: runtimehost.PhaseTunnelConnecting, SIMReady: true, AccessReady: true},
			{Phase: runtimehost.PhaseIMSRegistering, SIMReady: true, AccessReady: true, TunnelReady: true},
			{Phase: runtimehost.PhaseIMSReady, SIMReady: true, AccessReady: true, TunnelReady: true, IMSReady: true},
		} {
			state.DeviceID, state.UpdatedAt = req.DeviceID, time.Now()
			req.Observer.OnRuntimeEvent(ctx, runtimehost.Event{State: state})
			select {
			case <-changes:
			default:
				t.Fatalf("%s did not notify SSE subscribers", state.Phase)
			}
			if got, ok := m.State(req.DeviceID); !ok || got != state {
				t.Fatalf("progress unavailable before Start returns: %+v", got)
			}
			if m.Active(req.DeviceID) || !m.Starting(req.DeviceID) {
				t.Fatal("progress incorrectly claimed a running instance")
			}
		}
		return &runtimehost.Instance{}, nil
	})
	_, err := m.StartRuntime(context.Background(), RuntimeStartRequest{DeviceID: "dev-1", Epoch: claim.Epoch})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManagerStartupObserverCannotOverwriteNewAttempt(t *testing.T) {
	m := NewManager()
	claim := m.BeginStart("dev-1")
	var old runtimehost.Observer
	wantErr := errors.New("startup failed")
	m.SetRuntimeStartForTest(func(_ context.Context, req runtimehost.StartRequest) (*runtimehost.Instance, error) {
		old = req.Observer
		return nil, wantErr
	})
	if _, err := m.StartRuntime(context.Background(), RuntimeStartRequest{DeviceID: "dev-1", Epoch: claim.Epoch}); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	m.FailStart("dev-1", claim.Epoch, runtimehost.State{}, wantErr)
	// A retry need not go through explicit invalidation. Its startup events must
	// still be isolated from an earlier attempt's delayed notification queue.
	newClaim := m.BeginStart("dev-1")
	newState := runtimehost.State{Phase: runtimehost.PhaseSIMReady, LastReason: "new_attempt", UpdatedAt: time.Now()}
	m.RecordStartupState("dev-1", newState)
	changes, unsub := m.SubscribeState("dev-1")
	defer unsub()
	old.OnRuntimeEvent(context.Background(), runtimehost.Event{State: runtimehost.State{
		Phase: runtimehost.PhaseReady, SMSReady: true, UpdatedAt: time.Now().Add(time.Second),
	}})
	if state, ok := m.State("dev-1"); !ok || state != newState {
		t.Fatalf("late startup event overwrote new attempt: %+v", state)
	}
	select {
	case <-changes:
		t.Fatal("late startup event notified subscribers")
	default:
	}
	m.InvalidateRuntime("dev-1", "switch")
	if m.RuntimeStore().RecordStartupStateForEpoch("dev-1", newClaim.Epoch, newState) {
		t.Fatal("invalidated attempt published startup progress")
	}
}

type runtimeStartTestModem struct{}

func (runtimeStartTestModem) DeviceID() string { return "dev-1" }
func (runtimeStartTestModem) IsHealthy() bool  { return true }
func (runtimeStartTestModem) IsSimInserted() bool {
	return true
}
func (runtimeStartTestModem) QuerySIMInserted() (bool, error) { return true, nil }
func (runtimeStartTestModem) GetRegStatus() (int, string)     { return 1, "registered" }
func (runtimeStartTestModem) GetNetworkMode() string          { return "LTE" }
func (runtimeStartTestModem) ExecuteATSilent(string, time.Duration) (string, error) {
	return "", nil
}
func (runtimeStartTestModem) OpenLogicalChannel(string) (int, error) { return 0, nil }
func (runtimeStartTestModem) CloseLogicalChannel(int) error          { return nil }
func (runtimeStartTestModem) TransmitAPDU(int, string) (string, error) {
	return "", nil
}
func (runtimeStartTestModem) Stop() {}

func TestManagerStartRuntimeBuildsRequestAndClaimsInstance(t *testing.T) {
	manager := NewManager()
	deviceID := "dev-1"
	claim := manager.BeginStart(deviceID)
	if !claim.Accepted {
		t.Fatalf("BeginStart() = %+v, want accepted", claim)
	}
	wantInst := &runtimehost.Instance{}
	var captured runtimehost.StartRequest
	manager.SetRuntimeStartForTest(func(ctx context.Context, req runtimehost.StartRequest) (*runtimehost.Instance, error) {
		captured = req
		if !req.ShouldRun() {
			t.Fatal("StartRequest.ShouldRun() = false before invalidation, want true")
		}
		return wantInst, nil
	})

	result, err := manager.StartRuntime(context.Background(), RuntimeStartRequest{
		DeviceID: deviceID,
		TraceID:  "trace-1",
		Epoch:    claim.Epoch,
		Prepared: PreparedStart{
			Profile: identity.Profile{IMSI: "001010000000001"},
			Prepared: identity.PreparedSession{
				Profile: identity.Profile{IMSI: "001010000000001"},
			},
			NetworkMode: "LTE",
		},
		Modem:     runtimeStartTestModem{},
		Dataplane: runtimehost.DataplanePolicy{Mode: "userspace"},
	})
	if err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if result.Instance != wantInst || result.Stale {
		t.Fatalf("StartRuntime() = %+v, want claimed instance", result)
	}
	if manager.Instance(deviceID) != wantInst {
		t.Fatal("StartRuntime() should claim instance in runtime store")
	}
	if captured.Mode != runtimehost.StartModeMain || captured.DeviceID != deviceID || captured.TraceID != "trace-1" {
		t.Fatalf("captured request identity = mode %q device %q trace %q", captured.Mode, captured.DeviceID, captured.TraceID)
	}
	if captured.AuthenticationContext == nil {
		t.Fatal("missing subscription authentication context")
	}
	if captured.SIM == nil || captured.Access == nil {
		t.Fatal("captured request should include SIM and Access adapters")
	}
	if captured.NetworkMode != "LTE" || captured.Dataplane.Mode != "userspace" {
		t.Fatalf("captured request network/dataplane = %q/%q", captured.NetworkMode, captured.Dataplane.Mode)
	}
}

func TestManagerStartRuntimeStopsStaleStartedInstance(t *testing.T) {
	manager := NewManager()
	deviceID := "dev-stale"
	claim := manager.BeginStart(deviceID)
	manager.InvalidateRuntime(deviceID, "test")
	newState := runtimehost.State{Phase: runtimehost.PhaseSIMReady, LastReason: "new_card_preparing", UpdatedAt: time.Now()}
	manager.RecordStartupState(deviceID, newState)
	manager.SetRuntimeStartForTest(func(ctx context.Context, req runtimehost.StartRequest) (*runtimehost.Instance, error) {
		if req.ShouldRun() {
			t.Fatal("StartRequest.ShouldRun() = true after invalidation, want false")
		}
		return &runtimehost.Instance{}, nil
	})

	result, err := manager.StartRuntime(context.Background(), RuntimeStartRequest{
		DeviceID: deviceID,
		TraceID:  "trace-stale",
		Epoch:    claim.Epoch,
		Prepared: PreparedStart{
			Profile:     identity.Profile{IMSI: "001010000000001"},
			Prepared:    identity.PreparedSession{Profile: identity.Profile{IMSI: "001010000000001"}},
			NetworkMode: "LTE",
		},
		Modem: runtimeStartTestModem{},
	})
	if err != nil {
		t.Fatalf("StartRuntime() error = %v", err)
	}
	if !result.Stale {
		t.Fatalf("StartRuntime() stale = false, want true")
	}
	if manager.Active(deviceID) {
		t.Fatal("stale started instance should not become active")
	}
	if got, ok := manager.State(deviceID); !ok || got.LastReason != newState.LastReason {
		t.Fatalf("stale startup cleared the new card's state: %+v", got)
	}
}

func TestManagerLateStoppedEventCannotResurrectPreviousCardState(t *testing.T) {
	m := NewManager()
	deviceID := "dev-switch-state"
	inst := &runtimehost.Instance{}
	blocked, release, delivered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	inst.AddObserver(runtimehost.ObserverFunc(func(_ context.Context, ev runtimehost.Event) {
		if ev.State.Phase == runtimehost.PhaseStopped {
			close(blocked)
			<-release
		}
	}))
	m.SetRuntimeStartForTest(func(context.Context, runtimehost.StartRequest) (*runtimehost.Instance, error) { return inst, nil })
	claim := m.BeginStart(deviceID)
	if _, err := m.StartRuntime(context.Background(), RuntimeStartRequest{
		DeviceID: deviceID,
		Epoch:    claim.Epoch,
		Prepared: PreparedStart{Prepared: identity.PreparedSession{Profile: identity.Profile{IMSI: "001010000000001"}}},
	}); err != nil {
		t.Fatal(err)
	}
	inst.AddObserver(runtimehost.ObserverFunc(func(_ context.Context, ev runtimehost.Event) {
		if ev.State.Phase == runtimehost.PhaseStopped {
			close(delivered)
		}
	}))
	changes, unsub := m.SubscribeState(deviceID)
	defer unsub()
	if !m.TeardownForSwitch(context.Background(), deviceID) {
		t.Fatal("old card runtime was not torn down")
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("teardown did not notify the overview stream")
	}
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("stopped notification was not queued")
	}
	close(release)
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("stopped notification did not drain")
	}
	if state, ok := m.State(deviceID); ok {
		t.Fatalf("old stopped event resurrected previous card state: %+v", state)
	}
	select {
	case <-changes:
		t.Fatal("old instance published an overview update after removal")
	default:
	}
}
