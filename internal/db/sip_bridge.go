package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SIPToClient = "to_sip"
	SIPToModem  = "to_modem"
)

var ErrSIPBridgeQueueFull = errors.New("SIP SMS submission queue is full")

type SIPBridgeSMS struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Account        string    `gorm:"index" json:"account"`
	DeviceID       string    `json:"device_id"`
	ICCID          string    `json:"-"`
	Direction      string    `gorm:"index:idx_sip_bridge_work,priority:1" json:"direction"`
	State          string    `gorm:"index:idx_sip_bridge_work,priority:2" json:"state"`
	Peer           string    `json:"peer"`
	Content        string    `json:"-"`
	PhoneMessageID string    `json:"phone_message_id,omitempty"`
	Bearer         string    `json:"bearer,omitempty"`
	Attempts       int       `json:"attempts"`
	LastError      string    `json:"last_error,omitempty"`
	NextAttempt    time.Time `gorm:"index:idx_sip_bridge_work,priority:3" json:"next_attempt"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

var sipBridgeAccounts atomic.Pointer[map[string]string]

// ConfigureSIPBridgeAccounts associates inbound SMS with the configured device
// account at commit time. New accounts never inherit old device messages.
func ConfigureSIPBridgeAccounts(accounts map[string]string) {
	sipBridgeAccounts.Store(&accounts)
}

func enqueueSIPBridgeInbound(tx *gorm.DB, deviceID string, sms *SMS) error {
	accounts := sipBridgeAccounts.Load()
	if accounts == nil || (*accounts)[deviceID] == "" {
		return nil
	}
	job := SIPBridgeSMS{
		ID:      "sms-" + strconv.FormatUint(uint64(sms.ID), 10),
		Account: (*accounts)[deviceID], DeviceID: deviceID, ICCID: sms.ICCID,
		Direction: SIPToClient, State: "pending", Peer: sms.Sender,
		Content: sms.Content, Bearer: sms.Bearer, NextAttempt: time.Now(),
	}
	return tx.Create(&job).Error
}

func EnqueueSIPBridgeSubmission(ctx context.Context, job SIPBridgeSMS) error {
	job.Direction, job.State, job.NextAttempt = SIPToModem, "pending", time.Now()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&SIPBridgeSMS{}).Where("id = ?", job.ID).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			return nil
		}
		var pending int64
		if err := tx.Model(&SIPBridgeSMS{}).Where("account = ? AND direction = ? AND state IN ?", job.Account, SIPToModem, []string{"pending", "sending"}).
			Count(&pending).Error; err != nil {
			return err
		}
		if pending >= 256 {
			return ErrSIPBridgeQueueFull
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&job).Error
	})
}

func RecoverSIPBridgeQueue() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&SIPBridgeSMS{}).Where("state = ? AND direction = ?", "sending", SIPToClient).
			Update("state", "pending").Error; err != nil {
			return err
		}
		return tx.Model(&SIPBridgeSMS{}).Where("state = ? AND direction = ?", "sending", SIPToModem).
			Updates(map[string]any{"state": "unknown", "last_error": "服务在提交短信时中断，请先核对运营商投递状态"}).Error
	})
}

func NextSIPBridgeJob(ctx context.Context, direction string, accounts []string) (*SIPBridgeSMS, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	var job SIPBridgeSMS
	now := time.Now()
	conn := DB.WithContext(ctx)
	query := conn.Where("direction = ? AND state = ? AND next_attempt <= ? AND account IN ?", direction, "pending", now, accounts).
		Order("next_attempt, created_at, id").Limit(1).Find(&job)
	if query.Error != nil || query.RowsAffected == 0 {
		return nil, query.Error
	}
	result := conn.Model(&SIPBridgeSMS{}).Where("id = ? AND state = ?", job.ID, "pending").
		Updates(map[string]any{"state": "sending", "attempts": gorm.Expr("attempts + 1")})
	if result.Error != nil || result.RowsAffected != 1 {
		return nil, result.Error
	}
	job.Attempts++
	job.State = "sending"
	return &job, nil
}

func FinishSIPBridgeJob(ctx context.Context, job SIPBridgeSMS) error {
	result := DB.WithContext(ctx).Model(&SIPBridgeSMS{}).Where("id = ? AND state = ?", job.ID, "sending").
		Updates(map[string]any{"state": job.State, "last_error": job.LastError, "phone_message_id": job.PhoneMessageID,
			"bearer": job.Bearer, "next_attempt": job.NextAttempt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("SIP SMS job %q is no longer sending", job.ID)
	}
	return nil
}

func SIPBridgeJob(ctx context.Context, id string) (SIPBridgeSMS, error) {
	var result SIPBridgeSMS
	err := DB.WithContext(ctx).Where("id = ?", id).Take(&result).Error
	return result, err
}

func ListSIPBridgePending(ctx context.Context) ([]SIPBridgeSMS, error) {
	var rows []SIPBridgeSMS
	err := DB.WithContext(ctx).Where("state IN ?", []string{"pending", "sending", "failed", "unknown"}).
		Order("updated_at DESC, id DESC").Limit(100).Find(&rows).Error
	return rows, err
}

func RetrySIPBridgeDelivery(ctx context.Context, id string) (bool, error) {
	result := DB.WithContext(ctx).Model(&SIPBridgeSMS{}).Where(
		"id = ? AND direction = ? AND state IN ?", id, SIPToClient, []string{"pending", "failed"}).
		Updates(map[string]any{"state": "pending", "attempts": 0, "last_error": "", "next_attempt": time.Now()})
	return result.RowsAffected == 1, result.Error
}
