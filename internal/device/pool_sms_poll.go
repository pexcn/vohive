package device

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"
)

const qmiSMSZeroTPDUConfirmations = 3

var errQMISMSPollSkipped = errors.New("QMI SMS polling skipped")

type qmiSMSSlot struct {
	storage uint8
	index   uint32
}

type qmiSMSPollScope struct {
	identityGeneration uint64
	iccid              string
	reset              uint64
}

type qmiSMSZeroTPDURecord struct {
	pdu   []byte
	count int
}

// Only stored SMS polling and URC reads share this state. It never reaches the
// database. Serialize their I/O as well, so a poll cannot race a URC or cleanup.
type qmiSMSPollState struct {
	mu    sync.Mutex
	reset atomic.Uint64 // Modem reset can invalidate an in-flight read without waiting.
	scope qmiSMSPollScope
	bad   map[qmiSMSSlot]qmiSMSZeroTPDURecord
}

func (w *Worker) currentQMISMSPollScope() qmiSMSPollScope {
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()
	return qmiSMSPollScope{
		identityGeneration: w.state.Identity.Generation,
		iccid:              normalizeSIMIdentityForCompare(w.state.Identity.ICCID),
		reset:              w.qmiSMSPoll.reset.Load(),
	}
}

// Caller holds qmiSMSPoll.mu for the entire stored SMS operation. Identity
// changes and core resets invalidate both cached slots and late read results.
func (w *Worker) prepareQMISMSPoll() {
	scope := w.currentQMISMSPollScope()
	if scope != w.qmiSMSPoll.scope {
		w.qmiSMSPoll.bad = nil
		w.qmiSMSPoll.scope = scope
	}
}

func (w *Worker) qmiSMSPollCurrent() bool {
	return w.qmiSMSPoll.scope == w.currentQMISMSPollScope()
}

func (s *qmiSMSPollState) blocked(slot qmiSMSSlot) bool {
	return s.bad[slot].count >= qmiSMSZeroTPDUConfirmations
}

func (s *qmiSMSPollState) confirmZeroTPDU(slot qmiSMSSlot, pdu []byte) int {
	record := s.bad[slot]
	if !bytes.Equal(record.pdu, pdu) {
		record = qmiSMSZeroTPDURecord{pdu: bytes.Clone(pdu)}
	}
	if record.count < qmiSMSZeroTPDUConfirmations {
		record.count++
	}
	if s.bad == nil {
		s.bad = make(map[qmiSMSSlot]qmiSMSZeroTPDURecord)
	}
	s.bad[slot] = record
	return record.count
}

// Use only after decoding failed. Recognize zero-filled data, not its physical
// cause. A nonempty SMSC must have a numeric address, not just a plausible size.
func isCorruptedZeroTPDU(pdu []byte) bool {
	if len(pdu) == 0 {
		return false
	}
	allZero := func(data []byte) bool {
		for _, b := range data {
			if b != 0 {
				return false
			}
		}
		return true
	}
	if allZero(pdu) {
		return true
	}
	n := int(pdu[0])
	if n < 2 || n > 12 || 1+n >= len(pdu) || pdu[1]&0x80 == 0 || pdu[1]&0x70 == 0x50 {
		return false
	}
	for i, b := range pdu[2 : 1+n] {
		if b&0x0f > 9 || (b>>4 > 9 && !(b>>4 == 0xf && i == n-2)) {
			return false
		}
	}
	return allZero(pdu[1+n:])
}
