package esim

import (
	"context"
	"errors"
	"testing"

	"github.com/damonto/euicc-go/lpa"
	"github.com/iniwex5/vohive/internal/apduarbiter"
)

func TestProfilePreparationPrecedesEveryChannelOpen(t *testing.T) {
	for _, disable := range []bool{false, true} {
		for _, aid := range []string{"A0000005591010FFFFFFFF8900000100", ""} {
			t.Run(map[bool]string{false: "switch", true: "disable"}[disable]+"/"+aid, func(t *testing.T) {
				prepared := false
				opens := 0
				rootErr := errors.New("logical channel unavailable")
				arbiter := apduarbiter.New("test", apduarbiter.Options{})
				mgr := newManagerWithChannelFactory("test", func([]byte) (*lpa.Client, error) {
					opens++
					if !prepared || arbiter.Stats().ActiveBarriers != 1 {
						t.Fatal("management channel opened before preparation and barrier")
					}
					return nil, rootErr
				}, nil, nil, nil)
				mgr.apduArbiter = arbiter
				mgr.onBeforeSwitch = func(SwitchOperation, string) (uint64, error) {
					if arbiter.Stats().ActiveBarriers != 0 {
						t.Fatal("barrier acquired before old session cleanup")
					}
					prepared = true
					return 42, nil
				}
				failed := false
				mgr.onSwitchFailed = func(_ SwitchOperation, token uint64, err error) {
					failed = true
					if token != 42 || !errors.Is(err, rootErr) || !errors.Is(err, ErrProfileCommandNotSent) {
						t.Fatalf("failure lost token, cause or pre-write status: token=%d err=%v", token, err)
					}
					if arbiter.Stats().ActiveBarriers != 0 || !mgr.opMu.TryLock() {
						t.Fatal("failure recovery ran while switch locks were held")
					}
					mgr.opMu.Unlock()
				}
				var err error
				if disable {
					err = mgr.DisableProfile(context.Background(), "8986001234567890123", aid)
				} else {
					_, err = mgr.SwitchProfileWithResult(context.Background(), "8986001234567890123", aid)
				}
				if err == nil || !failed || opens == 0 {
					t.Fatalf("pre-write failure not handled: err=%v failed=%v opens=%d", err, failed, opens)
				}
			})
		}
	}
}

func TestProfilePreparationErrorDoesNotOpenChannel(t *testing.T) {
	for _, disable := range []bool{false, true} {
		t.Run(map[bool]string{false: "switch", true: "disable"}[disable], func(t *testing.T) {
			mgr := newManagerWithChannelFactory("test", func([]byte) (*lpa.Client, error) {
				t.Fatal("channel opened despite failed VoWiFi cleanup")
				return nil, nil
			}, nil, nil, nil)
			rootErr := errors.New("VoWiFi cleanup failed")
			mgr.onBeforeSwitch = func(SwitchOperation, string) (uint64, error) { return 7, rootErr }
			failed := false
			mgr.onSwitchFailed = func(_ SwitchOperation, token uint64, err error) {
				failed = token == 7 && errors.Is(err, rootErr) && errors.Is(err, ErrProfileCommandNotSent)
			}
			var err error
			if disable {
				err = mgr.DisableProfile(context.Background(), "8986001234567890123", "")
			} else {
				_, err = mgr.SwitchProfileWithResult(context.Background(), "8986001234567890123", "")
			}
			if !errors.Is(err, rootErr) || !failed {
				t.Fatalf("cleanup error not propagated: err=%v failed=%v", err, failed)
			}
		})
	}
}
