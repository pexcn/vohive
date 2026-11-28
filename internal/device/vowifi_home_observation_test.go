package device

import (
	"context"
	"testing"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
)

type observedHomeSIM struct {
	workerStatusBackendStub
	iccid string
	reads int
}

func (s *observedHomeSIM) GetIMSILive(context.Context) (string, error) {
	s.reads++
	if s.reads == 1 {
		return "204040000000001", nil
	}
	return "515660000000001", nil
}
func (s *observedHomeSIM) GetICCIDLive(context.Context) (string, error) {
	return s.iccid, nil
}

func TestObserveVoWiFiHomeIMSIRequiresLowPowerAndSameCard(t *testing.T) {
	for _, tc := range []struct {
		name, iccid, want string
		mode              backend.OperatingMode
		reads             int
	}{
		{"same card repeats read", "8963660000000000001", "515660000000001", backend.ModeRFOff, 2},
		{"replacement card", "8963660000000000002", "", backend.ModeLowPower, 2},
		{"online", "8963660000000000001", "", backend.ModeOnline, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Pool{ctx: context.Background()}
			sim := &observedHomeSIM{workerStatusBackendStub: workerStatusBackendStub{opMode: tc.mode}, iccid: tc.iccid}
			w := &Worker{ID: "sim", Backend: sim}
			profile := &identity.Profile{ICCID: "8963660000000000001"}
			got := p.observeVoWiFiHomeIMSI(w, sim, profile, "51566")
			if got != tc.want || sim.reads != tc.reads || len(sim.setOpModeCalls) != 0 {
				t.Fatalf("observed IMSI = %q, reads %d, RF changes %v", got, sim.reads, sim.setOpModeCalls)
			}
		})
	}
}
