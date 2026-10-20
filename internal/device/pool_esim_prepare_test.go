package device

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/esim"
	"github.com/iniwex5/vohive/internal/vowifihost"
)

func TestESIMPreparationFailureRestoresOldIdentityWithoutReleasingRadio(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	be := &esimSwitchRestoreBackendStub{mode: backend.BackendQMI, getMode: backend.ModeOnline}
	w := &Worker{ID: "dev", Backend: be, Config: config.DeviceConfig{
		ID: "dev", ESIMSwitch: config.ESIMSwitchConfig{RadioCycle: true},
	}}
	w.state.Identity.ICCID = "old-iccid"
	w.state.Identity.IMSI = "old-imsi"
	w.state.Identity.Ready = true
	p.workers[w.ID] = w
	rootErr := errors.New("old session cleanup failed")
	p.voWiFiHost().LifecycleControllerForTest().TestRun = func(context.Context, vowifihost.LifecycleCommand) error {
		return rootErr
	}
	before, _, failed, _, _ := p.newESIMSwitchCallbacks(w.ID)
	token, err := before(esim.SwitchOperationEnableProfile, "new-iccid")
	if token == 0 || !errors.Is(err, rootErr) {
		t.Fatalf("preparation lost token or cause: token=%d err=%v", token, err)
	}
	if len(be.setCalls) != 0 {
		t.Fatalf("radio released despite failed cleanup: %v", be.setCalls)
	}
	failed(token, errors.Join(esim.ErrProfileCommandNotSent, err))
	if p.IsESIMSwitching(w.ID) {
		t.Fatal("failed preparation left switching flag set")
	}
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()
	identity := w.state.Identity
	if identity.ICCID != "old-iccid" || identity.IMSI != "old-imsi" || !identity.Ready || identity.TargetICCID != "" {
		t.Fatalf("old identity was not restored: %+v", identity)
	}
}

func TestESIMFailureAfterWriteDoesNotRestoreOldIdentity(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	w := &Worker{ID: "dev", Config: config.DeviceConfig{ID: "dev"}}
	w.state.Identity.ICCID = "old-iccid"
	w.state.Identity.IMSI = "old-imsi"
	w.state.Identity.Ready = true
	p.workers[w.ID] = w
	before, _, failed, _, _ := p.newESIMSwitchCallbacks(w.ID)
	token, err := before(esim.SwitchOperationEnableProfile, "new-iccid")
	if err != nil {
		t.Fatal(err)
	}
	failed(token, errors.New("enable response lost after write"))
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()
	if w.state.Identity.ICCID != "" || w.state.Identity.Ready {
		t.Fatalf("uncertain card state restored as old identity: %+v", w.state.Identity)
	}
}
