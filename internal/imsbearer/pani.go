package imsbearer

import (
	"fmt"
	"strings"
)

// LTEPANI uses only the modem's serving cell data. The SIM's home PLMN
// would be wrong here when the bearer is active on a roaming network.
func LTEPANI(servingPLMN string, tac, cellID, earfcn uint32) string {
	if (len(servingPLMN) != 5 && len(servingPLMN) != 6) ||
		strings.Trim(servingPLMN, "0123456789") != "" || tac > 0xffff || cellID == 0 || cellID >= 1<<28 {
		return ""
	}
	duplex := "FDD"
	switch {
	case earfcn < 36000:
	case earfcn <= 60254:
		duplex = "TDD"
	default:
		return ""
	}
	return fmt.Sprintf("3GPP-E-UTRAN-%s;utran-cell-id-3gpp=%s%04X%07X", duplex, servingPLMN, tac, cellID)
}
