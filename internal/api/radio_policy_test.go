package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/device"
)

func TestRadioPolicyFlightAPIRequiresHardwareConfirmation(t *testing.T) {
	// This existing stub acknowledges mode changes but always reports Online.
	w := &device.Worker{Backend: &ussdDeviceBackendStub{}, Config: config.DeviceConfig{AirplaneEnabled: true}}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, flight, err := setWorkerFlightMode(ctx, w, true); err == nil || flight {
		t.Fatal("flight API reported success while RF stayed on")
	}
}

func TestRadioPolicyRebootAPIAllowsProtectedCards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{backend.BackendAT, backend.BackendQMI, backend.BackendMBIM} {
		for _, state := range []struct {
			name              string
			vowifi, switching bool
		}{{name: "flight"}, {name: "vowifi", vowifi: true}, {name: "switching", vowifi: true, switching: true}} {
			t.Run(mode+"/"+state.name, func(t *testing.T) {
				p := device.NewPool(nil)
				t.Cleanup(func() { _ = p.Shutdown() })
				b := &ussdDeviceBackendStub{mode: mode}
				w := &device.Worker{ID: "dev", Pool: p, Backend: b,
					Config: config.DeviceConfig{AirplaneEnabled: true, VoWiFiEnabled: state.vowifi}}
				injectWorker(p, w)
				if state.switching {
					setNestedPrivateField(t, p, []string{"switchingDevices"}, map[string]bool{w.ID: true})
				}
				rec := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(rec)
				ctx.Params = gin.Params{{Key: "device_id", Value: w.ID}}
				ctx.Request = httptest.NewRequest(http.MethodPost, "/devices/dev/actions/reboot", nil)
				(&Server{pool: p}).handleDeviceMgmtReboot(ctx)
				if state.switching {
					if rec.Code != http.StatusConflict || b.rebooted {
						t.Fatalf("reboot interrupted profile switching: %d %s", rec.Code, rec.Body.String())
					}
					return
				}
				if rec.Code != http.StatusOK || !b.rebooted {
					t.Fatalf("manual reboot rejected: %d %s", rec.Code, rec.Body.String())
				}
				if !w.Config.AirplaneEnabled || w.Config.VoWiFiEnabled != state.vowifi {
					t.Fatal("manual reboot changed the card policy")
				}
			})
		}
	}
}
