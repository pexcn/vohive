package qmicore

import (
	"testing"

	qmimanager "github.com/iniwex5/qmi-go/pkg/manager"
	"github.com/iniwex5/qmi-go/pkg/qmi"
)

func TestNativeAKASessionScope(t *testing.T) {
	for _, tc := range []struct {
		slot    uint8
		isim    bool
		session uint8
		auth    uint8
		ok      bool
	}{
		{1, false, qmi.UIMSessionTypePrimaryGWProvisioning, qmi.UIMAuthContext3GSecurity, true},
		{1, true, qmi.UIMSessionTypeNonProvisioningSlot1, qmi.UIMAuthContextIMSAKASecurity, true},
		{2, true, qmi.UIMSessionTypeNonProvisioningSlot2, qmi.UIMAuthContextIMSAKASecurity, true},
		{3, true, qmi.UIMSessionTypeNonProvisioningSlot3, qmi.UIMAuthContextIMSAKASecurity, true},
		{0, true, 0, 0, false},
	} {
		session, auth, ok := nativeAKASession(qmimanager.UIMReadiness{ActiveSlot: tc.slot}, tc.isim)
		if session != tc.session || auth != tc.auth || ok != tc.ok {
			t.Fatalf("slot %d, ISIM %v: session %d, context %d, usable %v", tc.slot, tc.isim, session, auth, ok)
		}
	}
}
