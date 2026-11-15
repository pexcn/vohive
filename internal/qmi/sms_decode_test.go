package qmicore

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/warthog618/sms"
	"github.com/warthog618/sms/encoding/tpdu"
)

func deliverFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := hex.DecodeString("040891214365870000621151710000000141")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDecodeIncomingSMSPDUFormats(t *testing.T) {
	direct := deliverFixture(t)
	full := append([]byte{8, 0x91, 0, 0x96, 0x09, 0x40, 0x61, 0x01, 0x01}, direct...)
	full = append(full, make([]byte, 176-len(full))...)
	full[75] = 0xf0 // Allows the old decoder's false 145-digit address to pass.
	legacy, err := qmimanager.DecodeIncomingSMSPDU(full, 1, 7)
	if err != nil || len(legacy.Sender) != 145 || legacy.Message != "" || legacy.Timestamp.Year() != 1999 {
		t.Fatalf("fixture must reproduce the old false success: sms=%+v err=%v", legacy, err)
	}

	for name, raw := range map[string][]byte{
		"direct":                            direct,
		"zero_smsc":                         append([]byte{0}, direct...),
		"padded_smsc_with_ambiguous_header": full,
		"rp_data":                           append([]byte{1, 0x23, 2, 0x91, 0x21, 0, byte(len(direct))}, direct...),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeIncomingSMSPDU(raw, 1, 7)
			if err != nil {
				t.Fatal(err)
			}
			if got.Sender != "+12345678" || got.Message != "A" || got.Timestamp.Format(time.DateTime) != "2026-11-15 17:00:00" || got.Storage != 1 || got.Index != 7 {
				t.Fatalf("unexpected decoded SMS: %+v", got)
			}
		})
	}
}

func TestDecodeIncomingSMSPDURejectsInvalidFields(t *testing.T) {
	for _, name := range []string{"empty", "truncated", "address_length", "toa", "zero_timestamp", "bcd", "month", "day", "hour", "udl"} {
		t.Run(name, func(t *testing.T) {
			raw := deliverFixture(t)
			switch name {
			case "empty":
				raw = nil
			case "truncated":
				raw = raw[:len(raw)-1]
			case "address_length":
				raw[1] = 145
			case "toa":
				raw[2] = 0x11
			case "zero_timestamp":
				clear(raw[9:16])
			case "bcd":
				raw[9] = 0xfa
			case "month":
				raw[10] = 0
			case "day":
				raw[10], raw[11] = 0x20, 0x03 // February 30.
			case "hour":
				raw[12] = 0x42
			case "udl":
				raw[16] = 160
			}
			if got, err := DecodeIncomingSMSPDU(raw, 1, 0); err == nil {
				t.Fatalf("accepted malformed PDU: %+v", got)
			}
		})
	}
}

func TestDecodeIncomingSMSPDUKeepsEmptyMessages(t *testing.T) {
	raw := deliverFixture(t)
	raw[16] = 0
	raw = raw[:17]
	got, err := DecodeIncomingSMSPDU(raw, 0, 3)
	if err != nil || got.Message != "" || got.Sender != "+12345678" {
		t.Fatalf("valid empty SMS rejected: sms=%+v err=%v", got, err)
	}
}

func TestDecodeIncomingSMSPDUPreservesAlphanumericUnicodeAndConcat(t *testing.T) {
	for _, text := range []string{"hello", "你好", strings.Repeat("hello", 40)} {
		pdus, err := sms.Encode([]byte(text), sms.AsDeliver)
		if err != nil {
			t.Fatal(err)
		}
		var combined bytes.Buffer
		for i, pdu := range pdus {
			pdu.OA = tpdu.Address{TOA: 0xd0, Addr: "RedteaGO"}
			pdu.SCTS.Time = time.Date(2026, 11, 15, 17, 0, 0, 0, time.FixedZone("test", -5*3600))
			raw, err := pdu.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			got, err := DecodeIncomingSMSPDU(raw, 1, uint32(i))
			if err != nil {
				t.Fatal(err)
			}
			if got.Sender != "RedteaGO" || !got.Timestamp.Equal(pdu.SCTS.Time) {
				t.Fatalf("lost address or timestamp: %+v", got)
			}
			if len(pdus) > 1 && (!got.IsConcat || got.ConcatTotal != len(pdus) || got.ConcatSeq != i+1) {
				t.Fatalf("lost concatenation metadata: %+v", got)
			}
			combined.WriteString(got.Message)
		}
		if combined.String() != text {
			t.Fatalf("decoded %q, want %q", combined.String(), text)
		}
	}
}
