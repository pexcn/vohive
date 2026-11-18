package backend

import (
	"testing"

	"github.com/iniwex5/qmi-go/pkg/qmi"
)

// QMI NAS 的 LTE RSSNR / 5G NR SINR 单位是 0.1 dB 缩放整数，qmiSNRToDB 应四舍五入为 dB 整数。
func TestQMISNRToDB(t *testing.T) {
	cases := []struct {
		raw  int16
		want int
	}{
		{134, 13}, // 13.4 → 13（截图里的异常值，修复前显示 134）
		{135, 14}, // 13.5 → 14（四舍五入进位）
		{0, 0},    // 守卫处已过滤，但保证安全
		{4, 0},    // 0.4 → 0
		{5, 1},    // 0.5 → 1
		{300, 30}, // 30.0 dB
		{-15, -2}, // -1.5 → -2
		{-23, -2}, // -2.3 → -2
	}
	for _, c := range cases {
		if got := qmiSNRToDB(c.raw); got != c.want {
			t.Errorf("qmiSNRToDB(%d)=%d want %d", c.raw, got, c.want)
		}
	}
}

func TestApplyQMISignalInfoMapsLTEAndNRMeasurements(t *testing.T) {
	lteRSRP, lteRSRQ, lteSNR := int16(-95), int16(-9), int16(135)
	nrRSRP, nrRSRQ, nrSNR := int16(-101), int16(-12), int16(-15)
	info := SignalInfo{RSSI: -65}
	applyQMISignalInfo(&info, &qmi.SignalInfo{
		LTE:  &qmi.LTESignalInfo{RSRP: &lteRSRP, RSRQ: &lteRSRQ, SNR: &lteSNR},
		NR5G: &qmi.NR5GSignalInfo{RSRP: &nrRSRP, RSRQ: &nrRSRQ, SNR: &nrSNR},
	})
	want := SignalInfo{RSSI: -65, RSRP: -95, RSRQ: -9, SINR: 14, NR5GRSRP: -101, NR5GRSRQ: -12, NR5GSINR: -2}
	if info != want {
		t.Fatalf("signal=%+v want %+v", info, want)
	}
}

func TestApplyQMISignalInfoPreservesMissingMeasurements(t *testing.T) {
	info := SignalInfo{RSRP: -95, RSRQ: -9, SINR: 14, NR5GRSRP: -101}
	zero := int16(0)
	applyQMISignalInfo(&info, &qmi.SignalInfo{LTE: &qmi.LTESignalInfo{SNR: &zero}})
	want := SignalInfo{RSRP: -95, RSRQ: -9, SINR: 0, NR5GRSRP: -101}
	if info != want {
		t.Fatalf("signal=%+v want present zero and retained missing fields %+v", info, want)
	}
	applyQMISignalInfo(&info, &qmi.SignalInfo{})
	if info != want {
		t.Fatalf("absent RAT measurements changed signal to %+v", info)
	}
}
