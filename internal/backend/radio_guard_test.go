package backend

import (
	"context"
	"errors"
	"testing"
)

func TestRadioGuardRejectsOnlineBeforeHardware(t *testing.T) {
	denied := errors.New("card requires RF off")
	for _, b := range []interface {
		DeviceBackend
		SetRadioOnlineGuard(func(context.Context) (func(), error))
	}{&ATBackend{}, &QMIBackend{}, &MBIMBackend{}} {
		t.Run(b.Mode(), func(t *testing.T) {
			calls := 0
			b.SetRadioOnlineGuard(func(context.Context) (func(), error) {
				calls++
				return nil, denied
			})
			// All hardware sources are nil: reaching one would panic.
			if err := b.SetOperatingMode(t.Context(), ModeOnline); !errors.Is(err, denied) {
				t.Fatalf("Online bypassed guard: %v", err)
			}
			if calls != 1 {
				t.Fatalf("guard calls=%d", calls)
			}
		})
	}
}

func TestRadioGuardStillAllowsTurningRFOff(t *testing.T) {
	b := NewMBIMBackend("", &fakeMBIMSource{})
	b.SetRadioOnlineGuard(func(context.Context) (func(), error) {
		t.Fatal("RF off must not require permission to enable RF")
		return nil, nil
	})
	if err := b.SetOperatingMode(t.Context(), ModeRFOff); err != nil {
		t.Fatal(err)
	}
}

func TestQMIRebootDoesNotUseOnlineGuard(t *testing.T) {
	b, err := NewQMIBackend("", &qmiBackendSendSourceStub{})
	if err != nil {
		t.Fatal(err)
	}
	b.SetRadioOnlineGuard(func(context.Context) (func(), error) {
		t.Fatal("explicit reboot consulted the Online policy")
		return nil, nil
	})
	if err := b.Reboot(t.Context()); err != nil {
		t.Fatal(err)
	}
}
