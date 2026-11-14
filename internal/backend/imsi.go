package backend

import (
	"errors"
	"strings"
)

var ErrPlaceholderIMSI = errors.New("IMSI 为全 9 占位值，尚未获得有效 IMSI")

// NormalizeIMSI rejects the known all-nine placeholder without treating an
// empty result as a missing SIM or changing support for other IMSI formats.
func NormalizeIMSI(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "999999999999999" {
		return "", ErrPlaceholderIMSI
	}
	return value, nil
}
