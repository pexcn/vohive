package db

import (
	"gorm.io/gorm"
)

// SQLite index names are global. A legacy index on the wrong table prevents
// AutoMigrate from creating the expected index on the current table.
func repairSMSIndexes(tx *gorm.DB) error {
	var indexes []struct {
		Name    string
		TblName string
	}
	if err := tx.Raw("SELECT name, tbl_name FROM sqlite_master WHERE type = 'index'").Scan(&indexes).Error; err != nil {
		return err
	}
	owners := make(map[string]string, len(indexes))
	for _, index := range indexes {
		owners[index.Name] = index.TblName
	}
	for _, model := range []any{&SMS{}, &SMSContact{}, &SMSDelivery{}, &SMSDeliveryPart{}} {
		stmt := &gorm.Statement{DB: tx}
		if err := stmt.Parse(model); err != nil {
			return err
		}
		for _, index := range stmt.Schema.ParseIndexes() {
			if owner := owners[index.Name]; owner != "" && owner != stmt.Schema.Table {
				if err := tx.Migrator().DropIndex(model, index.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
