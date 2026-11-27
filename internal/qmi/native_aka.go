package qmicore

import (
	"bytes"
	"context"
	"errors"
	"time"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/qmi-go/pkg/qmi"
	"github.com/iniwex5/vohive/internal/apduarbiter"
	"github.com/iniwex5/vohive/internal/backend"
)

type nativeAKAState struct {
	iccid       string
	slot        uint8
	isimAID     []byte
	aidExpires  time.Time
	unsupported [2]bool
}

func (m *Manager) AuthenticateSIM(ctx context.Context, isim bool, challenge []byte) ([]byte, error) {
	if m == nil || m.qmiMgr == nil {
		return nil, backend.ErrNativeAKAUnavailable
	}
	lease, err := m.acquireAPDUTransportLease(ctx, 10*time.Second, "vowifi_aka", apduarbiter.APDUClassUSIMAKA, 0, apduarbiter.TransportScopeExclusive)
	if err != nil {
		return nil, err
	}
	if lease != nil {
		defer lease.Release()
	}

	card, err := m.qmiMgr.GetUIMReadiness(ctx)
	if err != nil || !card.CardPresent || !card.ProvisioningActive || (isim && !card.SlotKnown) {
		return nil, backend.ErrNativeAKAUnavailable
	}

	// The readiness snapshot can carry a cached identity from before a SIM refresh.
	iccid, err := m.qmiMgr.GetICCIDStrictLive(ctx)
	if err != nil || iccid == "" {
		return nil, backend.ErrNativeAKAUnavailable
	}
	app := 0
	if isim {
		app = 1
	}
	m.nativeAKAMu.Lock()
	defer m.nativeAKAMu.Unlock()
	state := &m.nativeAKA
	if state.iccid != iccid || state.slot != card.ActiveSlot {
		*state = nativeAKAState{iccid: iccid, slot: card.ActiveSlot}
	}
	if state.unsupported[app] {
		return nil, backend.ErrNativeAKAUnavailable
	}

	session, authContext, ok := nativeAKASession(card, isim)
	if !ok {
		return nil, backend.ErrNativeAKAUnavailable
	}
	var aid []byte
	if isim {
		if time.Now().After(state.aidExpires) {
			aid, err = m.qmiMgr.GetISIMAID(ctx)
			if err != nil || len(aid) <= len(isimApplicationPrefix) || !bytes.HasPrefix(aid, isimApplicationPrefix) {
				return nil, backend.ErrNativeAKAUnavailable
			}
			state.isimAID = aid
			state.aidExpires = time.Now().Add(30 * time.Second)
		}
		aid = state.isimAID
	}
	result, err := m.qmiMgr.UIMAuthenticateContext(ctx, session, aid, authContext, challenge)
	var unsupported *qmi.NotSupportedError
	if errors.As(err, &unsupported) || errors.Is(err, qmi.ErrServiceNotSupported) {
		state.unsupported[app] = true
		return nil, backend.ErrNativeAKAUnavailable
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("QMI UIM Authenticate returned no result")
	}
	sw1, sw2 := byte(0x90), byte(0x00)
	if result.HasCardResult {
		sw1, sw2 = result.SW1, result.SW2
	}
	return append(append([]byte(nil), result.Data...), sw1, sw2), nil
}

var isimApplicationPrefix = []byte{0xA0, 0, 0, 0, 0x87, 0x10, 0x04}

func nativeAKASession(card qmimanager.UIMReadiness, isim bool) (session, authContext uint8, ok bool) {
	if !isim {
		return qmi.UIMSessionTypePrimaryGWProvisioning, qmi.UIMAuthContext3GSecurity, true
	}
	switch card.ActiveSlot {
	case 1:
		session = qmi.UIMSessionTypeNonProvisioningSlot1
	case 2:
		session = qmi.UIMSessionTypeNonProvisioningSlot2
	case 3:
		session = qmi.UIMSessionTypeNonProvisioningSlot3
	default:
		return 0, 0, false
	}
	return session, qmi.UIMAuthContextIMSAKASecurity, true
}
