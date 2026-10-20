package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/iniwex5/vohive/internal/db"
)

type receiptChannel struct {
	name  string
	calls int
	fail  bool
}

func (c *receiptChannel) Name() string { return c.name }
func (c *receiptChannel) Send(string) error {
	c.calls++
	if c.fail {
		return errors.New("injected channel failure")
	}
	return nil
}
func (c *receiptChannel) RegisterCommand(string, CommandHandler) {}
func (c *receiptChannel) Start() error                           { return nil }
func (c *receiptChannel) Close() error                           { return nil }
func TestDurableSMSReceiptsAndWebhookEventID(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "notifications.db")); err != nil {
		t.Fatal(err)
	}
	success, failed := &receiptChannel{name: "success"}, &receiptChannel{name: "failure", fail: true}
	var eventIDs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		eventIDs = append(eventIDs, payload.EventID)
		w.WriteHeader(204)
	}))
	defer server.Close()
	webhook, err := NewWebhookChannel(webhookConfigForTest(server.URL, ""))
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{channels: []Channel{success, failed, webhook}}
	if err = manager.NotifySMSWithID(context.Background(), "stable-id", "dev", "10086", "hello", time.Now()); err == nil {
		t.Fatal("channel failure acknowledged")
	}
	failed.fail = false
	if err = manager.NotifySMSWithID(context.Background(), "stable-id", "dev", "10086", "hello", time.Now()); err != nil {
		t.Fatal(err)
	}
	if success.calls != 1 || failed.calls != 2 || len(eventIDs) != 1 || eventIDs[0] != "stable-id" {
		t.Fatalf("consumer receipts: %d %d %v", success.calls, failed.calls, eventIDs)
	}
	if err = manager.NotifySMSWithID(context.Background(), "stable-id", "dev", "10086", "hello", time.Now()); err != nil || failed.calls != 2 {
		t.Fatal("acknowledged event replayed", err)
	}
}
