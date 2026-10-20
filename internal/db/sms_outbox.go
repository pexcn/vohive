package db

import (
	"errors"
	"gorm.io/gorm/clause"
	"time"
)

// Stable ID is the committed assembly ID. An acknowledged consumer can safely
// ignore repeats. External delivery without such an acknowledgement is at least
// once: a process can crash after delivery but before recording its result.
type SMSNotification struct {
	ID                              string `gorm:"primaryKey"`
	DeviceID, IMSI, Sender, Content string
	At                              time.Time
	Delivered                       bool `gorm:"index"`
	LeaseUntil                      time.Time
}

func PendingSMSNotifications(now time.Time) ([]SMSNotification, error) {
	var rows []SMSNotification
	if DB == nil {
		return rows, errors.New("SMS database unavailable")
	}
	err := DB.Where("delivered = ? AND lease_until <= ?", false, now).Order("at, id").Limit(32).Find(&rows).Error
	return rows, err
}
func ClaimSMSNotification(id string, now time.Time) (bool, error) {
	r := DB.Model(&SMSNotification{}).Where("id = ? AND delivered = ? AND lease_until <= ?", id, false, now).Update("lease_until", now.Add(2*time.Minute))
	return r.RowsAffected == 1, r.Error
}
func AcknowledgeSMSNotification(id string) error {
	return DB.Model(&SMSNotification{}).Where("id = ?", id).Update("delivered", true).Error
}

type SMSNotificationReceipt struct {
	NotificationID string `gorm:"primaryKey"`
	Consumer       string `gorm:"primaryKey"`
	At             time.Time
}

func SMSNotificationConsumerDone(id, consumer string) (bool, error) {
	if DB == nil {
		return false, errors.New("SMS database unavailable")
	}
	var n int64
	err := DB.Model(&SMSNotificationReceipt{}).Where("notification_id = ? AND consumer = ?", id, consumer).Count(&n).Error
	return n != 0, err
}
func RecordSMSNotificationReceipt(id, consumer string) error {
	if DB == nil {
		return errors.New("SMS database unavailable")
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&SMSNotificationReceipt{NotificationID: id, Consumer: consumer, At: time.Now()}).Error
}
