package qmicore

import (
	"net"
	"testing"

	"github.com/iniwex5/qmi-go/pkg/qmi"
)

func TestIMSBearerSelectsPcoForItsAddressFamily(t *testing.T) {
	settings := qmi.RuntimeSettings{
		IPv4Address: net.ParseIP("192.0.2.2"), IPv6Address: net.ParseIP("2001:db8::2"),
		IPv4DNS1: net.ParseIP("192.0.2.53"), IPv6DNS1: net.ParseIP("2001:db8::53"),
		PCSCFv4: []net.IP{net.ParseIP("192.0.2.1")}, PCSCFv6: []net.IP{net.ParseIP("2001:db8::1")},
		PCSCFDomains: []string{"pcscf.ims.example"}, MTU: 1400,
	}
	b, err := imsBearerSettings(settings, 6)
	if err != nil || len(b.Addresses) != 1 || b.Addresses[0].String() != "2001:db8::2" ||
		len(b.DNS) != 1 || b.DNS[0].String() != "2001:db8::53" || len(b.PCSCF) != 2 ||
		b.PCSCF[0] != "2001:db8::1" || b.PCSCF[1] != "pcscf.ims.example" || b.MTU != 1400 {
		t.Fatalf("unexpected IPv6 IMS settings: %+v, %v", b, err)
	}
	settings.IPv6Address = net.ParseIP("fe80::2")
	if _, err := imsBearerSettings(settings, 6); err == nil {
		t.Fatal("IMS bearer accepted a link-local address")
	}
}

func TestLTEPANIUsesServingPLMNWidthAndDuplex(t *testing.T) {
	cell := qmi.LTECellLocationInfo{MCC: "466", MNC: "01", TAC: 42, GlobalCellID: 0xabcdef, EARFCN: 39150, HasEARFCN: true}
	if got := ltePANI(&cell); got != "3GPP-E-UTRAN-TDD;utran-cell-id-3gpp=46601002A0ABCDEF" {
		t.Fatalf("LTE PANI = %q", got)
	}
	cell.HasEARFCN = false
	if got := ltePANI(&cell); got != "" {
		t.Fatalf("incomplete serving cell produced %q", got)
	}
}

func TestNativeIMSMustBeRegisteredOnTheCellularBearer(t *testing.T) {
	status := qmi.IMSARegistrationStatus{
		HasStatus: true, Status: qmi.IMSARegistrationStateRegistered,
		HasTechnology: true, Technology: qmi.IMSARegistrationTechnologyWWAN,
	}
	if !nativeIMSOwnsWWAN(&status) {
		t.Fatal("native cellular registration was overlooked")
	}
	status.Technology = qmi.IMSARegistrationTechnologyWLAN
	if nativeIMSOwnsWWAN(&status) {
		t.Fatal("native WLAN registration was treated as a cellular conflict")
	}
}
