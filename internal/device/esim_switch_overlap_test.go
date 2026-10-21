package device

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/internal/esim"
	"github.com/iniwex5/vohive/internal/vowifihost"
)

func TestESIMSwitchRejectsOverlapWithoutChangingActiveTask(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	be := &esimSwitchRestoreBackendStub{mode: backend.BackendQMI, getMode: backend.ModeRFOff}
	w := &Worker{ID: "dev", Backend: be}
	p.workers[w.ID] = w
	cleanupCalls := 0
	p.voWiFiHost().LifecycleControllerForTest().TestRun = func(context.Context, vowifihost.LifecycleCommand) error {
		cleanupCalls++
		return nil
	}
	token, err := p.handleESIMSwitchBefore(w.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	generation := w.state.Identity.Generation
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			next, err := p.handleESIMSwitchBefore(w.ID, "second")
			if next != 0 || !errors.Is(err, esim.ErrOperationInProgress) {
				t.Errorf("overlap acquired ownership: token=%d err=%v", next, err)
			}
		}()
	}
	wg.Wait()
	if cleanupCalls != 1 || w.state.Identity.Generation != generation || !p.isLatestSwitchToken(w.ID, token) {
		t.Fatal("rejected request changed lifecycle, identity generation or switch token")
	}
	p.handleESIMSwitchFailedWithError(w.ID, token, errors.New("command failed"))
	if next := p.beginESIMSwitch(w.ID, "second"); next.SwitchToken <= token {
		t.Fatal("switch remained busy after failure cleanup")
	}
}

type delayedSwitchIdentityBackend struct {
	esimSwitchRestoreBackendStub
	entered chan struct{}
	release chan struct{}
}

func (b *delayedSwitchIdentityBackend) GetIMSILive(context.Context) (string, error) {
	close(b.entered)
	<-b.release
	return "old-imsi", nil
}

func TestPostSwitchIdentityRejectsSupersededReplyBeforeCacheAndDatabase(t *testing.T) {
	for _, invalidate := range []string{"token", "generation"} {
		t.Run(invalidate, func(t *testing.T) {
			initDevicePhoneNumberTestDB(t)
			p := NewPool(nil)
			defer p.cancel()
			be := &delayedSwitchIdentityBackend{
				esimSwitchRestoreBackendStub: esimSwitchRestoreBackendStub{mode: backend.BackendQMI, getMode: backend.ModeRFOff, liveICCID: "8986000000000000001"},
				entered:                      make(chan struct{}), release: make(chan struct{}),
			}
			t.Cleanup(func() {
				select {
				case <-be.release:
				default:
					close(be.release)
				}
			})
			w := &Worker{ID: "dev", Backend: be}
			w.state.Identity.IMEI = "imei-switch"
			p.workers[w.ID] = w
			if err := db.UpsertDevice("imei-switch", w.ID, "EC20", ""); err != nil {
				t.Fatal(err)
			}
			snapshot := p.beginESIMSwitch(w.ID, be.liveICCID)
			snapshot.IdentityGeneration = w.BeginSIMIdentityTransition(be.liveICCID, "first")
			done := make(chan error, 1)
			go func() {
				_, err := p.refreshPostSwitchIdentity(w.ID, w, snapshot)
				done <- err
			}()
			select {
			case <-be.entered:
			case <-time.After(time.Second):
				t.Fatal("identity query did not start")
			}
			if invalidate == "token" {
				p.clearESIMSwitchIfToken(w.ID, snapshot.SwitchToken)
				p.beginESIMSwitch(w.ID, "8986000000000000002")
			} else {
				w.BeginSIMIdentityTransition("8986000000000000002", "new generation")
			}
			w.cacheMu.Lock()
			w.state.Identity.ICCID, w.state.Identity.IMSI = "8986000000000000002", "new-imsi"
			w.cacheMu.Unlock()
			p.PersistIdentityState(w)
			close(be.release)
			select {
			case err := <-done:
				if !errors.Is(err, errStalePostSwitchIdentity) {
					t.Fatalf("late reply err=%v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("late identity query did not exit")
			}
			if status := w.ProjectDeviceStatus(); status.ICCID != "8986000000000000002" || status.IMSI != "new-imsi" {
				t.Fatalf("late query overwrote identity: %+v", status)
			}
			var stored db.Device
			if err := db.DB.First(&stored, "imei = ?", "imei-switch").Error; err != nil {
				t.Fatal(err)
			}
			if stored.CurrentICCID == nil || *stored.CurrentICCID != "8986000000000000002" {
				t.Fatalf("late query overwrote device SIM association: %+v", stored)
			}
		})
	}
}

func TestPostSwitchIdentityRefreshStillRunsAfterSuccessfulCompletion(t *testing.T) {
	p := NewPool(nil)
	defer p.cancel()
	be := &esimSwitchRestoreBackendStub{mode: backend.BackendQMI, getMode: backend.ModeRFOff, liveICCID: "target", liveIMSI: "imsi"}
	w := &Worker{ID: "dev", Backend: be}
	p.workers[w.ID] = w
	snapshot := p.beginESIMSwitch(w.ID, "target")
	snapshot.IdentityGeneration = w.BeginSIMIdentityTransition("target", "switch")
	p.clearESIMSwitchIfToken(w.ID, snapshot.SwitchToken)
	if ready, err := p.refreshPostSwitchIdentity(w.ID, w, snapshot); err != nil || !ready {
		t.Fatalf("completed switch lost its bounded identity refresh: ready=%v err=%v", ready, err)
	}
}
