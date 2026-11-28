package device

import (
	"context"
	"fmt"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/carrier"
)

func (p *Pool) openVoLTEIMSBearer(ctx context.Context, w *Worker, iccid string, parameters carrier.PresetParameters) (runtimehost.IMSBearer, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !w.voLTERequested() || p.IsESIMSwitching(w.ID) || db.CanonicalICCID(w.CurrentICCID()) != db.CanonicalICCID(iccid) {
		return runtimehost.IMSBearer{}, fmt.Errorf("VoLTE IMS 承载已过期，SIM 或卡策略发生变化")
	}
	if err := w.EnsureQMIRegistration(ctx, true); err != nil {
		return runtimehost.IMSBearer{}, err
	}
	release, err := w.BeginRadioOnline(ctx)
	if err != nil {
		return runtimehost.IMSBearer{}, err
	}
	defer release()
	return w.QMICore.OpenIMSBearer(ctx, parameters.APN, parameters.IPStack)
}
