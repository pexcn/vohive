package device

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/qmi-go/pkg/qmi"
	qmicore "github.com/iniwex5/vohive/internal/qmi"
	"github.com/iniwex5/vohive/pkg/logger"
)

func zeroTPDUSample(t *testing.T) []byte {
	t.Helper()
	head, err := hex.DecodeString("0891349609406101F1")
	if err != nil {
		t.Fatal(err)
	}
	return append(head, make([]byte, 246)...)
}

func TestIsCorruptedZeroTPDU(t *testing.T) {
	sample := zeroTPDUSample(t)
	nonzero := bytes.Clone(sample)
	nonzero[len(nonzero)-1] = 1
	badAddress := bytes.Clone(sample)
	badAddress[2] = 0xff
	for _, tt := range []struct {
		name string
		pdu  []byte
		want bool
	}{
		{"observed_record", sample, true},
		{"all_zero", make([]byte, 255), true},
		{"nil", nil, false},
		{"empty", []byte{}, false},
		{"valid_sms", qmiRawSMSFixtureDirectTPDU(t), false},
		{"nonzero_tpdu", nonzero, false},
		{"truncated_smsc", sample[:8], false},
		{"missing_tpdu", sample[:9], false},
		{"invalid_smsc_address", badAddress, false},
		{"invalid_toa", []byte{2, 0, 0x21, 0, 0}, false},
		{"no_smsc_digits", []byte{1, 0x91, 0, 0}, false},
		{"alphanumeric_toa", []byte{2, 0xd0, 0x21, 0, 0}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCorruptedZeroTPDU(tt.pdu); got != tt.want {
				t.Fatalf("isCorruptedZeroTPDU()=%v, want %v", got, tt.want)
			}
		})
	}
}

func newZeroTPDUWorker(t *testing.T) (*Worker, *qmiSMSCoreStub, *mbimInboxNotifierStub) {
	t.Helper()
	_, err := qmicore.DecodeIncomingSMSPDU(zeroTPDUSample(t), 1, 0)
	if err == nil {
		t.Fatal("zero-filled sample unexpectedly decoded")
	}
	stub := &qmiSMSCoreStub{
		listByStorage: map[uint8][]struct {
			Index uint32
			Tag   qmi.MessageTagType
		}{1: {{Index: 0, Tag: qmi.TagTypeMTNotRead}}},
		readErrors:  map[string]error{"1:0": fmt.Errorf("read result: %w", err)},
		readResults: make(map[string]*qmimanager.DecodedSMS),
	}
	notifier := &mbimInboxNotifierStub{}
	w := &Worker{ID: "zero-tpdu", qmiSMS: stub, Pool: &Pool{notifier: notifier}}
	return w, stub, notifier
}

func pollStoredSMS(t *testing.T, w *Worker, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := w.CheckAllSMSQMI(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQMISMSPollStopsIdenticalZeroTPDUWithoutDeleting(t *testing.T) {
	logger.Setup(logger.LogConfig{Debug: true, Filename: filepath.Join(t.TempDir(), "app.log")})
	ch := logger.GlobalBroadcaster.Subscribe()
	defer logger.GlobalBroadcaster.Unsubscribe(ch)
	w, stub, notifier := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 12)
	if len(stub.readCalls) != 3 || len(stub.listCalls) != 48 {
		t.Fatalf("reads=%v lists=%d; want 3 reads and continued listings", stub.readCalls, len(stub.listCalls))
	}
	if len(stub.deleteCalls) != 0 || len(notifier.sms) != 0 {
		t.Fatalf("invalid record was deleted or notified: %v %v", stub.deleteCalls, notifier.sms)
	}
	counts := map[string]int{}
	for _, entry := range drainLoggerEntries(ch) {
		fields := readLogFields(t, entry)
		if event, ok := fields["event"].(string); ok {
			counts[event]++
		}
	}
	if counts["SMS_ZERO_TPDU_DETECTED"] != 1 || counts["SMS_ZERO_TPDU_POLL_SKIPPED"] != 1 || counts["SMS_PDU_DECODE_REJECTED"] != 0 {
		t.Fatalf("unexpected diagnostic counts: %v", counts)
	}

	// The same index in another storage is a different slot.
	stub.listByStorage[0] = append(stub.listByStorage[0], struct {
		Index uint32
		Tag   qmi.MessageTagType
	}{0, qmi.TagTypeMTNotRead})
	stub.readResults["0:0"] = &qmimanager.DecodedSMS{Sender: "Apple", Message: "verification", Timestamp: time.Now()}
	pollStoredSMS(t, w, 1)
	if len(stub.readCalls) != 4 || stub.readCalls[3] != "0:0" || len(stub.deleteCalls) != 0 || len(notifier.sms) != 1 {
		t.Fatalf("other storage not handled normally: reads=%v deletes=%v notifications=%v", stub.readCalls, stub.deleteCalls, notifier.sms)
	}
}

func TestQMISMSPollURCRechecksBlockedSlotAndAllowsReuse(t *testing.T) {
	w, stub, notifier := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 5)
	w.handleNewSMSQMI(1, 0)
	pollStoredSMS(t, w, 5)
	if len(stub.readCalls) != 4 {
		t.Fatalf("URC should allow one read without resuming polling: %v", stub.readCalls)
	}
	stub.readErrors["1:0"] = errors.New("temporary transport failure")
	w.handleNewSMSQMI(1, 0)
	pollStoredSMS(t, w, 5)
	if len(stub.readCalls) != 5 {
		t.Fatalf("failed URC read should not resume a blocked slot's polling: %v", stub.readCalls)
	}
	delete(stub.readErrors, "1:0")
	// An empty but valid decoded SMS must also clear the blocked slot.
	stub.readResults["1:0"] = &qmimanager.DecodedSMS{Sender: "Apple", Timestamp: time.Now()}
	w.handleNewSMSQMI(qmiSMSStorageUnknown, 0)
	if len(stub.readCalls) != 6 || len(stub.deleteCalls) != 0 || len(notifier.sms) != 1 {
		t.Fatalf("reused slot not processed: reads=%v deletes=%v notifications=%v", stub.readCalls, stub.deleteCalls, notifier.sms)
	}
	// A later invalid message in this slot needs three fresh confirmations.
	_, stub.readErrors["1:0"] = qmicore.DecodeIncomingSMSPDU(zeroTPDUSample(t), 1, 0)
	pollStoredSMS(t, w, 6)
	if len(stub.readCalls) != 9 {
		t.Fatalf("successful read did not clear old confirmations: %v", stub.readCalls)
	}
}

func TestQMISMSPollRequiresConsecutiveIdenticalDecodeFailures(t *testing.T) {
	for _, reason := range []string{"different_zero_payload", "other_decode_error", "transport_error"} {
		t.Run(reason, func(t *testing.T) {
			w, stub, _ := newZeroTPDUWorker(t)
			pollStoredSMS(t, w, 2)
			original := stub.readErrors["1:0"]
			switch reason {
			case "different_zero_payload":
				_, stub.readErrors["1:0"] = qmicore.DecodeIncomingSMSPDU(make([]byte, 255), 1, 0)
			case "other_decode_error":
				_, stub.readErrors["1:0"] = qmicore.DecodeIncomingSMSPDU([]byte{0x08, 0x91, 0x34}, 1, 0)
			case "transport_error":
				stub.readErrors["1:0"] = errors.New("transport timeout")
			}
			pollStoredSMS(t, w, 1)
			stub.readErrors["1:0"] = original
			pollStoredSMS(t, w, 6)
			if len(stub.readCalls) != 6 || len(stub.deleteCalls) != 0 {
				t.Fatalf("streak not reset: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
			}
		})
	}
	for _, err := range []error{errors.New("transport failure"), &qmicore.IncomingSMSDecodeError{PDU: []byte{8, 0x91, 0x34}, Cause: errors.New("invalid")}} {
		w, stub, _ := newZeroTPDUWorker(t)
		stub.readErrors["1:0"] = err
		pollStoredSMS(t, w, 6)
		if len(stub.readCalls) != 6 || len(stub.deleteCalls) != 0 {
			t.Fatalf("unrelated error was suppressed or deleted: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
		}
	}
}

func TestQMISMSPollFallbackCannotReadBlockedSlot(t *testing.T) {
	w, stub, _ := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 3)
	stub.listByStorage[0] = append(stub.listByStorage[0], struct {
		Index uint32
		Tag   qmi.MessageTagType
	}{0, qmi.TagTypeMTNotRead})
	stub.readErrors["0:0"] = errors.New("primary storage unavailable")
	pollStoredSMS(t, w, 4)
	if fmt.Sprint(stub.readCalls) != "[1:0 1:0 1:0 0:0 0:0 0:0 0:0]" || len(stub.deleteCalls) != 0 {
		t.Fatalf("fallback bypassed blocked slot: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
	}
}

func TestQMISMSPollTransportFailureResetsStreakBeforeFallback(t *testing.T) {
	w, stub, _ := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 2)
	original := stub.readErrors["1:0"]
	stub.readErrors["1:0"] = errors.New("primary storage unavailable")
	stub.listByStorage[0] = append(stub.listByStorage[0], struct {
		Index uint32
		Tag   qmi.MessageTagType
	}{0, qmi.TagTypeMTNotRead})
	stub.readResults["0:0"] = &qmimanager.DecodedSMS{Sender: "other-slot", Message: "valid", Timestamp: time.Now()}
	w.handleNewSMSQMI(1, 0)
	stub.listByStorage[0] = nil
	stub.readErrors["1:0"] = original
	pollStoredSMS(t, w, 6)
	if len(stub.readCalls) != 7 || len(stub.deleteCalls) != 0 {
		t.Fatalf("fallback kept the primary slot's old streak: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
	}
}

func TestQMISMSPollKnownInvalidReadTagIsNeverDeleted(t *testing.T) {
	for _, confirmations := range []int{1, 3} {
		t.Run(fmt.Sprint(confirmations), func(t *testing.T) {
			w, stub, _ := newZeroTPDUWorker(t)
			pollStoredSMS(t, w, confirmations)
			stub.listByStorage[1][0].Tag = qmi.TagTypeMTRead
			pollStoredSMS(t, w, 6)
			if len(stub.readCalls) != 3 || len(stub.deleteCalls) != 0 {
				t.Fatalf("tag change bypassed protection: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
			}
		})
	}
}

func TestQMISMSPollScopeChangesClearBlockedSlots(t *testing.T) {
	for _, reset := range []string{"profile_switch", "different_iccid", "core_reset", "new_worker"} {
		t.Run(reset, func(t *testing.T) {
			w, stub, _ := newZeroTPDUWorker(t)
			pollStoredSMS(t, w, 5)
			switch reset {
			case "profile_switch":
				w.BeginSIMIdentityTransition("new-profile", "test")
			case "different_iccid":
				w.cacheMu.Lock()
				w.state.Identity.ICCID = "new-card"
				w.cacheMu.Unlock()
			case "core_reset":
				w.qmiSMSPoll.reset.Add(1)
			case "new_worker":
				w = &Worker{ID: w.ID, qmiSMS: stub, Pool: &Pool{}}
			}
			pollStoredSMS(t, w, 5)
			if len(stub.readCalls) != 6 {
				t.Fatalf("old blocked slot survived %s: %v", reset, stub.readCalls)
			}
		})
	}
}

type partialListSMSCore struct {
	*qmiSMSCoreStub
}

func (s partialListSMSCore) ListSMS(storage uint8, tag qmi.MessageTagType) ([]struct {
	Index uint32
	Tag   qmi.MessageTagType
}, error) {
	if storage == 1 && tag == qmi.TagTypeMTRead {
		return nil, errors.New("listing failed")
	}
	return s.qmiSMSCoreStub.ListSMS(storage, tag)
}

func TestQMISMSPollDisappearanceRequiresCompleteStorageListing(t *testing.T) {
	w, stub, _ := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 3)
	saved := stub.listByStorage[1]
	stub.listByStorage[1] = nil
	w.qmiSMS = partialListSMSCore{stub}
	pollStoredSMS(t, w, 1)
	w.qmiSMS = stub
	stub.listByStorage[1] = saved
	pollStoredSMS(t, w, 1)
	if len(stub.readCalls) != 3 {
		t.Fatalf("incomplete listing cleared blocked slot: %v", stub.readCalls)
	}
	stub.listByStorage[1] = nil
	pollStoredSMS(t, w, 1)
	stub.listByStorage[1] = saved
	pollStoredSMS(t, w, 5)
	if len(stub.readCalls) != 6 {
		t.Fatalf("confirmed disappearance did not clear blocked slot: %v", stub.readCalls)
	}
}

type pausedSMSCore struct {
	*qmiSMSCoreStub
	started chan struct{}
	release chan struct{}
	once    sync.Once
	result  *qmimanager.DecodedSMS
}

func (s *pausedSMSCore) ReadSMS(storage uint8, index uint32) (*qmimanager.DecodedSMS, error) {
	var result *qmimanager.DecodedSMS
	s.once.Do(func() {
		close(s.started)
		<-s.release
		result = s.result
	})
	msg, err := s.qmiSMSCoreStub.ReadSMS(storage, index)
	if result != nil {
		return result, nil
	}
	return msg, err
}

func waitSMSOperation(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("stored SMS operation did not complete")
	}
}

func TestQMISMSPollDiscardsReadsFromPreviousScope(t *testing.T) {
	for _, change := range []string{"core_reset", "profile_switch", "stale_valid_message"} {
		t.Run(change, func(t *testing.T) {
			w, stub, notifier := newZeroTPDUWorker(t)
			pollStoredSMS(t, w, 2)
			paused := &pausedSMSCore{qmiSMSCoreStub: stub, started: make(chan struct{}), release: make(chan struct{})}
			if change == "stale_valid_message" {
				paused.result = &qmimanager.DecodedSMS{Sender: "old-card", Message: "old", Timestamp: time.Now()}
			}
			w.qmiSMS = paused
			done := make(chan struct{})
			go func() { defer close(done); _ = w.CheckAllSMSQMI() }()
			waitSMSOperation(t, paused.started)
			if change == "profile_switch" {
				w.BeginSIMIdentityTransition("other-card", "test")
			} else {
				w.qmiSMSPoll.reset.Add(1)
			}
			close(paused.release)
			waitSMSOperation(t, done)
			pollStoredSMS(t, w, 6)
			if len(stub.readCalls) != 6 || len(stub.deleteCalls) != 0 || len(notifier.sms) != 0 {
				t.Fatalf("stale result reused: reads=%v deletes=%v notifications=%v", stub.readCalls, stub.deleteCalls, notifier.sms)
			}
		})
	}
}

func TestQMISMSPollSerializesURCAndPolling(t *testing.T) {
	w, stub, _ := newZeroTPDUWorker(t)
	pollStoredSMS(t, w, 2)
	paused := &pausedSMSCore{qmiSMSCoreStub: stub, started: make(chan struct{}), release: make(chan struct{})}
	w.qmiSMS = paused
	polled, indicated := make(chan struct{}), make(chan struct{})
	go func() { defer close(polled); _ = w.CheckAllSMSQMI() }()
	waitSMSOperation(t, paused.started)
	go func() { defer close(indicated); w.handleNewSMSQMI(1, 0) }()
	close(paused.release)
	waitSMSOperation(t, polled)
	waitSMSOperation(t, indicated)
	pollStoredSMS(t, w, 5)
	if len(stub.readCalls) != 4 || len(stub.deleteCalls) != 0 {
		t.Fatalf("expected three confirmations and one forced URC read: reads=%v deletes=%v", stub.readCalls, stub.deleteCalls)
	}
}
