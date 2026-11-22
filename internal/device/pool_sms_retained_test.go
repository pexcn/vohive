package device

import (
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

func newRetainedSMSWorker(indices ...uint32) (*Worker, *qmiSMSCoreStub, *mbimInboxNotifierStub) {
	stub := &qmiSMSCoreStub{
		listByStorage: map[uint8][]struct {
			Index uint32
			Tag   qmi.MessageTagType
		}{},
		readResults: map[string]*qmimanager.DecodedSMS{},
		readErrors:  map[string]error{},
	}
	for _, index := range indices {
		stub.listByStorage[1] = append(stub.listByStorage[1], struct {
			Index uint32
			Tag   qmi.MessageTagType
		}{index, qmi.TagTypeMTNotRead})
		stub.readResults[stub.key(1, index)] = &qmimanager.DecodedSMS{
			Storage: 1, Index: index, Sender: "10086", Message: fmt.Sprint("message ", index),
			Timestamp: time.Date(2026, 11, 22, 21, 0, 0, 0, time.FixedZone("first", 19800)),
		}
	}
	notifier := &mbimInboxNotifierStub{}
	return &Worker{ID: "retained-sms", qmiSMS: stub, Pool: &Pool{notifier: notifier}}, stub, notifier
}

func TestRetainedQMISMSStopsRepeatedReadsAndLogs(t *testing.T) {
	logger.Setup(logger.LogConfig{Debug: true, Filename: filepath.Join(t.TempDir(), "app.log")})
	ch := logger.GlobalBroadcaster.Subscribe()
	defer logger.GlobalBroadcaster.Unsubscribe(ch)
	w, stub, notifier := newRetainedSMSWorker(3, 4, 5, 6)
	pollStoredSMS(t, w, 60)
	if len(stub.readCalls) != 4 || len(stub.listCalls) != 240 || len(notifier.sms) != 4 || len(stub.deleteCalls) != 0 {
		t.Fatalf("retained SMS was polled again: reads=%v lists=%d notifications=%d deletes=%v", stub.readCalls, len(stub.listCalls), len(notifier.sms), stub.deleteCalls)
	}
	announcements := 0
	for _, entry := range drainLoggerEntries(ch) {
		if readLogFields(t, entry)["event"] == "SMS_AUTO_DELETE_SKIPPED" {
			announcements++
		}
	}
	if announcements != 4 {
		t.Fatalf("skip-delete announcements=%d, want one per retained SMS", announcements)
	}
}

func TestRetainedQMISMSURCReadsAndDistinguishesSlotContents(t *testing.T) {
	w, stub, notifier := newRetainedSMSWorker(3)
	pollStoredSMS(t, w, 2)
	// Equivalent timestamp locations must not make a duplicate SMS look new.
	stub.readResults["1:3"].Timestamp = time.Date(2026, 11, 22, 21, 0, 0, 0, time.FixedZone("second", 19800))
	w.handleNewSMSQMI(1, 3)
	if len(stub.readCalls) != 2 || len(notifier.sms) != 1 {
		t.Fatalf("duplicate indication reprocessed SMS: reads=%v notifications=%v", stub.readCalls, notifier.sms)
	}
	stub.readResults["1:3"].Message = "new slot content"
	w.handleNewSMSQMI(1, 3)
	if len(notifier.sms) != 2 || notifier.sms[1].Content != "new slot content" {
		t.Fatalf("reused slot was ignored: %+v", notifier.sms)
	}
	stub.readResults["1:3"].Timestamp = stub.readResults["1:3"].Timestamp.Add(time.Minute)
	w.handleNewSMSQMI(1, 3)
	pollStoredSMS(t, w, 5)
	if len(stub.readCalls) != 4 || len(notifier.sms) != 3 || len(stub.deleteCalls) != 0 {
		t.Fatalf("new timestamp or retained polling handled incorrectly: reads=%v notifications=%v", stub.readCalls, notifier.sms)
	}
}

func TestRetainedQMISMSReadResidualAnnouncementDoesNotHideUnreadSMS(t *testing.T) {
	w, stub, notifier := newRetainedSMSWorker(3)
	stub.listByStorage[1][0].Tag = qmi.TagTypeMTRead
	pollStoredSMS(t, w, 4)
	if len(stub.readCalls) != 0 || len(stub.deleteCalls) != 0 {
		t.Fatalf("read residual was read or deleted: %v %v", stub.readCalls, stub.deleteCalls)
	}
	stub.listByStorage[1][0].Tag = qmi.TagTypeMTNotRead
	pollStoredSMS(t, w, 4)
	if len(stub.readCalls) != 1 || len(notifier.sms) != 1 || len(stub.deleteCalls) != 0 {
		t.Fatalf("announced residual hid unread SMS: reads=%v notifications=%v", stub.readCalls, notifier.sms)
	}
}

func TestRetainedQMISMSMarksExpireWithScopeOrConfirmedDisappearance(t *testing.T) {
	for _, reset := range []string{"identity", "core", "worker", "disappearance"} {
		t.Run(reset, func(t *testing.T) {
			w, stub, notifier := newRetainedSMSWorker(3)
			pollStoredSMS(t, w, 3)
			switch reset {
			case "identity":
				w.BeginSIMIdentityTransition("next-card", "test")
			case "core":
				w.qmiSMSPoll.reset.Add(1)
			case "worker":
				w = &Worker{ID: w.ID, qmiSMS: stub, Pool: &Pool{notifier: notifier}}
			case "disappearance":
				list := stub.listByStorage[1]
				stub.listByStorage[1] = nil
				pollStoredSMS(t, w, 1)
				stub.listByStorage[1] = list
			}
			pollStoredSMS(t, w, 3)
			if len(stub.readCalls) != 2 || len(notifier.sms) != 2 {
				t.Fatalf("retained marks survived %s: reads=%v notifications=%v", reset, stub.readCalls, notifier.sms)
			}
		})
	}
}

func TestRetainedQMISMSIncompleteListingDoesNotClearMarks(t *testing.T) {
	w, stub, _ := newRetainedSMSWorker(3)
	pollStoredSMS(t, w, 1)
	list := stub.listByStorage[1]
	stub.listByStorage[1] = nil
	w.qmiSMS = partialListSMSCore{stub}
	pollStoredSMS(t, w, 1)
	w.qmiSMS, stub.listByStorage[1] = stub, list
	pollStoredSMS(t, w, 1)
	if len(stub.readCalls) != 1 {
		t.Fatalf("incomplete listing cleared retained marks: %v", stub.readCalls)
	}
}

func TestRetainedQMISMSURCDecodeFailureResumesConfirmation(t *testing.T) {
	w, stub, notifier := newRetainedSMSWorker(3)
	pollStoredSMS(t, w, 1)
	_, stub.readErrors["1:3"] = qmicore.DecodeIncomingSMSPDU(zeroTPDUSample(t), 1, 3)
	w.handleNewSMSQMI(1, 3)
	pollStoredSMS(t, w, 6)
	if len(stub.readCalls) != 4 || len(notifier.sms) != 1 || len(stub.deleteCalls) != 0 {
		t.Fatalf("retained record hid a changed invalid payload: %v", stub.readCalls)
	}
}

func TestRetainedQMISMSSerializesConcurrentPollingAndURCs(t *testing.T) {
	w, stub, notifier := newRetainedSMSWorker(3)
	pollStoredSMS(t, w, 1)
	stub.readResults["1:3"].Message = "new slot content"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); _ = w.CheckAllSMSQMI() }()
		go func() { defer wg.Done(); w.handleNewSMSQMI(1, 3) }()
	}
	wg.Wait()
	if len(notifier.sms) != 2 || len(stub.deleteCalls) != 0 {
		t.Fatalf("concurrent reads duplicated handling: notifications=%v deletes=%v", notifier.sms, stub.deleteCalls)
	}
}
