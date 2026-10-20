package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/db"
)

func TestSMSDeliveryQueryWithoutRunningSession(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "delivery.db")); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	if err := db.CreateSMSDelivery("persisted", "sim", "dev", "1234", "test", 1, at); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ReserveSMSTPReference("dev", "sim", "persisted", 1, "submit", 1, at); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSMSSIPResult("persisted", 1, 202, "accepted", at); err != nil {
		t.Fatal(err)
	}
	server := &Server{} // no worker, modem, or active runtime is needed for history
	request := func(id string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/sms/delivery/"+id, nil)
		ctx.Params = gin.Params{{Key: "message_id", Value: id}}
		server.handleSMSDelivery(ctx)
		return rec
	}
	rec := request("persisted")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body)
	}
	var result struct {
		Delivery db.SMSDeliveryStatus `json:"delivery"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Delivery.State != "accepted" || result.Delivery.DeliveryState != "pending" || result.Delivery.Parts[0].TPMR == nil {
		t.Fatalf("delivery=%+v", result.Delivery)
	}
	if rec = request("missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", rec.Code)
	}
}
