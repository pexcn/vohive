package vowifihost

import "testing"

func TestAuthenticationContextSubscriptionIsolation(t *testing.T) {
	m := NewManager()
	if m.authenticationContext("device", "") != nil {
		t.Fatal("unknown SIM must not acquire credentials")
	}
	a := m.authenticationContext("device", "001010000000001")
	if a == nil || m.authenticationContext("device", "001010000000001") != a {
		t.Fatal("same subscription must retain reconnect context")
	}
	if m.authenticationContext("other", "001010000000001") == a {
		t.Fatal("different devices shared authentication state")
	}
	b := m.authenticationContext("device", "001010000000002")
	if b == a {
		t.Fatal("SIM switch reused credentials")
	}
	if m.authenticationContext("device", "001010000000001") == a {
		t.Fatal("old SIM credential slot resurrected")
	}
}
