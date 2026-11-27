package db

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SIMHomeIdentity is scoped to the physical card or eSIM profile, not the
// visited operator or a transient IMSI advertised during roaming.
type SIMHomeIdentity struct {
	ICCID string `gorm:"column:iccid;primaryKey"`
	IMSI  string `gorm:"column:imsi"`
	MCC   string
	MNC   string
}

func RecordSIMHomeIdentity(iccid, imsi, mcc, mnc string) error {
	return recordSIMHomeIdentity(iccid, imsi, mcc, mnc, false)
}

// RecordPreferredSIMHomeIdentity can replace the first observation when a
// selected carrier rule explicitly requires the newly verified home PLMN.
func RecordPreferredSIMHomeIdentity(iccid, imsi, mcc, mnc string) error {
	return recordSIMHomeIdentity(iccid, imsi, mcc, mnc, true)
}

func recordSIMHomeIdentity(iccid, imsi, mcc, mnc string, preferred bool) error {
	iccid, imsi = CanonicalICCID(iccid), strings.TrimSpace(imsi)
	mcc, mnc = strings.TrimSpace(mcc), strings.TrimSpace(mnc)
	if DB == nil || iccid == "" || len(mcc) != 3 || (len(mnc) != 2 && len(mnc) != 3) ||
		!strings.HasPrefix(imsi, mcc+mnc) || strings.Trim(mcc+mnc, "0123456789") != "" {
		return nil
	}
	row := SIMHomeIdentity{ICCID: iccid, IMSI: imsi, MCC: mcc, MNC: mnc}
	conflict := clause.OnConflict{DoNothing: true}
	if preferred {
		conflict = clause.OnConflict{
			Columns:   []clause.Column{{Name: "iccid"}},
			DoUpdates: clause.AssignmentColumns([]string{"imsi", "mcc", "mnc"}),
			Where:     clause.Where{Exprs: []clause.Expression{gorm.Expr("imsi NOT LIKE ?", mcc+mnc+"%")}},
		}
	}
	return DB.Clauses(conflict).Create(&row).Error
}

func GetSIMHomeIdentity(iccid string) (SIMHomeIdentity, error) {
	var home SIMHomeIdentity
	iccid = CanonicalICCID(iccid)
	if DB == nil || iccid == "" {
		return home, nil
	}
	err := DB.Where("iccid = ?", iccid).Take(&home).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return home, nil
	}
	return home, err
}
