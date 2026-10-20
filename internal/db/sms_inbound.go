package db

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// SMSInboundAssembly stores raw protocol fragments until the module decodes a
// complete message. History and its committed tombstone share one transaction.
type SMSInboundAssembly struct {
	ID        string `gorm:"primaryKey"`
	DeviceID  string `gorm:"index:idx_sms_inbound_owner"`
	IMSI      string `gorm:"index:idx_sms_inbound_owner"`
	GroupKey  string `gorm:"index"`
	Total     int
	Data      []byte
	Hashes    []byte
	FirstSCTS time.Time
	Committed bool
	CreatedAt time.Time
	ExpiresAt time.Time `gorm:"index"`
}

type SMSInboundFragment struct {
	Key                   string
	PartNo, Total         int
	TPDU                  []byte
	ServiceCentreTime, At time.Time
}

type SMSInboundSnapshot struct {
	ID        string
	Total     int
	Parts     [][]byte
	Committed bool
}

func inboundSnapshot(row SMSInboundAssembly) (SMSInboundSnapshot, error) {
	a := SMSInboundSnapshot{ID: row.ID, Total: row.Total, Committed: row.Committed}
	if !row.Committed {
		if err := json.Unmarshal(row.Data, &a.Parts); err != nil {
			return a, err
		}
		if len(a.Parts) != row.Total {
			return a, errors.New("corrupt SMS assembly")
		}
	}
	return a, nil
}

func SaveInboundSMSFragment(deviceID, imsi string, f SMSInboundFragment) (SMSInboundSnapshot, error) {
	var out SMSInboundSnapshot
	if DB == nil {
		return out, errors.New("SMS database unavailable")
	}
	if deviceID == "" || imsi == "" || len(f.Key) != 64 || f.Total < 1 || f.Total > 255 || f.PartNo < 1 || f.PartNo > f.Total || len(f.TPDU) == 0 || len(f.TPDU) > 256 {
		return out, errors.New("invalid inbound SMS fragment")
	}
	now := time.Now()
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at < ?", now).Delete(&SMSInboundAssembly{}).Error; err != nil {
			return err
		}
		var rows []SMSInboundAssembly
		if err := tx.Where("device_id = ? AND imsi = ? AND group_key = ?", deviceID, imsi, f.Key).Find(&rows).Error; err != nil {
			return err
		}
		var row SMSInboundAssembly
		for _, r := range rows {
			// Concatenation references can wrap. A new SCTS separated by ten minutes
			// starts a new generation; conflicting overlapping generations are rejected.
			delta := f.ServiceCentreTime.Sub(r.FirstSCTS)
			if delta < 0 {
				delta = -delta
			}
			if delta <= 10*time.Minute {
				if row.ID != "" {
					return errors.New("ambiguous inbound SMS generation")
				}
				row = r
			}
		}
		if row.ID == "" {
			var active, committed int64
			base := func() *gorm.DB {
				return tx.Model(&SMSInboundAssembly{}).Where("device_id = ? AND imsi = ?", deviceID, imsi)
			}
			if err := base().Where("committed = ?", false).Count(&active).Error; err != nil {
				return err
			}
			if err := base().Where("committed = ?", true).Count(&committed).Error; err != nil {
				return err
			}
			if active >= 256 {
				return errors.New("inbound SMS storage limit reached")
			}
			if committed >= 2048 {
				var oldest SMSInboundAssembly
				if err := base().Where("committed = ?", true).Order("created_at asc").First(&oldest).Error; err != nil {
					return err
				}
				if err := tx.Delete(&oldest).Error; err != nil {
					return err
				}
			}
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return err
			}
			data, _ := json.Marshal(make([][]byte, f.Total))
			row = SMSInboundAssembly{ID: hex.EncodeToString(id[:]), DeviceID: deviceID, IMSI: imsi, GroupKey: f.Key, Total: f.Total, Data: data, FirstSCTS: f.ServiceCentreTime, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		if row.Total != f.Total {
			return errors.New("SMS fragment total conflict")
		}
		if row.Committed {
			var hashes []string
			if err := json.Unmarshal(row.Hashes, &hashes); err != nil {
				return err
			}
			h := sha256.Sum256(f.TPDU)
			if len(hashes) != row.Total || hashes[f.PartNo-1] != hex.EncodeToString(h[:]) {
				return errors.New("SMS fragment conflicts with completed message")
			}
			out = SMSInboundSnapshot{ID: row.ID, Total: row.Total, Committed: true}
			return nil
		}
		a, err := inboundSnapshot(row)
		if err != nil {
			return err
		}
		old := a.Parts[f.PartNo-1]
		if old != nil && !bytes.Equal(old, f.TPDU) {
			return errors.New("SMS fragment sequence conflict")
		}
		a.Parts[f.PartNo-1] = append([]byte(nil), f.TPDU...)
		data, err := json.Marshal(a.Parts)
		if err != nil {
			return err
		}
		if err := tx.Model(&row).Update("data", data).Error; err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}

func LoadPendingInboundSMS(deviceID, imsi string) ([]SMSInboundSnapshot, error) {
	if DB == nil {
		return nil, errors.New("SMS database unavailable")
	}
	var rows []SMSInboundAssembly
	if err := DB.Where("device_id = ? AND imsi = ? AND committed = ? AND expires_at >= ?", deviceID, imsi, false, time.Now()).Order("created_at asc").Limit(256).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SMSInboundSnapshot, 0, len(rows))
	for _, row := range rows {
		a, err := inboundSnapshot(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func CommitInboundSMS(deviceID, imsi, id, sender, text string, at time.Time, suppress bool) (bool, error) {
	if DB == nil {
		return false, errors.New("SMS database unavailable")
	}
	// Resolve host metadata before taking the single-connection transaction.
	phone, _ := GetSIMCardPhoneNumberByIMSI(imsi)
	phone = normalizeSMSLocalPhone(imsi, 1, phone, sender, phone)
	sms := SMS{IMSI: imsi, ICCID: GetICCIDForIMSI(imsi), Peer: normalizeSMSPeer(1, sender, phone), LocalPhone: phone, Sender: sender, Recipient: phone, Content: text, Type: 1, Status: 0, Timestamp: at.Truncate(time.Second)}
	fresh := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var row SMSInboundAssembly
		if err := tx.Where("id = ? AND device_id = ? AND imsi = ?", id, deviceID, imsi).First(&row).Error; err != nil {
			return err
		}
		if row.Committed {
			return nil
		}
		a, err := inboundSnapshot(row)
		if err != nil {
			return err
		}
		hashes := make([]string, row.Total)
		for i, p := range a.Parts {
			if len(p) == 0 {
				return errors.New("cannot commit incomplete SMS")
			}
			h := sha256.Sum256(p)
			hashes[i] = hex.EncodeToString(h[:])
		}
		if !suppress {
			if err := tx.Create(&sms).Error; err != nil {
				return err
			}
			if sms.Peer != "" {
				if err := upsertSMSContactFromSMS(tx, &sms); err != nil {
					return err
				}
			}
		}
		data, err := json.Marshal(hashes)
		if err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"committed": true, "data": nil, "hashes": data, "expires_at": time.Now().Add(7 * 24 * time.Hour)}).Error; err != nil {
			return err
		}
		fresh = true
		return nil
	})
	return fresh, err
}
