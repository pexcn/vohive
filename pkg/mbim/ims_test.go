package mbim

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestIMSSessionConnectEncoding(t *testing.T) {
	data := encodeConnectWithType(1, ActivationCommandActivate, "ims", "", "", AuthProtocolNone, ContextIPTypeIPv6, UUIDContextTypeIMS)
	if got := le.Uint32(data); got != 1 {
		t.Fatalf("session = %d", got)
	}
	if got := UUID(data[44:60]); got != UUIDContextTypeIMS {
		t.Fatalf("context = %v", got)
	}
	if got := le.Uint32(data[40:]); got != ContextIPTypeIPv6 {
		t.Fatalf("IP type = %d", got)
	}
}

func TestPCOForSeparateIMSSession(t *testing.T) {
	pco := []byte{0x80, 0x00, 0x0c, 0x04, 10, 1, 2, 3, 0x00, 0x0d, 0x04, 8, 8, 8, 8}
	got, err := parsePCSCFOptions(pco)
	want := []netip.Addr{netip.MustParseAddr("10.1.2.3")}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("P-CSCF from PCO = %v, %v, want %v", got, err, want)
	}
	if _, err := parsePCSCFOptions(pco[:len(pco)-1]); err == nil {
		t.Fatal("accepted truncated PCO")
	}
}
