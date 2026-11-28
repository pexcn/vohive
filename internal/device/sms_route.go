package device

import (
	"fmt"
	"strings"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vowifi-go/runtimehost"
)

// SMSBearer selects the live transport and keeps VoLTE SMS off the modem
// path even while the requested IMS registration is starting or recovering.
func (p *Pool) SMSBearer(w *Worker, requested string) (string, error) {
	if p.IsESIMSwitching(w.ID) {
		return "", fmt.Errorf("设备正在切卡，短信服务暂不可用")
	}
	requested = strings.ToLower(strings.TrimSpace(requested))
	volte := w.requiresVoLTESMS()
	if requested == db.SMSBearerCellular && volte {
		return "", fmt.Errorf("用户态 VoLTE 已启用，短信必须通过 IMS 发送")
	}
	if requested == db.SMSBearerCellular {
		return db.SMSBearerCellular, nil
	}
	inst := p.voWiFiHost().Instance(w.ID)
	if inst != nil {
		state := inst.State()
		if volte && (state.IMSAccess != runtimehost.IMSAccessLTE || !state.SMSReady) {
			return "", fmt.Errorf("VoLTE 短信服务尚未就绪")
		}
		if state.IMSAccess == runtimehost.IMSAccessLTE {
			if !state.SMSReady || !volte {
				return "", fmt.Errorf("VoLTE 短信服务尚未就绪")
			}
			return db.SMSBearerVoLTE, nil
		}
		return db.SMSBearerVoWiFi, nil
	}
	if volte || requested == "ims" {
		return "", fmt.Errorf("IMS 短信服务尚未就绪")
	}
	return db.SMSBearerCellular, nil
}

func smsBearerForAccess(access string) string {
	if access == runtimehost.IMSAccessLTE {
		return db.SMSBearerVoLTE
	}
	return db.SMSBearerVoWiFi
}

func (w *Worker) requiresVoLTESMS() bool {
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	return w.Config.VoLTEEnabled
}
