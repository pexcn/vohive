package mbimcore

import (
	"context"
	"testing"
	"time"

	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/iniwex5/vohive/pkg/mbim"
)

func TestManagerGetUIMReadinessUsesSubscriberReadyAndQMICardStatus(t *testing.T) {
	sawQMI := false
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{
				{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus, mbim.CIDBasicConnectDeviceServiceSubscribeList}},
				{Svc: mbim.UUIDMSBasicConnectExtensions, CIDs: []uint32{mbim.CIDMSBasicConnectExtVersion}},
				{Svc: mbim.UUIDQMI, CIDs: []uint32{mbim.CIDQMIMsg}},
			})
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServiceSubscribeList:
			return testCommandDone(h.tx, svc, cid, nil), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectSubscriberReadyStatus:
			info := mbim.TestSubscriberReadyInfo(1, "460001234567890", "89860012345678901234")
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDMSBasicConnectExtensions) && cid == mbim.CIDMSBasicConnectExtVersion:
			return mbim.BuildCommandDoneForTest(h.tx, svc, cid, []byte{0x00, 0x01, 0x00, 0x02}), true
		case svc.Equal(mbim.UUIDQMI) && cid == mbim.CIDQMIMsg:
			sawQMI = true
			payload := w[48:]
			if len(payload) >= 10 && payload[4] == 0x00 && mbim.ReadU16ForTest(payload[8:10]) == 0x0022 {
				return testCommandDone(h.tx, svc, cid, mbim.TestQMIAllocateUIMClientResp(0x21)), true
			}
			if len(payload) >= 10 && payload[4] == 0x00 && mbim.ReadU16ForTest(payload[8:10]) == 0x0023 {
				return testCommandDone(h.tx, svc, cid, mbim.TestQMIRelClientResp()), true
			}
			if len(payload) >= 11 && payload[4] == 0x0B && mbim.ReadU16ForTest(payload[9:11]) == 0x002F {
				return testCommandDone(h.tx, svc, cid, mbim.TestQMIUIMGetCardStatusResp(1, 0, true)), true
			}
			t.Fatalf("unexpected QMI payload: % X", payload)
			return nil, false
		default:
			return testCommandDone(h.tx, svc, cid, nil), true
		}
	})
	m := New("/dev/cdc-wdm0", "direct")
	if err := m.openWithTransport(context.Background(), ft); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer m.Close()
	if !m.qmiReadUsable() {
		t.Fatal("qmiReadUsable=false want true")
	}

	got, err := m.GetUIMReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetUIMReadiness: %v", err)
	}
	if !got.TransportReady || !got.ControlReady || !got.UIMReady {
		t.Fatalf("readiness flags = %+v", got)
	}
	if !sawQMI {
		t.Fatal("expected QMI card-status query")
	}
	if got.ActiveSlot != 1 || !got.SlotKnown || got.Reason != qmimanager.UIMReadinessReady {
		t.Fatalf("slot/ready = %+v", got)
	}
	if got.ICCID != "89860012345678901234" || got.IMSI != "460001234567890" {
		t.Fatalf("identities = %+v", got)
	}
}

func TestManagerGetUIMReadinessTreatsSubscriberTimeoutAsControlUnavailable(t *testing.T) {
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{
				{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus, mbim.CIDBasicConnectDeviceServiceSubscribeList}},
			})
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServiceSubscribeList:
			return testCommandDone(h.tx, svc, cid, nil), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectSubscriberReadyStatus:
			return nil, false
		default:
			return testCommandDone(h.tx, svc, cid, nil), true
		}
	})
	m := New("/dev/cdc-wdm0", "direct")
	if err := m.openWithTransport(context.Background(), ft); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := m.GetUIMReadiness(ctx)
	if err == nil {
		t.Fatal("expected readiness error")
	}
	if !got.TransportReady || got.ControlReady || got.Reason != qmimanager.UIMReadinessControlUnavailable {
		t.Fatalf("readiness = %+v", got)
	}
}

func TestManagerGetUIMReadinessReportsIdentityEmpty(t *testing.T) {
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus}}})
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectSubscriberReadyStatus:
			info := mbim.TestSubscriberReadyInfo(1, "", "")
			return testCommandDone(h.tx, svc, cid, info), true
		}
		return testCommandDone(h.tx, svc, cid, nil), true
	})
	m := New("/dev/cdc-wdm0", "direct")
	_ = m.openWithTransport(context.Background(), ft)
	defer m.Close()

	got, err := m.GetUIMReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetUIMReadiness: %v", err)
	}
	if got.Reason != qmimanager.UIMReadinessIdentityEmpty || !got.UIMReady {
		t.Fatalf("readiness = %+v, want IdentityEmpty and UIMReady", got)
	}
}

func TestManagerGetUIMReadinessReportsCardAbsent(t *testing.T) {
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus}}})
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectSubscriberReadyStatus:
			info := mbim.TestSubscriberReadyInfo(2, "", "") // 2 = NotInserted
			return testCommandDone(h.tx, svc, cid, info), true
		}
		return testCommandDone(h.tx, svc, cid, nil), true
	})
	m := New("/dev/cdc-wdm0", "direct")
	_ = m.openWithTransport(context.Background(), ft)
	defer m.Close()

	got, err := m.GetUIMReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetUIMReadiness: %v", err)
	}
	if got.Reason != qmimanager.UIMReadinessCardAbsent || got.CardPresent {
		t.Fatalf("readiness = %+v, want CardAbsent and not CardPresent", got)
	}
}

func TestManagerGetUIMReadinessReportsTransportFatal(t *testing.T) {
	// A fake transport that returns an error when reading
	// Wait, we can test that by returning a fatal error directly in SubscriberReady mock,
	// or overriding transport read loop.
	// Since we can just return a fatal error from MBIM transport:
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		if svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices {
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus}}})
			return testCommandDone(h.tx, svc, cid, info), true
		}
		if svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServiceSubscribeList {
			return testCommandDone(h.tx, svc, cid, nil), true
		}
		// Don't reply to SubscriberReadyStatus, letting it timeout... wait, timeout is ControlUnavailable.
		// To trigger TransportFatal, we need the transport to fail.
		// FakeTransport Close doesn't abort ongoing requests. We will close the transport manually.
		return nil, false
	})
	m := New("/dev/cdc-wdm0", "direct")
	_ = m.openWithTransport(context.Background(), ft)
	// Immediately close the transport so SubscriberReady fails with transport fatal.
	m.Close()

	got, err := m.GetUIMReadiness(context.Background())
	if err == nil {
		t.Fatal("expected error on closed transport")
	}
	if got.Reason != qmimanager.UIMReadinessTransportFatal || got.TransportReady {
		t.Fatalf("readiness = %+v, want TransportFatal and not TransportReady", got)
	}
}

func TestManagerGetUIMReadinessKeepsReadinessWhenSlotQueryUnavailable(t *testing.T) {
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc := testMBIMService(w)
		cid := testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			info := mbim.TestDeviceServicesInfo([]struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{
				{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectSubscriberReadyStatus, mbim.CIDBasicConnectDeviceServiceSubscribeList}},
				{Svc: mbim.UUIDQMI, CIDs: []uint32{mbim.CIDQMIMsg}},
			})
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServiceSubscribeList:
			return testCommandDone(h.tx, svc, cid, nil), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectSubscriberReadyStatus:
			info := mbim.TestSubscriberReadyInfo(1, "460001234567890", "89860012345678901234")
			return testCommandDone(h.tx, svc, cid, info), true
		case svc.Equal(mbim.UUIDQMI) && cid == mbim.CIDQMIMsg:
			payload := w[48:]
			if len(payload) >= 10 && payload[4] == 0x00 && mbim.ReadU16ForTest(payload[8:10]) == 0x0022 {
				return testCommandDone(h.tx, svc, cid, mbim.TestQMIAllocateUIMClientResp(0x21)), true
			}
			if len(payload) >= 10 && payload[4] == 0x00 && mbim.ReadU16ForTest(payload[8:10]) == 0x0023 {
				return testCommandDone(h.tx, svc, cid, mbim.TestQMIRelClientResp()), true
			}
			if len(payload) >= 11 && payload[4] == 0x0B && mbim.ReadU16ForTest(payload[9:11]) == 0x002F {
				return nil, false
			}
			t.Fatalf("unexpected QMI payload: % X", payload)
			return nil, false
		default:
			return testCommandDone(h.tx, svc, cid, nil), true
		}
	})
	m := New("/dev/cdc-wdm0", "direct")
	if err := m.openWithTransport(context.Background(), ft); err != nil {
		t.Fatalf("open: %v", err)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	got, err := m.GetUIMReadiness(ctx)
	if err != nil {
		t.Fatalf("GetUIMReadiness: %v", err)
	}
	if !got.TransportReady || !got.ControlReady || !got.UIMReady {
		t.Fatalf("readiness flags = %+v", got)
	}
	if got.SlotKnown || got.ActiveSlot != 0 || got.Err != nil {
		t.Fatalf("slot degradation = %+v", got)
	}
}

func newSIMPowerTestManager(t *testing.T, qmiAvailable, failPower bool) (*Manager, *int) {
	t.Helper()
	radioCalls := 0
	ft := mbim.NewFakeTransport(func(w []byte) ([]byte, bool) {
		h := testMBIMHeader(w)
		if h.typ == mbim.MessageTypeOpen {
			return testOpenDone(h.tx), true
		}
		if h.typ != mbim.MessageTypeCommand {
			return nil, false
		}
		svc, cid := testMBIMService(w), testLe.Uint32(w[36:])
		switch {
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectRadioState:
			radioCalls++
			return testCommandDone(h.tx, svc, cid, nil), true
		case svc.Equal(mbim.UUIDBasicConnect) && cid == mbim.CIDBasicConnectDeviceServices:
			services := []struct {
				Svc  mbim.UUID
				CIDs []uint32
			}{
				{Svc: mbim.UUIDBasicConnect, CIDs: []uint32{mbim.CIDBasicConnectRadioState, mbim.CIDBasicConnectDeviceServiceSubscribeList}},
				{Svc: mbim.UUIDQMI, CIDs: []uint32{mbim.CIDQMIMsg}},
			}
			if !qmiAvailable {
				services = services[:1]
			}
			return testCommandDone(h.tx, svc, cid, mbim.TestDeviceServicesInfo(services)), true
		case svc.Equal(mbim.UUIDQMI) && cid == mbim.CIDQMIMsg:
			payload := w[48:]
			if len(payload) >= 10 && payload[4] == 0x00 {
				switch mbim.ReadU16ForTest(payload[8:10]) {
				case 0x0022:
					return testCommandDone(h.tx, svc, cid, mbim.TestQMIAllocateUIMClientResp(0x21)), true
				case 0x0023:
					return testCommandDone(h.tx, svc, cid, mbim.TestQMIRelClientResp()), true
				}
			}
			if len(payload) >= 11 && payload[4] == 0x0B {
				msg := mbim.ReadU16ForTest(payload[9:11])
				if msg == 0x0030 || msg == 0x0031 {
					if failPower {
						return testCommandDone(h.tx, svc, cid, mbim.TestQMIReadErrorResp(msg, 0x0001)), true
					}
					return testCommandDone(h.tx, svc, cid, mbim.TestQMIResultSuccessResp(0x0B, 0x21, 2, msg, nil)), true
				}
			}
			t.Errorf("unexpected QMI payload: % X", payload)
		}
		return testCommandDone(h.tx, svc, cid, nil), true
	})
	m := New("/dev/test", "direct")
	if err := m.openWithTransport(t.Context(), ft); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, &radioCalls
}

func TestManagerSIMPowerNeverEnablesRadio(t *testing.T) {
	for _, tt := range []struct {
		name               string
		qmi, powerOn, fail bool
	}{
		{name: "off", qmi: true}, {name: "on", qmi: true, powerOn: true},
		{name: "failed_on", qmi: true, powerOn: true, fail: true},
		{name: "unsupported_off"}, {name: "unsupported_on", powerOn: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, radioCalls := newSIMPowerTestManager(t, tt.qmi, tt.fail)
			var err error
			if tt.powerOn {
				err = m.UIMPowerOnSIM(t.Context(), 1)
			} else {
				err = m.UIMPowerOffSIM(t.Context(), 1)
			}
			if (err != nil) != (!tt.qmi || tt.fail) || *radioCalls != 0 {
				t.Fatalf("SIM power result: err=%v, radio commands=%d", err, *radioCalls)
			}
		})
	}
}
