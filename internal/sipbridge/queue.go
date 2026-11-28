package sipbridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vohive/internal/db"
	"github.com/iniwex5/vohive/pkg/logger"
)

func (b *Bridge) runQueue(direction string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
		}
		accounts := b.allAccounts
		if direction == db.SIPToClient {
			accounts = make([]string, 0, len(b.allAccounts))
			for _, account := range b.allAccounts {
				if _, ok := b.currentContact(account); ok {
					accounts = append(accounts, account)
				}
			}
		}
		job, err := db.NextSIPBridgeJob(b.ctx, direction, accounts)
		if err != nil {
			if b.ctx.Err() == nil {
				logger.Warn("读取 SIP 短信任务失败", "err", err)
			}
			continue
		}
		if job != nil {
			b.runJob(job)
		}
	}
}

func (b *Bridge) runJob(job *db.SIPBridgeSMS) {
	account := b.accounts[job.Account]
	switch {
	case account.DeviceID != job.DeviceID:
		job.State, job.LastError = "failed", "账号绑定的设备已变化"
	case job.Direction == db.SIPToClient:
		if err := b.deliverToClient(*job); err == nil {
			job.State, job.LastError = "delivered", ""
		} else {
			job.LastError, job.State = err.Error(), "failed"
			if job.Attempts < 5 {
				job.State = "pending"
				job.NextAttempt = time.Now().Add(time.Duration(1<<(job.Attempts-1)) * 5 * time.Second)
			}
		}
	default:
		ctx, cancel := context.WithTimeout(b.ctx, 45*time.Second)
		id, bearer, state, err := b.callbacks.Send(ctx, *job, account.Bearer)
		cancel()
		job.PhoneMessageID, job.Bearer, job.State, job.LastError = id, bearer, state, ""
		if err != nil {
			job.LastError = err.Error()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.FinishSIPBridgeJob(ctx, *job); err != nil {
		logger.Warn("更新 SIP 短信任务失败", "id", job.ID, "err", err)
	}
}

func (b *Bridge) deliverToClient(job db.SIPBridgeSMS) error {
	registered, ok := b.currentContact(job.Account)
	if !ok {
		return fmt.Errorf("SIP 账号未注册")
	}
	req := sip.NewRequest(sip.MESSAGE, registered.uri)
	req.SetTransport("UDP")
	// Send only to the observed, authenticated UDP peer. A Contact header
	// cannot instruct the bridge to send the user's SMS to a third party.
	req.SetDestination(registered.source)
	fromUser := job.Peer
	if !phoneNumber.MatchString(fromUser) && !sipUser.MatchString(fromUser) {
		fromUser = "sms"
	}
	from := &sip.FromHeader{Address: sip.Uri{Scheme: "sip", User: fromUser, Host: b.advertise}}
	from.Params.Add("tag", sip.GenerateTagN(16))
	req.AppendHeader(from)
	callID := sip.CallIDHeader(job.ID + "@vohive")
	req.AppendHeader(&callID)
	req.AppendHeader(sip.NewHeader("Content-Type", "text/plain; charset=utf-8"))
	req.AppendHeader(sip.NewHeader("X-VoHive-SMS-ID", job.ID))
	if fromUser != job.Peer && !strings.ContainsAny(job.Peer, "\x00\r\n") {
		req.AppendHeader(sip.NewHeader("X-VoHive-SMS-From", job.Peer))
	}
	req.SetBody([]byte(job.Content))
	ctx, cancel := context.WithTimeout(b.ctx, 8*time.Second)
	defer cancel()
	res, err := b.client.Do(ctx, req)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("SIP 客户端返回 %d", res.StatusCode)
	}
	return nil
}
