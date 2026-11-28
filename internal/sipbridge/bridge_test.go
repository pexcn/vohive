package sipbridge

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"github.com/iniwex5/vohive/internal/config"
	"github.com/iniwex5/vohive/internal/db"
)

func TestSIPBridgeSendsAndDeliversSMSOverUDP(t *testing.T) {
	if err := db.Init(filepath.Join(t.TempDir(), "bridge.db")); err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sql.Close() })
	sent := make(chan db.SIPBridgeSMS, 1)
	bridge, err := Start(config.SIPBridgeConfig{
		Enabled: true, ListenAddr: "127.0.0.1:0",
		Accounts: []config.SIPBridgeAccount{{Username: "client", Password: "unit-test-secret", DeviceID: "modem", Bearer: "ims"}},
	}, SMSCallbacks{
		CurrentICCID: func(string) string { return "12345" },
		Send: func(_ context.Context, job db.SIPBridgeSMS, bearer string) (string, string, string, error) {
			job.Bearer = bearer
			select {
			case sent <- job:
			default:
			}
			return "ims-message", db.SMSBearerVoWiFi, "submitted", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := bridge.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	if err := db.SaveReceivedSMSForDevice("modem", "001010000000001", "", "+15550000001", "received over the air", time.Now(), db.SMSBearerCellular); err != nil {
		t.Fatal(err)
	}

	client, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(8 * time.Second))
	address := bridge.listener.LocalAddr().(*net.UDPAddr)
	sipAddress := "sip:" + address.String()
	read := func() sip.Message {
		t.Helper()
		buffer := make([]byte, 4096)
		n, source, err := client.ReadFromUDP(buffer)
		if err != nil {
			t.Fatal(err)
		}
		if source.Port != address.Port {
			t.Fatalf("SIP used port %d, want %d", source.Port, address.Port)
		}
		message, err := sip.ParseMessage(buffer[:n])
		if err != nil {
			t.Fatal(err)
		}
		return message
	}
	send := func(data string) {
		t.Helper()
		if _, err := client.WriteToUDP([]byte(data), address); err != nil {
			t.Fatal(err)
		}
	}
	makeRequest := func(method, target, callID string, seq int, body string, auth *digest.Credentials) string {
		local := client.LocalAddr().String()
		headers := fmt.Sprintf("%s %s SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=z9hG4bK%s;rport\r\nFrom: <sip:client@%s>;tag=client\r\nTo: <sip:client@%s>\r\nCall-ID: %s\r\nCSeq: %d %s\r\n",
			method, target, local, sip.GenerateTagN(16), address.IP, address.IP, callID, seq, method)
		if method == "REGISTER" {
			headers += "Contact: <sip:client@198.51.100.250:5060>;expires=120\r\n"
		} else {
			headers += "Content-Type: text/plain; charset=utf-8\r\n"
		}
		if auth != nil {
			headers += "Authorization: " + auth.String() + "\r\n"
		}
		return headers + fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	response := func(want int) *sip.Response {
		t.Helper()
		res, ok := read().(*sip.Response)
		if !ok || res.StatusCode != want {
			t.Fatalf("wanted SIP %d, got %v", want, res)
		}
		return res
	}
	credentials := func(res *sip.Response, method, uri string) *digest.Credentials {
		t.Helper()
		ch, err := digest.ParseChallenge(res.GetHeader("WWW-Authenticate").Value())
		if err != nil {
			t.Fatal(err)
		}
		cr, err := digest.Digest(ch, digest.Options{Method: method, URI: uri, Username: "client", Password: "unit-test-secret"})
		if err != nil {
			t.Fatal(err)
		}
		return cr
	}

	send(makeRequest("REGISTER", sipAddress, "register-test", 1, "", nil))
	registerAuth := credentials(response(401), "REGISTER", sipAddress)
	send(makeRequest("REGISTER", sipAddress, "register-test", 2, "", registerAuth))
	response(200)

	msg, ok := read().(*sip.Request)
	if !ok || msg.Method != sip.MESSAGE || string(msg.Body()) != "received over the air" || msg.From().Address.User != "+15550000001" {
		t.Fatalf("offline SMS did not reach the client: %v", msg)
	}
	inboundID := msg.GetHeader("X-VoHive-SMS-ID").Value()
	send(sip.NewResponseFromRequest(msg, 200, "OK", nil).String())

	target := "sip:+15550000002@" + address.String()
	send(makeRequest("MESSAGE", target, "sms-test", 1, "submitted by the client", nil))
	sendAuth := credentials(response(401), "MESSAGE", target)
	send(makeRequest("MESSAGE", target, "sms-test", 2, "submitted by the client", sendAuth))
	id := response(202).GetHeader("X-VoHive-SMS-ID").Value()
	send(makeRequest("MESSAGE", target, "sms-test", 2, "submitted by the client", nil))
	retryAuth := credentials(response(401), "MESSAGE", target)
	send(makeRequest("MESSAGE", target, "sms-test", 2, "submitted by the client", retryAuth))
	if retryID := response(202).GetHeader("X-VoHive-SMS-ID").Value(); retryID != id {
		t.Fatalf("retransmission created a new job %s (original %s)", retryID, id)
	}
	select {
	case job := <-sent:
		if job.ID != id || job.Peer != "+15550000002" || job.Bearer != "ims" || job.Content != "submitted by the client" {
			t.Fatalf("incorrectly routed job: %+v", job)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SMS did not reach the modem submission callback")
	}

	for deadline := time.Now().Add(time.Second); ; {
		inbound, inErr := db.SIPBridgeJob(t.Context(), inboundID)
		outbound, outErr := db.SIPBridgeJob(t.Context(), id)
		if inErr == nil && outErr == nil && inbound.State == "delivered" && outbound.State == "submitted" &&
			outbound.PhoneMessageID == "ims-message" && outbound.Attempts == 1 && strings.HasPrefix(inboundID, "sms-") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("unexpected delivery state: %s, %s", inbound.State, outbound.State)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
