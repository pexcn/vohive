package device

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/db"
)

type imsiIdentityBackend struct {
	workerStartupIdentityBackendStub
	imsiErr error
}

func (b *imsiIdentityBackend) GetIMSILive(context.Context) (string, error) {
	return b.liveIMSI, b.imsiErr
}

func TestRefreshIdentityLiveDistinguishesInvalidIMSIFromTemporaryFailure(t *testing.T) {
	const oldICCID, newICCID, oldIMSI, newIMSI = "8986000000000000001", "8986000000000000002", "460001234567890", "234150123456789"
	for _, mode := range []string{backend.BackendAT, backend.BackendQMI, backend.BackendMBIM} {
		for _, tc := range []struct {
			name, iccid, imsi, wantIMSI string
			err                         error
		}{
			{"normal", oldICCID, newIMSI, newIMSI, nil},
			{"placeholder clears valid cache", oldICCID, "999999999999999", "", nil},
			{"temporary failure retains same card", oldICCID, "", oldIMSI, context.DeadlineExceeded},
			{"empty read retains same card", oldICCID, "", oldIMSI, nil},
			{"different card with read failure", newICCID, "", "", context.DeadlineExceeded},
			{"different card with empty read", newICCID, "", "", nil},
			{"different card with placeholder", newICCID, "999999999999999", "", nil},
			{"typed placeholder error", newICCID, "", "", backend.ErrPlaceholderIMSI},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				b := &imsiIdentityBackend{workerStartupIdentityBackendStub: workerStartupIdentityBackendStub{
					workerPhoneBackendStub: workerPhoneBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: mode}},
					liveICCID:              tc.iccid, liveIMSI: tc.imsi,
				}, imsiErr: tc.err}
				w := &Worker{ID: "test-imsi", Backend: b}
				w.state.Identity.ICCID, w.state.Identity.IMSI = oldICCID, oldIMSI
				if err := w.RefreshIdentityLive(nil, "test_imsi"); err != nil {
					t.Fatal(err)
				}
				if got := w.ProjectDeviceStatus(); got.ICCID != tc.iccid || got.IMSI != tc.wantIMSI {
					t.Fatalf("identity = %s/%s, want %s/%s", got.ICCID, got.IMSI, tc.iccid, tc.wantIMSI)
				}
				if !w.state.Identity.Ready {
					t.Fatal("recognized ICCID must remain available to card management")
				}
				if tc.wantIMSI == "" && w.SIMIdentityAllowsOverviewFallback() {
					t.Fatal("missing IMSI must not enable stale identity fallback")
				}
				if tc.imsi == "999999999999999" || errors.Is(tc.err, backend.ErrPlaceholderIMSI) {
					if !strings.Contains(w.state.Identity.LastError, backend.ErrPlaceholderIMSI.Error()) {
						t.Fatalf("missing placeholder diagnosis: %q", w.state.Identity.LastError)
					}
				}
				// A later normal read recovers through the existing identity path.
				b.liveIMSI, b.imsiErr = newIMSI, nil
				if err := w.RefreshIdentityLive(nil, "test_recovered"); err != nil {
					t.Fatal(err)
				}
				if w.GetCachedIMSI() != newIMSI || w.state.Identity.LastError != "" {
					t.Fatalf("identity did not recover: %+v", w.state.Identity)
				}
			})
		}
	}
}

func TestPostSwitchIdentityDoesNotAcceptPlaceholderIMSI(t *testing.T) {
	p := NewPool(nil)
	b := &imsiIdentityBackend{workerStartupIdentityBackendStub: workerStartupIdentityBackendStub{
		workerPhoneBackendStub: workerPhoneBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI}},
		liveICCID:              "8986000000000000002", liveIMSI: "999999999999999",
	}}
	w := &Worker{ID: "test-imsi", Backend: b}
	w.state.Identity.ICCID, w.state.Identity.IMSI = "8986000000000000001", "460001234567890"
	ready, err := p.refreshPostSwitchIdentityWithPolling(w.ID, w, esimSwitchContext{
		ICCIDBefore: w.state.Identity.ICCID, IMSIBefore: w.state.Identity.IMSI, TargetICCID: b.liveICCID,
	}, time.Nanosecond, time.Millisecond)
	if err != nil || !ready || w.state.Identity.ICCID != b.liveICCID || w.GetCachedIMSI() != "" {
		t.Fatalf("post-switch identity: ready=%v err=%v state=%+v", ready, err, w.state.Identity)
	}
	if !strings.Contains(w.state.Identity.LastError, backend.ErrPlaceholderIMSI.Error()) {
		t.Fatalf("placeholder diagnosis was lost: %+v", w.state.Identity)
	}
	b.liveIMSI = "234150123456789"
	if err := w.RefreshIdentityLive(nil, "post_switch_recovered"); err != nil {
		t.Fatal(err)
	}
	if w.GetCachedIMSI() != b.liveIMSI {
		t.Fatal("IMSI did not recover after switching")
	}
}

type recoveringIMSIBackend struct {
	imsiIdentityBackend
	reads int
}

func (b *recoveringIMSIBackend) GetIMSILive(context.Context) (string, error) {
	b.reads++
	if b.reads == 1 {
		return "999999999999999", nil
	}
	return "234150123456789", nil
}

func TestPostSwitchIdentityWaitsPastPlaceholderForNormalIMSI(t *testing.T) {
	b := &recoveringIMSIBackend{imsiIdentityBackend: imsiIdentityBackend{
		workerStartupIdentityBackendStub: workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI}},
			liveICCID:              "8986000000000000002",
		},
	}}
	w := &Worker{ID: "test-imsi", Backend: b}
	ready, err := NewPool(nil).refreshPostSwitchIdentityWithPolling(w.ID, w, esimSwitchContext{
		ICCIDBefore: "8986000000000000001", IMSIBefore: "460001234567890", TargetICCID: b.liveICCID,
	}, time.Second, time.Millisecond)
	if err != nil || !ready || b.reads != 2 || w.GetCachedIMSI() != "234150123456789" || w.state.Identity.LastError != "" {
		t.Fatalf("did not wait for recovered IMSI: ready=%v err=%v reads=%d state=%+v", ready, err, b.reads, w.state.Identity)
	}
}

func TestBuildVoWiFiStartProfileRejectsPlaceholderBeforePreparation(t *testing.T) {
	b := &vowifiLiveIdentityBackendStub{
		workerSMSCBackendStub: workerSMSCBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI}},
		liveIMSI:              "999999999999999",
	}
	w := &Worker{ID: "test-imsi", Backend: b}
	w.state.Identity.IMSI = "460001234567890"
	if _, err := NewPool(nil).BuildVoWiFiStartProfile(w); !errors.Is(err, backend.ErrPlaceholderIMSI) {
		t.Fatalf("placeholder reached VoWiFi preparation: %v", err)
	}
}

func TestPersistIdentityStateRejectsPlaceholderButKeepsCard(t *testing.T) {
	initDevicePhoneNumberTestDB(t)
	w := &Worker{ID: "test-imsi", Backend: &workerPhoneBackendStub{
		workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI},
	}}
	w.state.Identity.IMEI, w.state.Identity.ICCID = "861234567890123", "8986000000000000001"
	w.state.Identity.IMSI, w.state.Identity.Ready = "999999999999999", true
	p := NewPool(nil)
	p.PersistRuntimeState(w)
	p.PersistIdentityState(w)
	var row db.SIMCard
	if err := db.DB.Where("iccid = ?", w.state.Identity.ICCID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.IMSI != "" {
		t.Fatalf("placeholder persisted: %q", row.IMSI)
	}
	var count int64
	if err := db.DB.Model(&db.SIMSubscription{}).Where("imsi = ?", "999999999999999").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid subscription created: count=%d err=%v", count, err)
	}
}

func TestWorkerGetIMSIDoesNotReturnPlaceholder(t *testing.T) {
	w := &Worker{Backend: &workerPhoneBackendStub{imsi: "999999999999999"}}
	if got := w.GetIMSI(); got != "" {
		t.Fatalf("placeholder exposed to IMSI consumers: %q", got)
	}
}

type unavailableIMSIBackend struct{ imsiIdentityBackend }

func (b *unavailableIMSIBackend) GetIMSI(context.Context) (string, error) {
	return "", b.imsiErr
}

func TestWorkerATIMSIFallbackStaysScopedToCurrentICCID(t *testing.T) {
	b := &unavailableIMSIBackend{imsiIdentityBackend: imsiIdentityBackend{
		workerStartupIdentityBackendStub: workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendAT}},
			liveICCID:              "8986000000000000002",
		}, imsiErr: context.DeadlineExceeded,
	}}
	w := &Worker{Backend: b}
	w.state.Identity.ICCID, w.state.Identity.IMSI = "8986000000000000001", "460001234567890"
	if got := w.GetIMSI(); got != "460001234567890" {
		t.Fatalf("same-card cached fallback lost: %q", got)
	}
	if err := w.RefreshIdentityLive(nil, "card_changed"); err != nil {
		t.Fatal(err)
	}
	if got := w.GetIMSI(); got != "" {
		t.Fatalf("fallback reused previous card IMSI: %q", got)
	}
	w.state.Identity.IMSI = "460001234567890"
	b.imsiErr = backend.ErrPlaceholderIMSI
	if got := w.GetIMSI(); got != "" {
		t.Fatalf("explicit placeholder failure fell back to cache: %q", got)
	}
}
