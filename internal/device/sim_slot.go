package device

import (
	"context"
	"time"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/vohive/internal/db"
)

type SIMSlotState struct {
	Physical   *uint8 `json:"physical,omitempty"`
	Logical    *uint8 `json:"logical,omitempty"`
	EUICC      bool   `json:"euicc"`
	Source     string `json:"source,omitempty"`
	iccid      string
	observedAt time.Time
}

// SIMSlotState returns the last observation for the current card only.
func (w *Worker) SIMSlotState() *SIMSlotState {
	state := w.simSlot.Load()
	if state == nil || state.iccid != db.CanonicalICCID(w.CurrentICCID()) ||
		time.Since(state.observedAt) > 2*time.Minute || state.Physical == nil && state.Logical == nil {
		return nil
	}
	return state
}

func (w *Worker) refreshSIMSlot(ctx context.Context) {
	iccid := db.CanonicalICCID(w.CurrentICCID())
	if iccid == "" || w.Pool != nil && w.Pool.IsESIMSwitching(w.ID) {
		return
	}
	if state := w.simSlot.Load(); state != nil && state.iccid == iccid && time.Since(state.observedAt) < time.Minute {
		return
	}
	var read func(context.Context) (qmimanager.UIMReadiness, error)
	switch {
	case w.MBIMCore != nil:
		read = w.MBIMCore.GetSIMSlotReadiness
	case w.QMICore != nil:
		read = w.QMICore.GetUIMReadiness
	default:
		return
	}
	if !w.simSlotRefreshing.CompareAndSwap(false, true) {
		return
	}
	defer w.simSlotRefreshing.Store(false)
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	value, err := read(readCtx)
	if err != nil || db.CanonicalICCID(value.ICCID) != iccid ||
		db.CanonicalICCID(w.CurrentICCID()) != iccid || w.Pool != nil && w.Pool.IsESIMSwitching(w.ID) {
		return
	}
	state := &SIMSlotState{iccid: iccid, observedAt: time.Now(), Source: value.SlotSource, EUICC: value.ActiveSlotIsEUICC}
	if value.SlotKnown && value.ActiveSlot != 0 {
		state.Logical = &value.ActiveSlot
	}
	if value.PhysicalSlotKnown && value.ActivePhysicalSlot != 0 {
		state.Physical = &value.ActivePhysicalSlot
	}
	w.simSlot.Store(state)
}
