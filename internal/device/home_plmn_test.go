package device

import (
	"context"
	"testing"

	"github.com/iniwex5/vohive/internal/backend"
)

type homePLMNRefreshBackend struct {
	imsiIdentityBackend
	metadataErr error
	homeErr     error
	imsiReads   int
	homeReads   int
}

func (b *homePLMNRefreshBackend) GetIMSILive(ctx context.Context) (string, error) {
	b.imsiReads++
	return b.imsiIdentityBackend.GetIMSILive(ctx)
}

func (b *homePLMNRefreshBackend) GetSIMMetadataLive(context.Context) (*backend.SIMMetadata, error) {
	return b.simMetadata, b.metadataErr
}

func (b *homePLMNRefreshBackend) GetNativeMCCMNC(context.Context) (string, string, error) {
	b.homeReads++
	return b.nativeMCC, b.nativeMNC, b.homeErr
}

func newHomePLMNRefreshWorker(mode string) (*Worker, *homePLMNRefreshBackend) {
	b := &homePLMNRefreshBackend{
		imsiIdentityBackend: imsiIdentityBackend{workerStartupIdentityBackendStub: workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{workerStatusBackendStub: workerStatusBackendStub{mode: mode}},
			liveICCID:              "8985200000000000001", liveIMSI: "454003063191853",
		}},
		homeErr: context.DeadlineExceeded,
	}
	return &Worker{ID: "home-plmn", Backend: b}, b
}

func TestRefreshIdentityLiveCompletesHomePLMNFromCurrentIMSI(t *testing.T) {
	for _, mode := range []string{backend.BackendAT, backend.BackendQMI, backend.BackendMBIM} {
		for _, read := range []string{"empty_metadata", "partial_metadata", "metadata_timeout"} {
			t.Run(mode+"/"+read, func(t *testing.T) {
				w, b := newHomePLMNRefreshWorker(mode)
				switch read {
				case "partial_metadata":
					b.simMetadata = &backend.SIMMetadata{GID1: "01", NativeMCC: "454"}
				case "metadata_timeout":
					b.metadataErr = context.DeadlineExceeded
				}
				if err := w.RefreshIdentityLive(nil, "test_home_fallback"); err != nil {
					t.Fatal(err)
				}
				status := w.ProjectDeviceStatus()
				if status.IMSI != b.liveIMSI || status.NativeMCC != "454" || status.NativeMNC != "00" {
					t.Fatalf("current IMSI did not complete the home PLMN: %+v", status)
				}
				if b.imsiReads != 1 || b.homeReads != 1 {
					t.Fatalf("fallback added SIM reads: IMSI=%d home=%d", b.imsiReads, b.homeReads)
				}
			})
		}
	}
}

func TestRefreshIdentityLivePreservesSameSIMHomePLMN(t *testing.T) {
	for _, mode := range []string{backend.BackendAT, backend.BackendQMI, backend.BackendMBIM} {
		t.Run(mode, func(t *testing.T) {
			w, b := newHomePLMNRefreshWorker(mode)
			b.liveIMSI = "234033123456789"
			w.state.Identity.ICCID, w.state.Identity.IMSI = b.liveICCID, b.liveIMSI
			w.state.Identity.NativeMCC, w.state.Identity.NativeMNC = "234", "033"
			// Preserve an EF-AD-derived three-digit MNC, even when a partial
			// read includes a different MCC. Do not construct a mixed PLMN.
			b.simMetadata = &backend.SIMMetadata{GID1: "01", NativeMCC: "999"}
			if err := w.RefreshIdentityLive(nil, "test_partial_metadata"); err != nil {
				t.Fatal(err)
			}
			status := w.ProjectDeviceStatus()
			if status.NativeMCC != "234" || status.NativeMNC != "033" || status.GID1 != "01" {
				t.Fatalf("partial metadata erased or mixed the cached home PLMN: %+v", status)
			}
			b.simMetadata, b.metadataErr = nil, context.DeadlineExceeded
			if err := w.RefreshIdentityLive(nil, "test_metadata_timeout"); err != nil {
				t.Fatal(err)
			}
			if got := w.ProjectDeviceStatus(); got.NativeMCC != "234" || got.NativeMNC != "033" {
				t.Fatalf("temporary failure degraded the cached MNC length: %+v", got)
			}
			// A complete fresh SIM result still takes precedence over cache.
			b.metadataErr = nil
			b.simMetadata = &backend.SIMMetadata{NativeMCC: "234", NativeMNC: "03"}
			if err := w.RefreshIdentityLive(nil, "test_complete_metadata"); err != nil {
				t.Fatal(err)
			}
			if got := w.ProjectDeviceStatus(); got.NativeMCC != "234" || got.NativeMNC != "03" {
				t.Fatalf("fresh complete metadata was ignored: %+v", got)
			}
		})
	}
}

func TestRefreshIdentityLiveHomePLMNDoesNotCrossSIMIdentities(t *testing.T) {
	for _, mode := range []string{backend.BackendAT, backend.BackendQMI, backend.BackendMBIM} {
		for _, read := range []string{"new_sim_not_ready", "new_sim_placeholder", "new_sim_ready", "new_imsi_same_iccid"} {
			t.Run(mode+"/"+read, func(t *testing.T) {
				w, b := newHomePLMNRefreshWorker(mode)
				w.state.Identity.ICCID, w.state.Identity.IMSI = "previous-card", "262011234567890"
				w.state.Identity.NativeMCC, w.state.Identity.NativeMNC = "262", "01"
				w.state.Identity.GID2 = "previous-card-data"
				b.simMetadata = &backend.SIMMetadata{NativeMCC: "454", GID1: "new-card-data"}
				wantMCC, wantMNC := "454", "00"
				switch read {
				case "new_sim_not_ready":
					b.liveIMSI, b.imsiErr = "", context.DeadlineExceeded
					wantMCC, wantMNC = "", ""
				case "new_sim_placeholder":
					b.liveIMSI = "999999999999999"
					wantMCC, wantMNC = "", ""
				case "new_imsi_same_iccid":
					w.state.Identity.ICCID = b.liveICCID
				}
				if err := w.RefreshIdentityLive(nil, "test_identity_change"); err != nil {
					t.Fatal(err)
				}
				status := w.ProjectDeviceStatus()
				if status.NativeMCC != wantMCC || status.NativeMNC != wantMNC || status.GID2 != "" || status.ICCID != b.liveICCID {
					t.Fatalf("home PLMN crossed identities: %+v", status)
				}
				if read == "new_sim_not_ready" {
					b.liveIMSI, b.imsiErr = "454003063191853", nil
					if err := w.RefreshIdentityLive(nil, "test_new_sim_recovered"); err != nil {
						t.Fatal(err)
					}
					if got := w.ProjectDeviceStatus(); got.NativeMCC != "454" || got.NativeMNC != "00" {
						t.Fatalf("new SIM's home PLMN did not recover: %+v", got)
					}
				}
			})
		}
	}
}

func TestRefreshIdentityLiveDoesNotInferHomePLMNFromInvalidIMSI(t *testing.T) {
	for _, imsi := range []string{"", "999999999999999", "45400invalid", "4540", "4540012345678901"} {
		t.Run(imsi, func(t *testing.T) {
			w, b := newHomePLMNRefreshWorker(backend.BackendQMI)
			b.liveIMSI = imsi
			if err := w.RefreshIdentityLive(nil, "test_invalid_home_imsi"); err != nil {
				t.Fatal(err)
			}
			if got := w.ProjectDeviceStatus(); got.NativeMCC != "" || got.NativeMNC != "" {
				t.Fatalf("invalid IMSI populated the home PLMN: %+v", got)
			}
		})
	}
}
