package db

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

// A sending job is never replayed after a crash. Its outcome is unknown until
// the referenced delivery record or an operator supplies stronger evidence.
type SMSQueueEntry struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	DeviceID    string    `gorm:"index" json:"device_id"`
	IMSI        string    `gorm:"index" json:"imsi"`
	Recipient   string    `json:"recipient"`
	Content     string    `json:"content"`
	Encoding    string    `json:"encoding"`
	State       string    `gorm:"index" json:"state"`
	MessageID   string    `json:"message_id"`
	Attempts    int       `json:"attempts"`
	NextAttempt time.Time `gorm:"index" json:"next_attempt"`
	ExpiresAt   time.Time `json:"expires_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func EnqueueSMS(device, imsi, key, to, text, encoding string) (SMSQueueEntry, error) {
	var row SMSQueueEntry
	if DB == nil {
		return row, errors.New("SMS database unavailable")
	}
	if device == "" || imsi == "" || key == "" || len(key) > 128 || to == "" || len(text) == 0 || len(text) > 65536 {
		return row, errors.New("invalid SMS queue request")
	}
	digest := sha256.Sum256([]byte(device + "\x00" + imsi + "\x00" + key))
	id := hex.EncodeToString(digest[:])
	now := time.Now()
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := tx.First(&row, "id = ?", id).Error
		if err == nil {
			if row.Recipient != to || row.Content != text || row.Encoding != encoding {
				return errors.New("idempotency key already has another payload")
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err = tx.Model(&SMSQueueEntry{}).Where("state IN ?", []string{"queued", "sending"}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 1024 {
			return errors.New("SMS queue full")
		}
		if err = tx.Model(&SMSQueueEntry{}).Where("device_id = ? AND imsi = ? AND state IN ?", device, imsi, []string{"queued", "sending"}).Count(&count).Error; err != nil {
			return err
		}
		if count >= 64 {
			return errors.New("subscription SMS queue full")
		}
		row = SMSQueueEntry{ID: id, DeviceID: device, IMSI: imsi, Recipient: to, Content: text, Encoding: encoding, State: "queued", NextAttempt: now, ExpiresAt: now.Add(24 * time.Hour), UpdatedAt: now}
		return tx.Create(&row).Error
	})
	return row, err
}
func RecoverSMSQueue() error {
	if DB == nil {
		return errors.New("SMS database unavailable")
	}
	return DB.Model(&SMSQueueEntry{}).Where("state = ?", "sending").Updates(map[string]any{"state": "unknown", "updated_at": time.Now()}).Error
}
func DueSMSQueue(now time.Time) ([]SMSQueueEntry, error) {
	var rows []SMSQueueEntry
	if DB == nil {
		return rows, errors.New("SMS database unavailable")
	}
	if err := DB.Model(&SMSQueueEntry{}).Where("state = ? AND expires_at <= ?", "queued", now).Update("state", "expired").Error; err != nil {
		return nil, err
	}
	err := DB.Where("state = ? AND next_attempt <= ? AND expires_at > ?", "queued", now, now).Order("next_attempt, id").Limit(16).Find(&rows).Error
	return rows, err
}
func ClaimSMSQueue(id string) (bool, error) {
	r := DB.Model(&SMSQueueEntry{}).Where("id = ? AND state = ? AND expires_at > ? AND next_attempt <= ?", id, "queued", time.Now(), time.Now()).Updates(map[string]any{"state": "sending", "attempts": gorm.Expr("attempts + 1"), "updated_at": time.Now()})
	return r.RowsAffected == 1, r.Error
}
func FinishSMSQueue(id, messageID, state string, next time.Time) error {
	if state != "queued" && state != "accepted" && state != "failed" && state != "unknown" {
		return errors.New("invalid queue result")
	}
	return DB.Model(&SMSQueueEntry{}).Where("id = ? AND state = ?", id, "sending").Updates(map[string]any{"state": state, "message_id": messageID, "next_attempt": next, "updated_at": time.Now()}).Error
}
func GetSMSQueue(id string) (SMSQueueEntry, error) {
	var row SMSQueueEntry
	if DB == nil {
		return row, errors.New("SMS database unavailable")
	}
	err := DB.First(&row, "id = ?", id).Error
	return row, err
}
