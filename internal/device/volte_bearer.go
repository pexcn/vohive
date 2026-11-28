package device

import (
	"context"
	"fmt"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	qmicore "github.com/iniwex5/vohive/internal/qmi"
	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/carrier"
)

func (p *Pool) openVoLTEIMSBearer(ctx context.Context, w *Worker, iccid string, parameters carrier.PresetParameters) (runtimehost.IMSBearer, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !w.voLTERequested() || p.IsESIMSwitching(w.ID) || db.CanonicalICCID(w.CurrentICCID()) != db.CanonicalICCID(iccid) {
		return runtimehost.IMSBearer{}, fmt.Errorf("VoLTE IMS 承载已过期，SIM 或卡策略发生变化")
	}
	ensureRegistration := w.EnsureQMIRegistration
	if w.MBIMCore != nil {
		ensureRegistration = w.EnsureMBIMRegistration
	}
	if err := ensureRegistration(ctx, true); err != nil {
		return runtimehost.IMSBearer{}, err
	}
	release, err := w.BeginRadioOnline(ctx)
	if err != nil {
		return runtimehost.IMSBearer{}, err
	}
	defer release()
	if w.MBIMCore != nil {
		return w.MBIMCore.OpenIMSBearer(ctx, w.Config.Interface, parameters.APN, parameters.IPStack)
	}
	if w.QMICore != nil {
		return w.QMICore.OpenIMSBearer(ctx, qmicore.IMSBearerOptions{
			APN: parameters.APN, IPStack: parameters.IPStack,
			MuxID: parameters.IMSQMIMuxID, ProfileIndex: parameters.IMSQMIProfileIndex,
			EndpointType: parameters.IMSQMIEndpointType, InterfaceID: parameters.IMSQMIInterfaceID,
			ClientType: parameters.IMSQMIClientType,
		})
	}
	return runtimehost.IMSBearer{}, fmt.Errorf("当前设备没有支持 IMS 的数据接口")
}
