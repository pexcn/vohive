package qmicore

import (
	"net"
	"testing"
)

func TestPublicIPReadinessRequiresADefaultRoute(t *testing.T) {
	if !isPublicIPDefaultRoute(nil) {
		t.Fatal("netlink nil destination default route was not recognized")
	}
	for _, tc := range []struct {
		cidr string
		want bool
	}{
		{"0.0.0.0/0", true}, {"::/0", true},
		{"0.0.0.0/1", false}, {"2001:db8::/32", false},
	} {
		_, destination, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatal(err)
		}
		if got := isPublicIPDefaultRoute(destination); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.cidr, got, tc.want)
		}
	}
}
