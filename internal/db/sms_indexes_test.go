package db

import (
	"path/filepath"
	"testing"
)

func TestSMSMigrationReassignsIndexWithWrongOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DROP INDEX idx_sms_imsi_ts",
		"CREATE INDEX idx_sms_imsi_ts ON sms_delivery(imsi)",
	} {
		if err := DB.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if previous, err := DB.DB(); err == nil {
		_ = previous.Close()
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	var table string
	const lookup = "SELECT tbl_name FROM sqlite_master WHERE name = 'idx_sms_imsi_ts'"
	if err := DB.Raw(lookup).Scan(&table).Error; err != nil || table != "sms" {
		t.Fatalf("migrated index owner = %q, err = %v", table, err)
	}
}
