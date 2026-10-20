package sim

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestAuthenticationDiagnosticsNeverExposeBytes(t *testing.T) {
	for _, n := range []int{1, 4, 8, 16, 32, 64} {
		value := bytes.Repeat([]byte{0xab}, n)
		for _, got := range []string{maskHexBytes(value), maskHexString(hex.EncodeToString(value))} {
			if got == "" || strings.Contains(got, "ab") {
				t.Fatalf("%d-byte authentication value not fully redacted", n)
			}
		}
	}
	if maskHexBytes(nil) != "" || maskHexString("") != "" {
		t.Fatal("empty authentication value was not preserved")
	}
}
