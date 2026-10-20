package db

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"time"
)

var ErrSMSReportAmbiguous = errors.New("ambiguous SMS report")

// ReserveSMSTPReference persists the part before any network write. A reference
// cannot be reused while an unresolved report is expected (seven days), or for
// ten minutes after any submission. Exhaustion is an error, never a resend.
func ReserveSMSTPReference(deviceID, imsi, id string, no int, callID string, rpMR int, at time.Time) (int, error) {
	if DB == nil {
		return 0, errors.New("SMS database unavailable")
	}
	var reference int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var d SMSDelivery
		if err := tx.Where("message_id = ? AND device_id = ? AND imsi = ?", id, deviceID, imsi).First(&d).Error; err != nil {
			return err
		}
		if no < 1 || no > d.PartsTotal || rpMR < 0 || rpMR > 255 {
			return errors.New("invalid SMS part")
		}
		var existing SMSDeliveryPart
		err := tx.Where("message_id = ? AND part_no = ?", id, no).First(&existing).Error
		if err == nil {
			if existing.TPMR != nil && existing.CallID == callID {
				reference = *existing.TPMR
				return nil
			}
			return errors.New("SMS part already reserved")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var used []SMSDeliveryPart
		err = tx.Model(&SMSDeliveryPart{}).Select("sms_delivery_part.*").Joins("JOIN sms_delivery ON sms_delivery.message_id = sms_delivery_part.message_id").
			Where("sms_delivery.imsi = ? AND LTRIM(sms_delivery.peer, '+') = ?", imsi, strings.TrimPrefix(d.Peer, "+")).
			Where("tp_mr IS NOT NULL AND (sent_at >= ? OR (sent_at >= ? AND sms_delivery_part.state NOT IN ('failed', 'timeout') AND (tp_state = '' OR tp_state IS NULL OR tp_state IN ?)))", at.Add(-10*time.Minute), at.Add(-7*24*time.Hour), []string{"retrying", "unknown"}).Find(&used).Error
		if err != nil {
			return err
		}
		busy := [256]bool{}
		for _, p := range used {
			if p.TPMR != nil && *p.TPMR >= 0 && *p.TPMR < 256 {
				busy[*p.TPMR] = true
			}
		}
		for n := 0; n < 256; n++ {
			reference = (rpMR + n) & 255
			if !busy[reference] {
				return tx.Create(&SMSDeliveryPart{MessageID: id, PartNo: no, CallID: callID, RPMR: rpMR, TPMR: &reference, State: "pending", SentAt: at, CreatedAt: at, UpdatedAt: at}).Error
			}
		}
		return errors.New("all SMS TP references outstanding for recipient")
	})
	return reference, err
}

func RecordSMSSIPResult(id string, no, code int, state string, at time.Time) error {
	if DB == nil {
		return errors.New("SMS database unavailable")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var p SMSDeliveryPart
		if err := tx.Where("message_id = ? AND part_no = ?", id, no).First(&p).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": at}
		if code != 0 {
			updates["sip_code"] = code
		}
		// A late SIP response or transaction timeout must not erase RP/TP evidence.
		if p.ReportAt == nil && (p.TPStatus == nil || p.TPState == "unknown" || p.TPState == "retrying") {
			updates["state"] = state
		}
		if err := tx.Model(&p).Updates(updates).Error; err != nil {
			return err
		}
		return recomputeSMSDelivery(tx, id, at)
	})
}

func smsReportQuery(tx *gorm.DB, deviceID, imsi string) *gorm.DB {
	return tx.Model(&SMSDeliveryPart{}).Select("sms_delivery_part.*").Joins("JOIN sms_delivery ON sms_delivery.message_id = sms_delivery_part.message_id").Where("sms_delivery.device_id = ? AND sms_delivery.imsi = ?", deviceID, imsi)
}

func RecordSMSRPReport(deviceID, imsi, callID string, mr int, state string, cause int, at time.Time) error {
	if DB == nil {
		return errors.New("SMS database unavailable")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		q := smsReportQuery(tx, deviceID, imsi).Where("sms_delivery_part.rp_mr = ?", mr)
		if callID != "" {
			q = q.Where("sms_delivery_part.call_id = ?", callID)
		} else {
			q = q.Where("sms_delivery_part.sent_at >= ?", at.Add(-120*time.Second))
		}
		var parts []SMSDeliveryPart
		if err := q.Limit(2).Find(&parts).Error; err != nil {
			return err
		}
		if len(parts) == 0 {
			return gorm.ErrRecordNotFound
		}
		if len(parts) != 1 {
			return ErrSMSReportAmbiguous
		}
		p := parts[0]
		if p.ReportAt != nil || p.TPStatus != nil && state == "failed" {
			return nil
		}
		text := ""
		if state == "failed" {
			text = fmt.Sprintf("RP cause %d", cause)
		}
		if err := tx.Model(&p).Updates(map[string]any{"state": state, "rp_cause": cause, "report_at": at, "error_text": text, "updated_at": at}).Error; err != nil {
			return err
		}
		return recomputeSMSDelivery(tx, p.MessageID, at)
	})
}

type SMSTPReport struct {
	Reference, Status                    int
	Recipient                            string
	ServiceCentreTime, DischargeTime, At time.Time
}

func smsTPState(status int) string {
	switch {
	case status == 0:
		return "delivered"
	case status > 0 && status <= 2:
		return "completed_unconfirmed"
	case status >= 0x20 && status <= 0x3f:
		return "retrying"
	case status >= 0x40 && status <= 0x7f:
		return "delivery_failed"
	default:
		return "unknown"
	}
}

func RecordSMSTPReport(deviceID, imsi string, r SMSTPReport) error {
	if DB == nil {
		return errors.New("SMS database unavailable")
	}
	if r.Reference < 0 || r.Reference > 255 || r.Status < 0 || r.Status > 255 || r.Recipient == "" || r.ServiceCentreTime.IsZero() || r.ServiceCentreTime.After(r.At.Add(5*time.Minute)) {
		return gorm.ErrRecordNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var candidates []SMSDeliveryPart
		// Match TP-MR, recipient and the fixed subscription, never RP-MR. Bound a
		// first report by submission time; subsequent reports require exact SCTS.
		q := smsReportQuery(tx, deviceID, imsi).Where("tp_mr = ? AND LTRIM(sms_delivery.peer, '+') = ?", r.Reference, strings.TrimPrefix(r.Recipient, "+")).
			Where("sms_delivery_part.sent_at >= ? AND sms_delivery_part.sent_at <= ?", r.ServiceCentreTime.Add(-24*time.Hour), r.ServiceCentreTime.Add(5*time.Minute))
		if err := q.Find(&candidates).Error; err != nil {
			return err
		}
		var matches []SMSDeliveryPart
		for _, p := range candidates {
			if p.ServiceCentreTime == nil || p.ServiceCentreTime.Equal(r.ServiceCentreTime) {
				matches = append(matches, p)
			}
		}
		if len(matches) == 0 {
			return gorm.ErrRecordNotFound
		}
		if len(matches) != 1 {
			return ErrSMSReportAmbiguous
		}
		p := matches[0]
		// Final reports are immutable; duplicates and delayed temporary reports
		// cannot downgrade them. Unknown/reserved values do not prove delivery.
		if p.TPState == "delivered" || p.TPState == "delivery_failed" || p.TPState == "completed_unconfirmed" {
			return nil
		}
		if p.DischargeTime != nil && r.DischargeTime.Before(*p.DischargeTime) {
			return nil
		}
		state := smsTPState(r.Status)
		if err := tx.Model(&p).Updates(map[string]any{"tp_state": state, "tp_status": r.Status, "service_centre_time": r.ServiceCentreTime, "discharge_time": r.DischargeTime, "updated_at": r.At}).Error; err != nil {
			return err
		}
		return recomputeSMSDelivery(tx, p.MessageID, r.At)
	})
}

func recomputeSMSDelivery(tx *gorm.DB, id string, at time.Time) error {
	var d SMSDelivery
	if err := tx.Where("message_id = ?", id).First(&d).Error; err != nil {
		return err
	}
	var rows []SMSDeliveryPart
	if err := tx.Where("message_id = ?", id).Order("id asc").Find(&rows).Error; err != nil {
		return err
	}
	// Keep deployed non-unique indices compatible, counting each part only once.
	parts := map[int]SMSDeliveryPart{}
	for _, p := range rows {
		if p.PartNo >= 1 && p.PartNo <= d.PartsTotal {
			parts[p.PartNo] = p
		}
	}
	acks, accepted, unknown, delivered, completed, retrying := 0, 0, 0, 0, 0, 0
	failed, tpFailed, tpUnknown := false, false, false
	lastError := ""
	for _, p := range parts {
		switch p.State {
		case "acked":
			acks++
		case "accepted":
			accepted++
		case "unknown":
			unknown++
		case "failed", "timeout":
			failed = true
			lastError = p.ErrorText
		}
		switch p.TPState {
		case "delivered":
			delivered++
		case "completed_unconfirmed":
			completed++
		case "retrying":
			retrying++
		case "delivery_failed":
			tpFailed = true
		case "unknown":
			tpUnknown = true
		}
	}
	state := "pending"
	switch {
	case failed:
		state = "failed"
	case d.PartsTotal > 0 && acks == d.PartsTotal:
		state = "acked"
	case acks > 0:
		state = "partial_ack"
	case unknown > 0:
		state = "unknown"
	case d.PartsTotal > 0 && accepted == d.PartsTotal:
		state = "accepted"
	}
	deliveryState := "pending"
	switch {
	case tpFailed:
		deliveryState = "delivery_failed"
	case d.PartsTotal > 0 && delivered == d.PartsTotal:
		deliveryState = "delivered"
	case d.PartsTotal > 0 && completed+delivered == d.PartsTotal:
		deliveryState = "completed_unconfirmed"
	case delivered > 0:
		deliveryState = "partial_delivery"
	case retrying > 0:
		deliveryState = "retrying"
	case unknown > 0 || tpUnknown:
		deliveryState = "unknown"
	case failed:
		deliveryState = "failed"
	}
	return tx.Model(&d).Updates(map[string]any{"state": state, "acks": acks, "delivery_state": deliveryState, "delivered": delivered, "last_error": lastError, "updated_at": at}).Error
}
