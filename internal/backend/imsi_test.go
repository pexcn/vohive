package backend

import (
	"context"
	"errors"
	"testing"

	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/iniwex5/vohive/pkg/mbim"
)

func TestNormalizeIMSIRejectsOnlyExactPlaceholder(t *testing.T) {
	for _, value := range []string{"460001234567890", "999001234567890", "99999999999999", "", " 460001234567890 "} {
		got, err := NormalizeIMSI(value)
		if err != nil || (value != "" && got == "") {
			t.Fatalf("NormalizeIMSI(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"999999999999999", " 999999999999999 "} {
		if got, err := NormalizeIMSI(value); got != "" || !errors.Is(err, ErrPlaceholderIMSI) {
			t.Fatalf("NormalizeIMSI(%q) = %q, %v", value, got, err)
		}
	}
}

type imsiQMISource struct {
	qmiBackendSendSourceStub
	snapshot *qmimanager.DeviceSnapshot
	imsi     string
	err      error
	calls    int
}

func (s *imsiQMISource) GetDeviceSnapshot() *qmimanager.DeviceSnapshot { return s.snapshot }
func (s *imsiQMISource) GetIMSI(context.Context) (string, error) {
	s.calls++
	return s.imsi, s.err
}

type strictIMSISource struct{ imsiQMISource }

func (s *strictIMSISource) GetIMSIStrictLive(ctx context.Context) (string, error) {
	return s.imsiQMISource.GetIMSI(ctx)
}

func TestQMIIMSIRejectsPlaceholderAndRecoversFromInvalidSnapshot(t *testing.T) {
	snapshot := &qmimanager.DeviceSnapshot{}
	snapshot.UpdateIdentities(qmimanager.DeviceIdentities{IMEI: "861234567890123", ICCID: "8986000000000000001", IMSI: "999999999999999"})
	source := &imsiQMISource{snapshot: snapshot, imsi: "999999999999999"}
	b, err := NewQMIBackend("/dev/test", source)
	if err != nil {
		t.Fatal(err)
	}
	for _, read := range []func(context.Context) (string, error){b.GetIMSI, b.GetIMSILive} {
		if got, err := read(context.Background()); got != "" || !errors.Is(err, ErrPlaceholderIMSI) {
			t.Fatalf("placeholder IMSI accepted: %q, %v", got, err)
		}
	}
	if mcc, mnc, err := b.GetNativeMCCMNC(context.Background()); mcc != "" || mnc != "" || !errors.Is(err, ErrPlaceholderIMSI) {
		t.Fatalf("placeholder used for home PLMN: %s/%s, %v", mcc, mnc, err)
	}
	source.imsi = "460001234567890"
	if got, err := b.GetIMSI(context.Background()); err != nil || got != source.imsi {
		t.Fatalf("invalid snapshot prevented recovery: %q, %v", got, err)
	}
	snapshot.UpdateIdentities(qmimanager.DeviceIdentities{IMSI: source.imsi})
	calls := source.calls
	if got, err := b.GetIMSI(context.Background()); err != nil || got != source.imsi || source.calls != calls {
		t.Fatalf("normal snapshot no longer reused: %q, %v, calls=%d", got, err, source.calls)
	}
}

func TestQMIIMSIStrictLiveRejectsPlaceholderAndPreservesReadErrors(t *testing.T) {
	source := &strictIMSISource{imsiQMISource: imsiQMISource{imsi: "999999999999999"}}
	b, err := NewQMIBackend("/dev/test", source)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := b.GetIMSILive(context.Background()); got != "" || !errors.Is(err, ErrPlaceholderIMSI) {
		t.Fatalf("strict placeholder accepted: %q, %v", got, err)
	}
	source.err = context.DeadlineExceeded
	if got, err := b.GetIMSILive(context.Background()); got != "" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("transport error lost: %q, %v", got, err)
	}
}

func TestMBIMIMSIRejectsPlaceholderWithoutLosingICCID(t *testing.T) {
	source := &fakeMBIMSource{sub: mbim.SubscriberReady{IMSI: "999999999999999", ICCID: "8986000000000000001"}}
	b := NewMBIMBackend("/dev/test", source)
	if got, err := b.GetIMSILive(context.Background()); got != "" || !errors.Is(err, ErrPlaceholderIMSI) {
		t.Fatalf("placeholder accepted: %q, %v", got, err)
	}
	if got, err := b.GetICCIDLive(context.Background()); err != nil || got != source.sub.ICCID {
		t.Fatalf("IMSI rejection affected ICCID: %q, %v", got, err)
	}
	source.sub.IMSI = "460001234567890"
	if got, err := b.GetIMSI(context.Background()); err != nil || got != source.sub.IMSI {
		t.Fatalf("normal IMSI did not recover: %q, %v", got, err)
	}
}
