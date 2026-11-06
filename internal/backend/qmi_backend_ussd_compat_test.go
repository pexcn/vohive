package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

func TestQMIBackendExecuteUSSDCancellationWaitsForRelease(t *testing.T) {
	for _, mode := range []string{"sync", "callback", "no-wait"} {
		for _, cancelFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancelFails=%v", mode, cancelFails), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				src := &qmiBackendUSSDNoWaitSourceStub{
					qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{
						blockOriginate: mode == "sync", originateHook: cancel,
					},
					noWaitErr: &qmi.QMIError{ErrorCode: qmi.QMIErrNotSupported},
				}
				wantSync := 1
				if mode == "no-wait" {
					wantSync = 0
					src.noWaitHook = func(ctx context.Context) error {
						cancel()
						return ctx.Err()
					}
				}
				if cancelFails {
					src.cancelErr = errors.New("cancel failed")
				}
				be, err := NewQMIBackend("/dev/null", src)
				if err != nil {
					t.Fatal(err)
				}
				_, err = be.ExecuteUSSD(ctx, "*109#", time.Second)
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error=%v, want context.Canceled", err)
				}
				if src.cancelCalls != 1 || src.cancelContextErr != nil || !src.cancelDeadline {
					t.Fatalf("cleanup calls=%d ctxErr=%v deadline=%v", src.cancelCalls, src.cancelContextErr, src.cancelDeadline)
				}
				// A stale callback and even a successful cancel ACK cannot unlock
				// the next request before the release indication.
				src.onUSSD(&qmi.VoiceUSSDIndication{USSData: &qmi.VoiceUSSDPayload{Text: "stale reply"}})
				_, err = be.ExecuteUSSD(context.Background(), "*100#", time.Second)
				if err == nil || !strings.Contains(err.Error(), "尚未释放") || len(src.originateRequests) != wantSync || len(src.noWaitRequests) != 1 {
					t.Fatalf("error=%v sync=%d no-wait=%d, want no new request", err, len(src.originateRequests), len(src.noWaitRequests))
				}
				src.originateHook = nil
				src.blockOriginate = false
				src.noWaitHook = nil
				src.noWaitErr = nil
				src.deliverNoWait = true
				src.noWait = &qmi.VoiceUSSDNoWaitIndication{USSData: &qmi.VoiceUSSDPayload{Text: "fresh reply"}}
				src.onRelease()
				got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
				if err != nil || got == nil || got.Text != "fresh reply" || len(src.originateRequests) != wantSync || len(src.noWaitRequests) != 2 {
					t.Fatalf("result=%+v error=%v sync=%d", got, err, len(src.originateRequests))
				}
			})
		}
	}
}

func TestQMIBackendExecuteUSSDAlreadyCanceledDoesNotSend(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = be.ExecuteUSSD(ctx, "*109#", time.Second)
	if !errors.Is(err, context.Canceled) || len(src.originateRequests) != 0 || src.cancelCalls != 0 {
		t.Fatalf("error=%v originate=%d cancel=%d", err, len(src.originateRequests), src.cancelCalls)
	}
	src.response = &qmi.VoiceUSSDResponse{USSData: &qmi.VoiceUSSDPayload{Text: "fresh reply"}}
	if _, err := be.ExecuteUSSD(context.Background(), "*109#", time.Second); err != nil {
		t.Fatalf("request that was never sent blocked next request: %v", err)
	}
}

func TestQMIBackendExecuteUSSDDoesNotRetryBusinessFailure(t *testing.T) {
	for _, withPayload := range []bool{false, true} {
		t.Run(fmt.Sprintf("payload=%v", withPayload), func(t *testing.T) {
			response := &qmi.VoiceUSSDResponse{HasFailureCause: true, FailureCause: 25}
			if withPayload {
				response.USSData = &qmi.VoiceUSSDPayload{Text: "diagnostic text"}
			}
			src := &qmiBackendUSSDNoWaitSourceStub{
				qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{
					response: response,
					responseErr: &qmi.QMIError{
						Service: qmi.ServiceVOICE, MessageID: qmi.VOICEOriginateUSSD,
						Result: 1, ErrorCode: 0x005c,
					},
				},
				noWaitErr: &qmi.QMIError{ErrorCode: qmi.QMIErrNotSupported},
			}
			be, err := NewQMIBackend("/dev/null", src)
			if err != nil {
				t.Fatal(err)
			}
			result, err := be.ExecuteUSSD(context.Background(), "*109#", time.Second)
			if result != nil || err == nil {
				t.Fatalf("result=%+v err=%v, want failure", result, err)
			}
			for _, detail := range []string{"SUPS_FAILURE_CASE", "failure_cause=0x0019", "ReleaseNormal"} {
				if !strings.Contains(err.Error(), detail) {
					t.Errorf("error %q missing %q", err, detail)
				}
			}
			if len(src.originateRequests) != 1 || len(src.noWaitRequests) != 1 {
				t.Fatalf("sync=%d no-wait=%d, want one unsupported attempt then one sync send", len(src.originateRequests), len(src.noWaitRequests))
			}
		})
	}
}

func TestQMIBackendExecuteUSSDNoWaitFailureDoesNotFallBack(t *testing.T) {
	src := &qmiBackendUSSDNoWaitSourceStub{
		qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{},
		noWaitErr: &qmi.QMIError{
			Service: qmi.ServiceVOICE, MessageID: qmi.VOICEOriginateUSSDNoWait,
			Result: 1, ErrorCode: 0x005c,
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatal(err)
	}
	result, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if result != nil || err == nil || !strings.Contains(err.Error(), "SUPS_FAILURE_CASE") {
		t.Fatalf("result=%+v error=%v, want network failure", result, err)
	}
	if len(src.originateRequests) != 0 || len(src.noWaitRequests) != 1 || src.cancelCalls != 0 {
		t.Fatalf("sync=%d no-wait=%d cancel=%d, want no resend", len(src.originateRequests), len(src.noWaitRequests), src.cancelCalls)
	}
}

func TestQMIBackendExecuteUSSDDoesNotRetryTimeout(t *testing.T) {
	src := &qmiBackendUSSDNoWaitSourceStub{
		qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatal(err)
	}
	_, err = be.ExecuteUSSD(context.Background(), "*109#", 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("error=%v, want timeout", err)
	}
	if len(src.originateRequests) != 0 || len(src.noWaitRequests) != 1 || src.cancelCalls != 1 {
		t.Fatalf("sync=%d no-wait=%d cancel=%d", len(src.originateRequests), len(src.noWaitRequests), src.cancelCalls)
	}
}

func TestQMIUSSDNoWaitPreservesBothFailureDetails(t *testing.T) {
	_, err, handled := qmiUSSDResultFromNoWait(&qmi.VoiceUSSDNoWaitIndication{
		HasErrorCode: true, ErrorCode: 92, HasFailureCause: true, FailureCause: 25,
		USSDataUTF16: utf16.Encode([]rune("不能当作成功的错误回复")),
	})
	if !handled || err == nil {
		t.Fatalf("handled=%v error=%v, want failure", handled, err)
	}
	for _, detail := range []string{"error_code=0x005C", "SUPS_FAILURE_CASE", "failure_cause=0x0019", "ReleaseNormal"} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q missing %q", err, detail)
		}
	}
}

func TestQMIUSSDDecodesUTF16Replies(t *testing.T) {
	const text = "余额 HK$12.50 📱"
	data := utf16.Encode([]rune(text))
	for _, channel := range []string{"response", "indication", "no-wait"} {
		t.Run(channel, func(t *testing.T) {
			var result USSDResult
			var err error
			var handled bool
			switch channel {
			case "response":
				var response *USSDResult
				response, err, handled = qmiUSSDResultFromResponse(&qmi.VoiceUSSDResponse{
					USSDataUTF16: data, Alpha: &qmi.VoiceUSSDPayload{Text: "SIM label"},
				})
				if response != nil {
					result = *response
				}
			case "indication":
				result, handled = qmiUSSDResultFromIndication(&qmi.VoiceUSSDIndication{USSDataUTF16: data})
			case "no-wait":
				result, err, handled = qmiUSSDResultFromNoWait(&qmi.VoiceUSSDNoWaitIndication{
					HasErrorCode: true, ErrorCode: 0,
					USSDataUTF16: data, Alpha: &qmi.VoiceUSSDPayload{Text: "SIM label"},
				})
			}
			if !handled || err != nil || result.Text != text || result.RawText != text || result.DCS != 3 {
				t.Fatalf("handled=%v result=%+v err=%v", handled, result, err)
			}
		})
	}
}
