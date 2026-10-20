package device

import (
	"context"
	"errors"
	"time"

	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/logger"
	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
)

func (p *Pool) smsDurabilityLoop() {
	if err := db.RecoverSMSQueue(); err != nil {
		logger.Warn("SMS queue recovery failed", "err", err)
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-tick.C:
			p.deliverSMSNotifications()
			p.sendQueuedSMS()
		}
	}
}
func (p *Pool) SetSMSQueueLimiter(allow func() bool) {
	p.mu.Lock()
	p.smsQueueAllow = allow
	p.mu.Unlock()
}

func (p *Pool) sendQueuedSMS() {
	rows, err := db.DueSMSQueue(time.Now())
	if err != nil {
		return
	}
	for _, row := range rows {
		if p.ctx.Err() != nil {
			return
		}
		inst := p.voWiFiHost().Instance(row.DeviceID)
		if inst == nil || !inst.State().SMSReady {
			continue
		}
		service := inst.Service()
		owner, ok := service.(interface{ Subscription() messaging.Subscription })
		if !ok || owner.Subscription() != (messaging.Subscription{DeviceID: row.DeviceID, IMSI: row.IMSI}) {
			continue
		}
		p.mu.RLock()
		allow := p.smsQueueAllow
		p.mu.RUnlock()
		if allow == nil || !allow() {
			return
		}
		claimed, err := db.ClaimSMSQueue(row.ID)
		if err != nil || !claimed {
			continue
		}
		ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
		outcome, err := service.SendSMSWithOptions(ctx, row.Recipient, row.Content, messaging.SendOptions{Encoding: row.Encoding})
		cancel()
		state := "accepted"
		next := time.Now()
		if err != nil {
			state = "unknown"
			if !errors.Is(err, messaging.ErrSendOutcomeUnknown) && outcome.State == "failed" && outcome.DeliveryState == "failed" {
				state = "failed"
			}
			if errors.Is(err, messaging.ErrSendNotAttempted) && !errors.Is(err, messaging.ErrSendOutcomeUnknown) {
				state = "queued"
				if row.Attempts >= 4 {
					state = "failed"
				}
				next = next.Add(time.Second * time.Duration(1<<uint(row.Attempts+1)))
			}
		}
		if err = db.FinishSMSQueue(row.ID, outcome.MessageID, state, next); err != nil {
			logger.Warn("SMS queue result persistence failed", "queue_id", row.ID, "err", err)
		}
	}
}

// Optional acknowledged consumers receive a stable idempotency key. Existing
// notifier interfaces keep working with at-least-once crash recovery.
type durableSMSNotifier interface {
	NotifySMSWithID(context.Context, string, string, string, string, time.Time) error
}

func (p *Pool) deliverSMSNotifications() {
	p.smsOutboxMu.Lock()
	defer p.smsOutboxMu.Unlock()
	notifier := p.getNotifier()
	if notifier == nil {
		return
	}
	rows, err := db.PendingSMSNotifications(time.Now())
	if err != nil {
		return
	}
	for _, row := range rows {
		if p.ctx.Err() != nil {
			return
		}
		claimed, err := db.ClaimSMSNotification(row.ID, time.Now())
		if err != nil || !claimed {
			continue
		}
		if durable, ok := notifier.(durableSMSNotifier); ok {
			err = durable.NotifySMSWithID(p.ctx, row.ID, row.DeviceID, row.Sender, row.Content, row.At)
		} else if n, ok := notifier.(SMSSourceNotifier); ok {
			n.NotifySMSWithSource(row.DeviceID, row.Sender, row.Content, "VoWiFi", row.At)
		} else {
			notifier.NotifySMS(row.DeviceID, row.Sender, row.Content, row.At)
		}
		if err == nil {
			if err = db.AcknowledgeSMSNotification(row.ID); err != nil {
				logger.Warn("SMS notification acknowledgement failed", "event_id", row.ID, "err", err)
			}
		}
	}
}
