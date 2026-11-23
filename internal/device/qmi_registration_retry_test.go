package device

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/cardpolicy"
	"github.com/iniwex5/vohive/internal/config"
)

func TestQMIRegistrationReconcileBackoffAndRecovery(t *testing.T) {
	w := &Worker{ID: "dev"}
	calls, waits := 0, 0
	wantDelays := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute,
		4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	w.reconcileQMIRegistration(context.Background(), "test", func(context.Context) error {
		calls++
		if calls <= len(wantDelays) {
			return fmt.Errorf("waiting for registration: %w", context.DeadlineExceeded)
		}
		return nil
	}, func(_ context.Context, delay time.Duration) error {
		if waits >= len(wantDelays) {
			t.Fatal("continued retrying after recovery")
		}
		if delay != wantDelays[waits] {
			t.Fatalf("retry %d delay = %v, want %v", waits+1, delay, wantDelays[waits])
		}
		waits++
		return nil
	})
	if waits != len(wantDelays) || calls != len(wantDelays)+1 {
		t.Fatalf("calls=%d waits=%d", calls, waits)
	}
}

func TestQMIRegistrationReconcileRecoversWithoutAnotherRegisterCommand(t *testing.T) {
	w := &Worker{ID: "dev"}
	ctrl := &qmiRegistrationTestController{servingSeq: qmiSearchingServingSeq(1, nil)}
	calls, waits := 0, 0
	w.reconcileQMIRegistration(context.Background(), "test", func(ctx context.Context) error {
		calls++
		return ensureQMIRegistration(ctx, w.ID, config.DeviceConfig{}, ctrl, ctrl,
			qmiRegistrationOptions{PollInterval: time.Millisecond, MaxAttempts: 2})
	}, func(context.Context, time.Duration) error {
		waits++
		ctrl.servingSeq = []*backend.ServingSystem{{RegStatus: 5, PSAttached: true}}
		return nil
	})
	if calls != 2 || waits != 1 || ctrl.registerCalls != 1 || len(ctrl.setModeCalls) != 0 {
		t.Fatalf("calls=%d waits=%d registers=%d radio=%v", calls, waits, ctrl.registerCalls, ctrl.setModeCalls)
	}
}

func TestQMIRegistrationReconcileDoesNotRetryOtherErrors(t *testing.T) {
	for _, err := range []error{nil, errQMIRegistrationDenied, errQMISIMNotReady,
		errQMIRegistrationSkipped, context.Canceled, errors.New("transport unavailable")} {
		t.Run(fmt.Sprint(err), func(t *testing.T) {
			w := &Worker{ID: "dev"}
			calls := 0
			w.reconcileQMIRegistration(context.Background(), "test", func(context.Context) error {
				calls++
				return err
			}, func(context.Context, time.Duration) error {
				t.Fatal("unexpected retry")
				return nil
			})
			if calls != 1 {
				t.Fatalf("calls=%d want 1", calls)
			}
		})
	}
}

func TestQMIRegistrationReconcileHonorsPolicyAfterBackoff(t *testing.T) {
	w := &Worker{ID: "dev"}
	calls := 0
	w.reconcileQMIRegistration(context.Background(), "test", func(context.Context) error {
		calls++
		return context.DeadlineExceeded
	}, func(context.Context, time.Duration) error {
		applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
		return nil
	})
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func waitQMIReconcileState(t *testing.T, w *Worker, wantInFlight, wantWaiting bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		w.qmiRegistrationMu.Lock()
		inFlight, waiting := w.qmiRegistrationInFlight, w.qmiRegistrationWaiting
		w.qmiRegistrationMu.Unlock()
		if inFlight == wantInFlight && waiting == wantWaiting {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("reconcile did not reach inFlight=%v waiting=%v", wantInFlight, wantWaiting)
}

func TestQMIRegistrationReconcileBackoffCancellation(t *testing.T) {
	for _, cause := range []string{"context", "worker_stop", "flight", "vowifi", "switch"} {
		t.Run(cause, func(t *testing.T) {
			w := &Worker{ID: "dev", stop: make(chan struct{})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !w.startQMIRegistrationReconcile(ctx, "test", func(context.Context) error {
				return context.DeadlineExceeded
			}) {
				t.Fatal("reconcile did not start")
			}
			waitQMIReconcileState(t, w, true, true)
			if w.startQMIRegistrationReconcile(ctx, "duplicate", func(context.Context) error {
				t.Error("duplicate reconcile ran during backoff")
				return nil
			}) {
				t.Fatal("duplicate reconcile accepted during backoff")
			}
			switch cause {
			case "context":
				cancel()
			case "worker_stop":
				close(w.stop)
			default:
				// Switch, flight and VoWiFi coordinators all cancel and drain
				// registration before taking ownership of the radio.
				if cause == "flight" {
					applyPolicyToWorker(w, cardpolicy.Policy{AirplaneEnabled: true})
				} else if cause == "vowifi" {
					applyPolicyToWorker(w, cardpolicy.Policy{VoWiFiEnabled: true})
				}
				drainCtx, drainCancel := context.WithTimeout(ctx, time.Second)
				defer drainCancel()
				if err := w.stopCellularRegistration(drainCtx); err != nil {
					t.Fatalf("drain backoff: %v", err)
				}
			}
			waitQMIReconcileState(t, w, false, false)
		})
	}
}

func TestQMIRegistrationReconcileParentDeadlineDoesNotRetry(t *testing.T) {
	w := &Worker{ID: "dev"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	w.reconcileQMIRegistration(ctx, "test", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, func(context.Context, time.Duration) error {
		t.Fatal("parent deadline retried")
		return nil
	})
}

func TestQMIRegistrationBackoffDoesNotSuppressUnhealthyEviction(t *testing.T) {
	p := NewPool(&config.Config{})
	w := &Worker{ID: "dev"}
	w.reconcileQMIRegistration(context.Background(), "test", func(context.Context) error {
		return context.DeadlineExceeded
	}, func(context.Context, time.Duration) error {
		w.qmiRegistrationMu.Lock()
		w.qmiRegistrationInFlight = true
		w.qmiRegistrationMu.Unlock()
		if suppressed, reason := p.suppressQMIUnhealthyEviction(w); suppressed {
			t.Fatalf("backoff suppressed unhealthy eviction: %s", reason)
		}
		return context.Canceled
	})
}
