package device

import (
	"errors"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/internal/smsnotify"
	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
	"gorm.io/gorm"
)

type vowifiDeliveryStore struct{}

func (vowifiDeliveryStore) CreateSMSDelivery(messageID, imsi, deviceID, peer, content string, partsTotal int, at time.Time) error {
	return db.CreateSMSDelivery(messageID, imsi, deviceID, peer, content, partsTotal, at)
}

func (vowifiDeliveryStore) UpsertSMSDeliveryPart(messageID string, partNo int, callID string, rpMR int, state string, sentAt time.Time) error {
	return db.UpsertSMSDeliveryPart(messageID, partNo, callID, rpMR, state, sentAt)
}

func (vowifiDeliveryStore) MarkSMSDeliveryPartReport(inReplyTo, callID, deviceID string, rpMR int, state string, sipCode int, rpCause int, errText string, at time.Time) (messaging.DeliveryPartMatch, error) {
	part, err := db.MarkSMSDeliveryPartReport(inReplyTo, callID, deviceID, rpMR, state, sipCode, rpCause, errText, at)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return messaging.DeliveryPartMatch{}, messaging.ErrDeliveryNotFound
		}
		return messaging.DeliveryPartMatch{}, err
	}
	return messaging.DeliveryPartMatch{
		MessageID: part.MessageID,
		PartNo:    part.PartNo,
		State:     part.State,
	}, nil
}

func (vowifiDeliveryStore) RecomputeSMSDelivery(messageID string, at time.Time) error {
	return db.RecomputeSMSDelivery(messageID, at)
}

func (vowifiDeliveryStore) UpdateSMSDeliveryState(messageID, state, lastError string, acks int, at time.Time) error {
	return db.UpdateSMSDeliveryState(messageID, state, lastError, acks, at)
}

// GetVoWiFiSMSDeliveryStatus reads history independently of an online session.
func GetVoWiFiSMSDeliveryStatus(messageID string) (*messaging.DeliveryStatus, error) {
	return vowifiDeliveryStore{}.GetSMSDeliveryStatus(messageID)
}

func (vowifiDeliveryStore) GetSMSDeliveryStatus(messageID string) (*messaging.DeliveryStatus, error) {
	status, err := db.GetSMSDeliveryStatus(messageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, messaging.ErrDeliveryNotFound
		}
		return nil, err
	}
	out := &messaging.DeliveryStatus{
		DeliveryState: status.DeliveryState,
		Delivered:     status.Delivered,
		ICCID:         status.ICCID,
		MessageID:     status.MessageID,
		IMSI:          status.IMSI,
		DeviceID:      status.DeviceID,
		Peer:          status.Peer,
		Content:       status.Content,
		PartsTotal:    status.PartsTotal,
		Acks:          status.Acks,
		State:         status.State,
		LastError:     status.LastError,
		CreatedAt:     status.CreatedAt,
		UpdatedAt:     status.UpdatedAt,
		Parts:         make([]messaging.DeliveryPartStatus, 0, len(status.Parts)),
	}
	for _, p := range status.Parts {
		out.Parts = append(out.Parts, messaging.DeliveryPartStatus{
			TPMR: p.TPMR, TPState: p.TPState, TPStatus: p.TPStatus, ServiceCentreTime: p.ServiceCentreTime, DischargeTime: p.DischargeTime,
			PartNo:      p.PartNo,
			CallID:      p.CallID,
			InReplyTo:   p.InReplyTo,
			RPMR:        p.RPMR,
			State:       p.State,
			SIPCode:     p.SIPCode,
			RPCause:     p.RPCause,
			RPCauseText: messaging.RPCauseText(p.RPCause),
			ErrorText:   p.ErrorText,
			SentAt:      p.SentAt,
			ReportAt:    p.ReportAt,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	return out, nil
}

var _ messaging.ReliabilityStore = vowifiDeliveryStore{}
var _ messaging.InboundStore = vowifiDeliveryStore{}

func smsReportError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, db.ErrSMSReportAmbiguous) {
		return messaging.ErrDeliveryNotFound
	}
	return err
}
func (vowifiDeliveryStore) ReserveSMSPart(o messaging.Subscription, id string, no int, callID string, rpMR int, at time.Time) (int, error) {
	return db.ReserveSMSTPReference(o.DeviceID, o.IMSI, id, no, callID, rpMR, at)
}
func (vowifiDeliveryStore) RecordSMSSIPResult(id string, no, code int, state string, at time.Time) error {
	return db.RecordSMSSIPResult(id, no, code, state, at)
}
func (vowifiDeliveryStore) RecordSMSRPReport(o messaging.Subscription, callID string, mr int, state string, cause int, at time.Time) error {
	return smsReportError(db.RecordSMSRPReport(o.DeviceID, o.IMSI, callID, mr, state, cause, at))
}
func (vowifiDeliveryStore) RecordSMSTPReport(o messaging.Subscription, r messaging.TPReport) error {
	return smsReportError(db.RecordSMSTPReport(o.DeviceID, o.IMSI, db.SMSTPReport{Reference: r.Reference, Recipient: r.Recipient, Status: r.Status, ServiceCentreTime: r.ServiceCentreTime, DischargeTime: r.DischargeTime, At: r.At}))
}
func (vowifiDeliveryStore) SaveInboundFragment(o messaging.Subscription, f messaging.InboundFragment) (messaging.InboundAssembly, error) {
	a, err := db.SaveInboundSMSFragment(o.DeviceID, o.IMSI, db.SMSInboundFragment{Key: f.Key, PartNo: f.PartNo, Total: f.Total, TPDU: f.TPDU, ServiceCentreTime: f.ServiceCentreTime, At: f.At})
	return messaging.InboundAssembly{ID: a.ID, Total: a.Total, Parts: a.Parts, Committed: a.Committed}, err
}
func (vowifiDeliveryStore) LoadPendingInboundSMS(o messaging.Subscription) ([]messaging.InboundAssembly, error) {
	rows, err := db.LoadPendingInboundSMS(o.DeviceID, o.IMSI)
	out := make([]messaging.InboundAssembly, 0, len(rows))
	for _, a := range rows {
		out = append(out, messaging.InboundAssembly{ID: a.ID, Total: a.Total, Parts: a.Parts, Committed: a.Committed})
	}
	return out, err
}
func (vowifiDeliveryStore) CommitInboundSMS(o messaging.Subscription, id, sender, text string, at time.Time) (bool, error) {
	return db.CommitInboundSMS(o.DeviceID, o.IMSI, id, sender, text, at, smsnotify.ShouldSuppressReceivedSMS(text))
}
