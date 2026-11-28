package device

import (
	"context"
	"strings"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vowifi-go/runtimehost/identity"
)

// Retry outside the startup request. Sampling never takes ownership of RF
// and stops as soon as the worker, SIM, or VoWiFi policy changes.
func (p *Pool) scheduleHomeIMSICapture(w *Worker, reader liveSIMIdentityReader, profile identity.Profile, plmn string) {
	if !w.homeIMSICapture.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer w.homeIMSICapture.Store(false)
		ctx, cancel := context.WithTimeout(p.Context(), 2*time.Minute)
		defer cancel()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.stop:
				return
			case <-ticker.C:
			}
			if p.GetWorker(w.ID) != w || p.IsESIMSwitching(w.ID) ||
				db.CanonicalICCID(w.CurrentICCID()) != profile.ICCID || w.voLTERequested() ||
				!p.currentCardPolicyAllowsVoWiFi(w, profile.ICCID, "home_imsi_capture") {
				return
			}
			home, _ := db.GetSIMHomeIdentity(profile.ICCID)
			if strings.HasPrefix(home.IMSI, plmn) {
				return
			}
			if imsi := p.observeVoWiFiHomeIMSIWithin(ctx, w, reader, &profile, plmn); imsi != "" {
				logger.Info("后台已采集 SIM 归属 IMSI", "device", w.ID)
				p.scheduleDesiredVoWiFiRecover(w.ID, "home_imsi_captured", time.Now())
				return
			}
		}
	}()
}
