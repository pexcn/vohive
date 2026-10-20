package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

type controlAgentProbe struct{ writes int }

func (a *controlAgentProbe) Dial(context.Context, voicehost.CallRequest) (voicehost.CallInfo, error) {
	return voicehost.CallInfo{ID: "call", State: "connected"}, nil
}
func (a *controlAgentProbe) Answer(context.Context, string, voicehost.CallRequest) (voicehost.CallInfo, error) {
	return voicehost.CallInfo{ID: "call", State: "answering"}, nil
}
func (a *controlAgentProbe) Hangup(context.Context, string) error                { return nil }
func (a *controlAgentProbe) Hold(context.Context, string, bool) error            { return nil }
func (a *controlAgentProbe) SendDTMF(context.Context, string, string, int) error { return nil }
func (a *controlAgentProbe) Calls() []voicehost.CallInfo                         { return []voicehost.CallInfo{{ID: "call"}} }
func (a *controlAgentProbe) ReadMedia(context.Context, string) (voicehost.MediaFrame, error) {
	return voicehost.MediaFrame{Packet: []byte{1}}, nil
}
func (a *controlAgentProbe) WriteMedia(context.Context, string, voicehost.MediaFrame) error {
	a.writes++
	return nil
}
func TestVoiceControlRoutesAndMediaBodyLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := voicehost.NewGateway()
	if err := gateway.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer gateway.Stop()
	agent := &controlAgentProbe{}
	gateway.RegisterAgent("dev", agent)
	server := &Server{voiceGW: gateway}
	router := gin.New()
	server.registerVoiceControlRoutes(router.Group("/api"))
	for _, test := range []struct{ method, path, body string }{{"GET", "/api/voice/dev/calls", ""}, {"GET", "/api/voice/dev/call/media", ""}, {"POST", "/api/voice/dev/dial", `{"callee":"10086"}`}, {"POST", "/api/voice/dev/call/answer", `{}`}, {"POST", "/api/voice/dev/call/media", `{"packet":"AQ=="}`}} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
	body := `{"packet":"` + strings.Repeat("A", 32768) + `"}`
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/voice/dev/call/media", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	if recorder.Code < 400 || agent.writes != 1 {
		t.Fatalf("oversized media reached the agent: status=%d writes=%d", recorder.Code, agent.writes)
	}
}
