package device

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vohive/pkg/smscodec"
	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
)

func (p *Pool) SubmitSIPBridgeSMS(ctx context.Context, job db.SIPBridgeSMS, requested string) (id, bearer, state string, err error) {
	w := p.GetWorker(job.DeviceID)
	if w == nil || job.ICCID == "" || db.CanonicalICCID(w.CurrentICCID()) != job.ICCID {
		return "", "", "failed", fmt.Errorf("提交时绑定的 SIM 不再可用")
	}
	bearer, err = p.SMSBearer(w, requested)
	if err != nil {
		return "", "", "failed", err
	}
	if err := ctx.Err(); err != nil {
		return "", bearer, "failed", err
	}
	imsi := w.GetCachedIMSI()
	if imsi == "" {
		imsi = w.GetIMSI()
	}
	if p.GetWorker(job.DeviceID) != w || p.IsESIMSwitching(job.DeviceID) || db.CanonicalICCID(w.CurrentICCID()) != job.ICCID {
		return "", bearer, "failed", fmt.Errorf("发送时 SIM 身份发生变化")
	}
	if bearer != db.SMSBearerCellular {
		outcome, err := p.SendVoWiFiSMSWithOptions(ctx, job.DeviceID, job.Peer, job.Content, smscodec.SubmitOptions{})
		switch {
		case err == nil:
			return outcome.MessageID, bearer, "submitted", nil
		case errors.Is(err, messaging.ErrSendOutcomeUnknown) || outcome.MessageID != "" && outcome.State != "failed":
			return outcome.MessageID, bearer, "unknown", err
		default:
			_ = RecordVoWiFiSMSSendFailure(p, job.DeviceID, job.Peer, job.Content, time.Now(), imsi)
			return outcome.MessageID, bearer, "failed", err
		}
	}
	err = w.SendSMS(job.Peer, job.Content)
	status, state := 2, "submitted"
	if err != nil {
		status, state = 3, "unknown"
	}
	if imsi != "" {
		if saveErr := db.SaveSMS(imsi, w.ID, job.Peer, job.Content, 2, status, time.Now()); saveErr != nil {
			logger.Warn("保存 SIP 发出的蜂窝短信失败", "device", w.ID, "err", saveErr)
		}
	}
	return "", bearer, state, err
}
