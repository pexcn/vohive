package db

import (
	"testing"
	"time"
)

func TestSMSQueueIdempotencyCrashAndSingleClaim(t *testing.T) {
	openTestDB(t)
	a, err := EnqueueSMS("device", "sim", "key", "10086", "hello", "auto")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EnqueueSMS("device", "sim", "key", "10086", "hello", "auto")
	if err != nil || a.ID != b.ID {
		t.Fatalf("duplicate: %+v %v", b, err)
	}
	if _, err = EnqueueSMS("device", "sim", "key", "10086", "other", "auto"); err == nil {
		t.Fatal("reused key changed payload")
	}
	c, err := EnqueueSMS("device", "other-sim", "key", "10086", "hello", "auto")
	if err != nil || c.ID == a.ID {
		t.Fatal("subscription isolation")
	}
	if ok, err := ClaimSMSQueue(a.ID); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, _ := ClaimSMSQueue(a.ID); ok {
		t.Fatal("two consumers claimed one message")
	}
	if err = RecoverSMSQueue(); err != nil {
		t.Fatal(err)
	}
	a, err = GetSMSQueue(a.ID)
	if err != nil || a.State != "unknown" {
		t.Fatalf("crash outcome %+v %v", a, err)
	}
	due, err := DueSMSQueue(time.Now())
	if err != nil || len(due) != 1 || due[0].ID != c.ID {
		t.Fatalf("unknown was replayed: %+v %v", due, err)
	}
}
