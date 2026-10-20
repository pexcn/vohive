package sim

// Authentication material must never be logged, including short RES values
// and the leading/trailing bytes of keys or complete APDU responses.
func maskHexBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return "[REDACTED]"
}

func maskHexString(s string) string {
	if s == "" {
		return ""
	}
	return "[REDACTED]"
}
