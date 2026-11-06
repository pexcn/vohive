package modem

import (
	"strings"
	"testing"
	"time"

	"go.bug.st/serial"
)

type scriptedATPort struct {
	serial.Port
	chunks []string
	writes []string
	reads  int
}

func (p *scriptedATPort) SetReadTimeout(time.Duration) error { return nil }

func (p *scriptedATPort) Write(data []byte) (int, error) {
	p.writes = append(p.writes, string(data))
	return len(data), nil
}

func (p *scriptedATPort) Read(buf []byte) (int, error) {
	if len(p.writes) == 0 || len(p.chunks) == 0 {
		return 0, nil
	}
	p.reads++
	n := copy(buf, p.chunks[0])
	if n == len(p.chunks[0]) {
		p.chunks = p.chunks[1:]
	} else {
		p.chunks[0] = p.chunks[0][n:]
	}
	return n, nil
}

func TestSerialATWaitsForUSSDNetworkReply(t *testing.T) {
	for _, chunks := range [][]string{
		{"\r\nOK\r\n", "\r\n+CUSD: 0,\"Balance 12.50\",15\r\n"},
		{"\r\n+CUSD: 0,\"Balance 12.50\",15\r\n", "\r\nOK\r\n"},
		{"\r\nOK\r\n+CUSD: 1,\"Menu\r\n", "OK\r\nERROR\r\n", "1. Balance\",15\r", "\n"},
		{"\r\nOK\r\n+CUSD: 0,\"Say \\\"OK\\\"\r\n", "then continue\",15\r\n"},
		{"\r\nOK\r\n", "+CUSD: 2\r\n"},
	} {
		t.Run(strings.Join(chunks, "|"), func(t *testing.T) {
			port := &scriptedATPort{chunks: append([]string(nil), chunks...)}
			response, err := (&SerialAT{port: port}).Execute(`AT+CUSD=1,"*109#",15`, time.Second)
			if err != nil || response != strings.Join(chunks, "") {
				t.Fatalf("response=%q err=%v", response, err)
			}
			if port.reads != len(chunks) || len(port.writes) != 1 || port.writes[0] != "AT+CUSD=1,\"*109#\",15\r\n" {
				t.Fatalf("reads=%d writes=%q", port.reads, port.writes)
			}
		})
	}
}

func TestSerialATUSSDOKWithoutReplyIsNotSuccess(t *testing.T) {
	port := &scriptedATPort{chunks: []string{"\r\nOK\r\n"}}
	response, err := (&SerialAT{port: port}).Execute(`AT+CUSD=1,"*109#",15`, 5*time.Millisecond)
	if response != "\r\nOK\r\n" || err == nil || !strings.Contains(err.Error(), "+CUSD") {
		t.Fatalf("response=%q err=%v", response, err)
	}
	if len(port.writes) != 1 {
		t.Fatalf("writes=%q, must not retry or cancel implicitly", port.writes)
	}
}

func TestSerialATCompletesOrdinaryCommandsAndImmediateErrors(t *testing.T) {
	for _, tc := range []struct{ command, response string }{
		{"AT+CREG?", "\r\n+CREG: 0,5\r\nOK\r\n"},
		{"AT+CUSD=1", "\r\nOK\r\n"},
		{"AT+CUSD=2", "\r\nOK\r\n"},
		{`AT+CUSD=1,"*109#",15`, "\r\n+CME ERROR: 100\r\n"},
		{`AT+CUSD=1,"*109#",15`, "\r\nERROR\r\n"},
	} {
		t.Run(tc.command+tc.response, func(t *testing.T) {
			port := &scriptedATPort{chunks: []string{tc.response, "must not read this"}}
			response, err := (&SerialAT{port: port}).Execute(tc.command, time.Second)
			if err != nil || response != tc.response || port.reads != 1 {
				t.Fatalf("response=%q err=%v reads=%d", response, err, port.reads)
			}
		})
	}
}

func TestSerialATOnlyCompletesAtResultLines(t *testing.T) {
	port := &scriptedATPort{chunks: []string{"\r\nBOOK\r\n", "OK\r", "\n"}}
	response, err := (&SerialAT{port: port}).Execute("AT+GMM", time.Second)
	if err != nil || response != "\r\nBOOK\r\nOK\r\n" || port.reads != 3 {
		t.Fatalf("response=%q err=%v reads=%d", response, err, port.reads)
	}
}
