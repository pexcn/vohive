package esim

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestGetProfilesSharesPendingBackgroundOverview(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var loads atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		loads.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{SkuName: "fresh chip"},
			Profiles: []EUICCProfiles{{
				AIDHex:   "A00001",
				Profiles: []ProfileItem{{ICCID: "iccid-1", Name: "saved name"}},
			}},
		}, nil
	})
	mgr.overviewCache = &EsimOverview{Profiles: []EUICCProfiles{{
		AIDHex: "A00001", Profiles: []ProfileItem{{ICCID: "iccid-1", Name: "old name"}},
	}}}
	// Successful writes invalidate the cache and start an asynchronous reload
	// before the browser asks for the updated profile list.
	mgr.invalidateOverviewCache("rename_profile")
	mgr.WarmOverviewAsync("rename_profile")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("background overview did not start")
	}

	type result struct {
		groups []EUICCProfiles
		err    error
	}
	const readers = 8
	results := make(chan result, readers)
	for range readers {
		go func() {
			groups, err := mgr.GetProfiles()
			results <- result{groups, err}
		}()
	}
	select {
	case got := <-results:
		close(release)
		t.Fatalf("profile reader returned before the background scan completed: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	for range readers {
		select {
		case got := <-results:
			if got.err != nil || len(got.groups) != 1 || len(got.groups[0].Profiles) != 1 || got.groups[0].Profiles[0].Name != "saved name" {
				t.Fatalf("profile reader got %+v, want saved name", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("profile reader did not finish")
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("overview scans = %d, want one shared scan", loads.Load())
	}
}
