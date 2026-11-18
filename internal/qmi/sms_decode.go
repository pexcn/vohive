package qmicore

import (
	"errors"
	"fmt"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/vohive/pkg/smscodec"
	"github.com/warthog618/sms/encoding/tpdu"
)

// DecodeIncomingSMSPDU accepts a full PDU, a direct TPDU or incoming RP-DATA.
// Validate each candidate before decoding: SMSC length/TOA bytes can otherwise
// be mistaken for a DELIVER header and a huge address in a padded storage slot.
func DecodeIncomingSMSPDU(raw []byte, storage uint8, index uint32) (*qmimanager.DecodedSMS, error) {
	candidates := [][]byte{raw}
	if len(raw) > 1 {
		n := int(raw[0])
		if n == 0 || (n >= 2 && raw[1]&0x80 != 0) {
			if 1+n < len(raw) {
				candidates = append(candidates, raw[1+n:])
			}
		}
	}
	if len(raw) >= 5 && raw[0] == 0x01 {
		i := 2 // RP-MTI and RP-MR, followed by two length-prefixed addresses.
		for addr := 0; addr < 2 && i < len(raw); addr++ {
			i += 1 + int(raw[i])
		}
		if i < len(raw) {
			n := int(raw[i])
			if n > 0 && n <= len(raw)-i-1 {
				candidates = append(candidates, raw[i+1:i+1+n])
			}
		}
	}

	var errs []error
	for _, candidate := range candidates {
		if err := validateIncomingDeliver(candidate); err != nil {
			errs = append(errs, err)
			continue
		}
		sender, text, ts, concat, err := smscodec.DecodeDeliverTPDU(candidate)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return &qmimanager.DecodedSMS{
			Index: index, Storage: storage, Sender: sender, Message: text, Timestamp: ts,
			IsConcat: concat.IsConcat, ConcatRef: concat.Ref,
			ConcatTotal: concat.Total, ConcatSeq: concat.Seq,
		}, nil
	}
	return nil, fmt.Errorf("invalid incoming SMS PDU: %w", errors.Join(errs...))
}

func validateIncomingDeliver(raw []byte) error {
	if len(raw) < 3 || raw[0]&0x03 != 0 {
		return fmt.Errorf("not an SMS-DELIVER header")
	}
	// TS 23.040: TP-OA contains at most 20 useful semi-octets, including
	// alphanumeric addresses. Do not impose an E.164-only sender restriction.
	if raw[1] == 0 || raw[1] > 20 || raw[2]&0x80 == 0 {
		return fmt.Errorf("invalid originating address: length=%d toa=0x%02x", raw[1], raw[2])
	}
	end, ok := smscodec.DeliverTPDUDeclaredLength(raw)
	if !ok {
		return fmt.Errorf("truncated or unsupported SMS-DELIVER")
	}
	dateOffset := 3 + (int(raw[1])+1)/2 + 2 // Header, TP-OA, TP-PID and TP-DCS.
	if end-(dateOffset+8) > 140 {
		return fmt.Errorf("SMS user data exceeds 140 octets")
	}
	date := raw[dateOffset : dateOffset+7]
	var fields [6]int
	for i, b := range date {
		if i == 6 {
			b &^= 0x08 // Time-zone sign in the low semi-octet.
		}
		if b&0x0f > 9 || b>>4 > 9 {
			return fmt.Errorf("invalid SMS timestamp BCD")
		}
		if i < 6 {
			fields[i] = int(b&0x0f)*10 + int(b>>4)
		}
	}
	var stamp tpdu.Timestamp
	if err := stamp.UnmarshalBinary(date); err != nil {
		return fmt.Errorf("invalid SMS timestamp: %w", err)
	}
	// The underlying decoder uses time.Date, which silently normalizes invalid
	// dates (including an all-zero SCTS into 1999-11-30).
	if int(stamp.Month()) != fields[1] || stamp.Day() != fields[2] ||
		stamp.Hour() != fields[3] || stamp.Minute() != fields[4] || stamp.Second() != fields[5] {
		return fmt.Errorf("invalid SMS timestamp calendar fields")
	}
	return nil
}
