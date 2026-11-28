package db

import (
	"path/filepath"
	"testing"
)

func TestCheckSchema(t *testing.T) {
	if err := Init(filepath.Join(t.TempDir(), "vohive.db")); err != nil {
		t.Fatal(err)
	}
	if !DB.Migrator().HasTable(&Device{}) {
		t.Fatal("device table is missing from a fresh database")
	}
}
