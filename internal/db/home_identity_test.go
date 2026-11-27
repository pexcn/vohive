package db

import "testing"

func TestSIMHomeIdentityRemainsBoundToICCID(t *testing.T) {
	initPhoneNumberTestDB(t)
	const iccid = "8944000000000000041"
	for _, input := range [][3]string{
		{"310260000000041", "234", "15"}, // Inconsistent identity, ignore.
		{"234015000000041", "234", "015"},
		{"310260000000041", "310", "260"},
	} {
		if err := RecordSIMHomeIdentity(iccid+"FF", input[0], input[1], input[2]); err != nil {
			t.Fatal(err)
		}
	}
	home, err := GetSIMHomeIdentity(iccid)
	if err != nil || home.MCC != "234" || home.MNC != "015" || home.IMSI != "234015000000041" {
		t.Fatalf("home identity = %+v, %v", home, err)
	}
	if err := RecordPreferredSIMHomeIdentity(iccid, "310260000000041", "310", "260"); err != nil {
		t.Fatal(err)
	}
	home, err = GetSIMHomeIdentity(iccid)
	if err != nil || home.IMSI != "310260000000041" || home.MCC != "310" || home.MNC != "260" {
		t.Fatalf("preferred home identity = %+v, %v", home, err)
	}

	other, err := GetSIMHomeIdentity("8944000000000000042")
	if err != nil || other != (SIMHomeIdentity{}) {
		t.Fatalf("another SIM inherited identity %+v, %v", other, err)
	}
}
