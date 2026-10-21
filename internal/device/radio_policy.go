package device

import (
	"context"
	"fmt"
	"time"

	"github.com/iniwex5/vohive/internal/backend"
	"github.com/iniwex5/vohive/internal/db"
)

// BeginRadioOnline covers operations that can re-enable RF, including modem
// resets. Policy updates/cancellation remain possible while a command drains;
// RFOff waits for that command so a late Online reply cannot undo flight mode.
func (w *Worker) BeginRadioOnline(ctx context.Context) (func(), error) {
	if w == nil {
		return nil, fmt.Errorf("设备不存在，禁止开启射频")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.lockRadio(ctx); err != nil {
		return nil, err
	}
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	restoreSwitch, _ := ctx.Value(switchSnapshotRegistrationKey{}).(bool)
	if err := ctx.Err(); err != nil {
		w.radioMu.Unlock()
		return nil, err
	}
	if w.cellularRegistrationBlockedLocked(restoreSwitch) ||
		(w.networkPolicyICCID != "" && db.CanonicalICCID(w.networkPolicyICCID) != db.CanonicalICCID(w.CurrentICCID())) {
		w.radioMu.Unlock()
		return nil, fmt.Errorf("卡策略未确认、要求飞行/VoWiFi 或设备正在切卡，禁止开启射频")
	}
	return w.radioMu.Unlock, nil
}

func (w *Worker) lockRadio(ctx context.Context) error {
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return waitForCondition(waitCtx, 10*time.Millisecond, w.radioMu.TryLock)
}

func (w *Worker) holdRadioPolicy() {
	w.registrationMu.Lock()
	w.radioPolicyPending = true
	w.registrationMu.Unlock()
}

func (w *Worker) radioMustStayOff() bool {
	w.registrationMu.Lock()
	defer w.registrationMu.Unlock()
	return w.radioPolicyPending || w.Config.VoWiFiEnabled || w.Config.AirplaneEnabled
}

// EnterAirplaneMode drains registration and data operations before confirming
// RFOff. The caller must first establish flight/VoWiFi/switch policy ownership.
func (w *Worker) EnterAirplaneMode(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("设备不存在")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := w.stopCellularRegistration(ctx); err != nil {
		return err
	}
	if nc := w.NetworkController(); nc != nil && nc.IsConnected() {
		_ = w.StopNetwork()
	}
	w.clearCachedIP()
	return w.ensureRadioOff(ctx)
}

// Query first to avoid a redundant low-power transition invalidating USIM
// channels. A successful command alone is not proof that RF is off.
func (w *Worker) ensureRadioOff(ctx context.Context) error {
	if w == nil || w.Backend == nil {
		return fmt.Errorf("设备后端未就绪，无法确认射频关闭")
	}
	if err := w.lockRadio(ctx); err != nil {
		return fmt.Errorf("等待射频操作退出失败: %w", err)
	}
	defer w.radioMu.Unlock()
	if mode, err := w.Backend.GetOperatingMode(ctx); err == nil && isFlightOperatingMode(mode) {
		return nil
	}
	if err := w.Backend.SetOperatingMode(ctx, backend.ModeRFOff); err != nil {
		return fmt.Errorf("关闭蜂窝射频失败: %w", err)
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := waitForCondition(verifyCtx, 100*time.Millisecond, func() bool {
		mode, err := w.Backend.GetOperatingMode(verifyCtx)
		return err == nil && isFlightOperatingMode(mode)
	})
	if err != nil {
		return fmt.Errorf("无法确认蜂窝射频已关闭: %w", err)
	}
	return nil
}
