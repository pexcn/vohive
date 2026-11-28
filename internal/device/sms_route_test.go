package device

import (
	"testing"

	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/db"
)

func TestVoLTESMSNeverFallsBackBeforeIMSRegistration(t *testing.T) {
	p := NewPool(&config.Config{})
	defer p.cancel()
	w := &Worker{ID: "modem", Config: config.DeviceConfig{VoLTEEnabled: true}}
	for _, bearer := range []string{"", "ims", db.SMSBearerCellular} {
		if got, err := p.SMSBearer(w, bearer); err == nil || got != "" {
			t.Fatalf("routed pending VoLTE using %q: %q, %v", bearer, got, err)
		}
	}
	if err := w.SendSMS("123", "text"); err == nil {
		t.Fatal("direct SMS bypassed the VoLTE mode")
	}
	w.Config.VoLTEEnabled = false
	if got, err := p.SMSBearer(w, ""); err != nil || got != db.SMSBearerCellular {
		t.Fatalf("ordinary modem SMS = %q, %v", got, err)
	}
}
