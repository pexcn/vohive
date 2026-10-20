package db

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSMSInboundRestartIsolationConflictAndAtomicCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	f := SMSInboundFragment{Key: strings.Repeat("a", 64), PartNo: 2, Total: 2, TPDU: []byte{2, 3}, ServiceCentreTime: at, At: at}
	a, err := SaveInboundSMSFragment("dev", "sim-old", f)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Parts) != 2 || a.Parts[0] != nil {
		t.Fatalf("snapshot=%+v", a)
	}
	// Simulate process restart by closing and reopening the actual database.
	sqlDB, _ := DB.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	rows, err := LoadPendingInboundSMS("dev", "sim-old")
	if err != nil || len(rows) != 1 {
		t.Fatalf("restored=%+v %v", rows, err)
	}
	other, err := LoadPendingInboundSMS("dev", "sim-new")
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-SIM restore=%+v %v", other, err)
	}
	if fresh, err := CommitInboundSMS("dev", "sim-old", a.ID, "10086", "long", at, false); err == nil || fresh {
		t.Fatal("committed incomplete SMS")
	}
	f.PartNo = 1
	f.TPDU = []byte{1, 2}
	a, err = SaveInboundSMSFragment("dev", "sim-old", f)
	if err != nil {
		t.Fatal(err)
	}
	f.TPDU = []byte{9, 9}
	if _, err := SaveInboundSMSFragment("dev", "sim-old", f); err == nil {
		t.Fatal("overwrote conflicting fragment")
	}
	// Force failure after assembly exists: both history and tombstone roll back.
	if err := DB.Exec("CREATE TRIGGER reject_inbound BEFORE INSERT ON sms BEGIN SELECT RAISE(ABORT, 'injected'); END").Error; err != nil {
		t.Fatal(err)
	}
	if fresh, err := CommitInboundSMS("dev", "sim-old", a.ID, "10086", "long", at, false); err == nil || fresh {
		t.Fatal("commit did not fail atomically")
	}
	rows, err = LoadPendingInboundSMS("dev", "sim-old")
	if err != nil || len(rows) != 1 {
		t.Fatalf("lost assembly on failure: %+v %v", rows, err)
	}
	if err := DB.Exec("DROP TRIGGER reject_inbound").Error; err != nil {
		t.Fatal(err)
	}
	fresh, err := CommitInboundSMS("dev", "sim-old", a.ID, "10086", "long", at, false)
	if err != nil || !fresh {
		t.Fatalf("commit %v %v", fresh, err)
	}
	fresh, err = CommitInboundSMS("dev", "sim-old", a.ID, "10086", "long", at, false)
	if err != nil || fresh {
		t.Fatalf("duplicate commit %v %v", fresh, err)
	}
	f.TPDU = []byte{1, 2}
	duplicate, err := SaveInboundSMSFragment("dev", "sim-old", f)
	if err != nil || !duplicate.Committed {
		t.Fatalf("duplicate fragment %+v %v", duplicate, err)
	}
	var sms []SMS
	if err := DB.Find(&sms).Error; err != nil {
		t.Fatal(err)
	}
	if len(sms) != 1 || sms[0].IMSI != "sim-old" || sms[0].Content != "long" {
		t.Fatalf("history %+v", sms)
	}
	// Same reference on another subscription is independent.
	otherAssembly, err := SaveInboundSMSFragment("dev", "sim-new", f)
	if err != nil || otherAssembly.ID == a.ID {
		t.Fatalf("subscription isolation %+v %v", otherAssembly, err)
	}
}

func TestSMSInboundGenerationAndResourceLimit(t *testing.T) {
	openTestDB(t)
	at := time.Now().UTC().Truncate(time.Second)
	f := SMSInboundFragment{Key: strings.Repeat("b", 64), PartNo: 1, Total: 2, TPDU: []byte{1}, ServiceCentreTime: at}
	a, err := SaveInboundSMSFragment("dev", "sim", f)
	if err != nil {
		t.Fatal(err)
	}
	f.ServiceCentreTime = at.Add(20 * time.Minute)
	b, err := SaveInboundSMSFragment("dev", "sim", f)
	if err != nil || a.ID == b.ID {
		t.Fatalf("reference reuse=%+v %v", b, err)
	}
	// These overlapping generations cannot be safely distinguished.
	f.ServiceCentreTime = at.Add(10 * time.Minute)
	if _, err := SaveInboundSMSFragment("dev", "sim", f); err == nil {
		t.Fatal("mixed ambiguous generations")
	}
	if err := DB.Model(&SMSInboundAssembly{}).Where("id = ?", a.ID).Update("expires_at", at.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := LoadPendingInboundSMS("dev", "sim")
	if err != nil || len(rows) != 1 {
		t.Fatalf("expiry %+v %v", rows, err)
	}
	f.Total = 256
	if _, err := SaveInboundSMSFragment("dev", "sim", f); err == nil {
		t.Fatal("unbounded assembly accepted")
	}
	f.Total = 2
	f.TPDU = make([]byte, 257)
	if _, err := SaveInboundSMSFragment("dev", "sim", f); err == nil {
		t.Fatal("oversized TPDU accepted")
	}
}
