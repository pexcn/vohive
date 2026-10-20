package db

import (
	"errors"
	"gorm.io/gorm"
	"testing"
	"time"
)

func mustReserveSMS(t *testing.T, id, imsi string, total, no, mr int, at time.Time) int {
	t.Helper()
	if no == 1 {
		if err := CreateSMSDelivery(id, imsi, "dev", "+15551234", "test", total, at); err != nil {
			t.Fatal(err)
		}
	}
	ref, err := ReserveSMSTPReference("dev", imsi, id, no, id+string(rune('0'+no)), mr, at)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestSMSReliabilityLayeredReportsAndLateSIP(t *testing.T) {
	openTestDB(t)
	at := time.Now().UTC().Truncate(time.Second)
	refs := []int{mustReserveSMS(t, "long", "sim-a", 2, 1, 7, at), mustReserveSMS(t, "long", "sim-a", 2, 2, 8, at)}
	check := func(state, delivery string, acks, delivered int) {
		t.Helper()
		s, err := GetSMSDeliveryStatus("long")
		if err != nil {
			t.Fatal(err)
		}
		if s.State != state || s.DeliveryState != delivery || s.Acks != acks || s.Delivered != delivered {
			t.Fatalf("status=%+v", s)
		}
	}
	if err := RecordSMSSIPResult("long", 1, 202, "accepted", at); err != nil {
		t.Fatal(err)
	}
	if err := RecordSMSRPReport("dev", "sim-a", "long1", 7, "acked", 0, at); err != nil {
		t.Fatal(err)
	}
	check("partial_ack", "pending", 1, 0)
	// RP acknowledgement and final delivery are separate. A report can beat SIP.
	r := SMSTPReport{Reference: refs[0], Recipient: "+15551234", Status: 0, ServiceCentreTime: at, DischargeTime: at.Add(time.Second), At: at.Add(2 * time.Second)}
	if err := RecordSMSTPReport("dev", "sim-a", r); err != nil {
		t.Fatal(err)
	}
	if err := RecordSMSSIPResult("long", 1, 0, "unknown", at); err != nil {
		t.Fatal(err)
	}
	check("partial_ack", "partial_delivery", 1, 1)
	if err := RecordSMSRPReport("dev", "sim-a", "long2", 8, "acked", 0, at); err != nil {
		t.Fatal(err)
	}
	check("acked", "partial_delivery", 2, 1)
	r.Reference = refs[1]
	r.Status = 0x21
	if err := RecordSMSTPReport("dev", "sim-a", r); err != nil {
		t.Fatal(err)
	}
	check("acked", "partial_delivery", 2, 1)
	r.Status = 0
	if err := RecordSMSTPReport("dev", "sim-a", r); err != nil {
		t.Fatal(err)
	}
	check("acked", "delivered", 2, 2)
	r.Status = 0x40
	if err := RecordSMSTPReport("dev", "sim-a", r); err != nil {
		t.Fatal(err)
	}
	if err := RecordSMSSIPResult("long", 2, 202, "accepted", at); err != nil {
		t.Fatal(err)
	}
	check("acked", "delivered", 2, 2)
}

func TestSMSReliabilitySubscriptionAmbiguityAndUnknown(t *testing.T) {
	openTestDB(t)
	at := time.Now().UTC().Truncate(time.Second)
	ref := mustReserveSMS(t, "first", "sim-a", 1, 1, 19, at)
	second := mustReserveSMS(t, "second", "sim-a", 1, 1, 19, at)
	if ref == second {
		t.Fatal("reused unresolved TP reference")
	}
	other := mustReserveSMS(t, "other", "sim-b", 1, 1, 19, at)
	if ref != other {
		t.Fatal("different SIM unnecessarily shares TP pool")
	}
	r := SMSTPReport{Reference: ref, Recipient: "+15551234", Status: 0, ServiceCentreTime: at, DischargeTime: at, At: at}
	if err := RecordSMSTPReport("dev", "sim-c", r); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-SIM match: %v", err)
	}
	r.Recipient = "+19999"
	if err := RecordSMSTPReport("dev", "sim-a", r); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-recipient match: %v", err)
	}
	r.Recipient = "+15551234"
	// Simulate a migrated/legacy reused reference: do not pick the newest row.
	if err := DB.Model(&SMSDeliveryPart{}).Where("message_id = ?", "second").Update("tp_mr", ref).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecordSMSTPReport("dev", "sim-a", r); !errors.Is(err, ErrSMSReportAmbiguous) {
		t.Fatalf("ambiguous match: %v", err)
	}
	if err := RecordSMSRPReport("dev", "sim-a", "", 19, "acked", 0, at); !errors.Is(err, ErrSMSReportAmbiguous) {
		t.Fatalf("ambiguous RP match: %v", err)
	}
	if err := RecordSMSSIPResult("first", 1, 0, "unknown", at); err != nil {
		t.Fatal(err)
	}
	s, err := GetSMSDeliveryStatus("first")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != "unknown" || s.DeliveryState != "unknown" {
		t.Fatalf("uncertain send=%+v", s)
	}
}

func TestSMSReliabilityFinalStatusClassification(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   string
	}{{0, "delivered"}, {1, "completed_unconfirmed"}, {2, "completed_unconfirmed"}, {3, "unknown"}, {0x20, "retrying"}, {0x3f, "retrying"}, {0x40, "delivery_failed"}, {0x7f, "delivery_failed"}, {0x80, "unknown"}} {
		if got := smsTPState(tc.status); got != tc.want {
			t.Fatalf("%x = %s", tc.status, got)
		}
	}
}

func TestSMSDeliveryLegacyNonUniqueIndexAndExpectedTotal(t *testing.T) {
	openTestDB(t)
	at := time.Now()
	if err := CreateSMSDelivery("legacy", "sim", "dev", "1", "long", 3, at); err != nil {
		t.Fatal(err)
	}
	if err := UpsertSMSDeliveryPart("legacy", 1, "call", 1, "pending", at); err != nil {
		t.Fatal(err)
	}
	if err := UpsertSMSDeliveryPart("legacy", 1, "call", 1, "acked", at); err != nil {
		t.Fatal(err)
	}
	// Existing databases can contain duplicate rows; count distinct part numbers.
	if err := DB.Create(&SMSDeliveryPart{MessageID: "legacy", PartNo: 1, State: "acked"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := RecomputeSMSDelivery("legacy", at); err != nil {
		t.Fatal(err)
	}
	s, err := GetSMSDeliveryStatus("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if s.Acks != 1 || s.State != "partial_ack" {
		t.Fatalf("premature acknowledgement=%+v", s)
	}
}

func TestSMSReliabilityDelayedReportAfterRestart(t *testing.T) {
	path := t.TempDir() + "/reports.db"
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second).Add(-48 * time.Hour)
	ref := mustReserveSMS(t, "before-restart", "sim", 1, 1, 77, at)
	if err := RecordSMSRPReport("dev", "sim", "before-restart1", 77, "acked", 0, at); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := DB.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	// Recent submissions must not consume the old TP-MR or an old SCTS report.
	newer := mustReserveSMS(t, "after-restart", "sim", 1, 1, 77, time.Now())
	if ref == newer {
		t.Fatal("reused unresolved reference after restart")
	}
	report := SMSTPReport{Reference: ref, Recipient: "+15551234", Status: 0, ServiceCentreTime: at, DischargeTime: time.Now(), At: time.Now()}
	if err := RecordSMSTPReport("dev", "sim", report); err != nil {
		t.Fatal(err)
	}
	s, err := GetSMSDeliveryStatus("before-restart")
	if err != nil {
		t.Fatal(err)
	}
	if s.DeliveryState != "delivered" || s.Acks != 1 {
		t.Fatalf("delayed report=%+v", s)
	}
	report.ServiceCentreTime = time.Now().Add(2 * time.Hour)
	if err := RecordSMSTPReport("dev", "sim", report); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("future report %v", err)
	}
}
