package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundDataOptInPersistsAndCanBeRevoked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("devices: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := DeviceConfig{ID: "manual-data", AutoConnectData: true}
	if err := AddDeviceInFile(path, d); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		d.AutoConnectData = enabled
		if err := UpdateDeviceInFile(path, d.ID, d); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if got.Devices[0].AutoConnectData != enabled {
			t.Fatalf("background data opt-in=%v not persisted", enabled)
		}
	}
}
