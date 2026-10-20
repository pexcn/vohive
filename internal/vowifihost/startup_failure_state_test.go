package vowifihost

import (
	"context"
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/carrier"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
)

type startupFailureAdapter struct {
	Adapter
	prepared PreparedStart
	failure  StartupErrorRequest
}

func (a *startupFailureAdapter) Context() context.Context { return context.Background() }
func (a *startupFailureAdapter) IsSwitching(string) bool  { return false }
func (a *startupFailureAdapter) PrepareStart(string, string, string) (PreparedStart, error) {
	return a.prepared, nil
}
func (a *startupFailureAdapter) BeforeStart(string, runtimehost.Modem, *runtimehost.ProxyConfig) func(context.Context, runtimehost.SessionConfig) error {
	return nil
}
func (a *startupFailureAdapter) HandleStartupError(req StartupErrorRequest) error {
	a.failure = req
	return req.Err
}

func TestStartupFailureRetainsRuntimeErrorClassAfterCleanup(t *testing.T) {
	m := NewManager()
	c := carrier.ResolveEffectiveCarrierConfig(carrier.EffectiveCarrierConfigInput{MCC: "530", MNC: "24"})
	a := &startupFailureAdapter{prepared: PreparedStart{
		Prepared: identity.PreparedSession{
			Profile:          identity.Profile{IMSI: "530240000000001", MCC: "530", MNC: "24"},
			EPDGAddr:         c.EPDGAddr,
			EffectiveCarrier: c,
		},
		Modem:        runtimeStartTestModem{},
		StartupState: runtimehost.State{Phase: runtimehost.PhaseSIMReady, SIMReady: true},
	}}
	m.ConfigureAdapter(a)
	if err := m.Enable(context.Background(), "dev-1"); err == nil || !strings.Contains(err.Error(), "no supported IKE/ESP pair") {
		t.Fatalf("unexpected startup outcome: %v", err)
	}
	if a.failure.State.LastErrorClass != "unsupported_carrier_proposals" {
		t.Fatalf("runtime error class lost during cleanup: %+v", a.failure.State)
	}
	if m.Active("dev-1") || m.Starting("dev-1") || a.failure.State.TunnelReady || a.failure.State.SMSReady {
		t.Fatal("failed runtime became ready or retained a startup claim")
	}
}
