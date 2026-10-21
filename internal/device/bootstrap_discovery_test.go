package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qmiq "github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/iniwex5/vohive/internal/config"
)

func TestQMIBootstrapBatchSharesIdentityProbes(t *testing.T) {
	stubQMIProbe(t)
	var devices []QMIDevice
	for i := range 2 {
		path := filepath.Join(t.TempDir(), "control")
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		devices = append(devices, QMIDevice{ControlPath: path, NetInterface: fmt.Sprintf("wwan%d", i)})
	}
	original := discoverQMIDevicesFn
	t.Cleanup(func() { discoverQMIDevicesFn = original })
	var discoveries, probes atomic.Int32
	discoverQMIDevicesFn = func() ([]QMIDevice, error) {
		discoveries.Add(1)
		return devices, nil
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var started, released sync.Once
	unblock := func() { released.Do(func() { close(release) }) }
	defer unblock()
	probeIMEIViaQMIFn = func(path string, opts qmiq.ClientOptions) (string, error) {
		probes.Add(1)
		started.Do(func() { close(entered) })
		<-release
		if opts.Logf == nil || !opts.QueryVersionOnOpen {
			t.Error("probe logging or service-version query was lost")
		}
		return "111111111111111", nil
	}
	p := NewPool(&config.Config{})
	defer p.cancel()
	cache := &qmiBootstrapIdentityCache{}
	done := make(chan struct{}, 2)
	for i := range 2 {
		go func() {
			defer func() { done <- struct{}{} }()
			// Deliberately unmatched identities stop before opening real hardware.
			p.startConfiguredDeviceBootstrap(config.DeviceConfig{
				ID: fmt.Sprintf("test-%d", i), DeviceBackend: "qmi", ModemIMEI: "999999999999999",
			}, "test", cache)
		}()
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("bootstrap did not probe a candidate")
	}
	unblock()
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("parallel bootstraps did not complete")
		}
	}
	if probes.Load() != 2 || discoveries.Load() != 2 || len(p.GetAllWorkers()) != 0 {
		t.Fatalf("probes=%d discoveries=%d workers=%d", probes.Load(), discoveries.Load(), len(p.GetAllWorkers()))
	}
}

func TestQMIBootstrapIdentityCacheBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	probe := func(dev QMIDevice, _ time.Duration, allow bool) (QMIDevice, string) {
		if !allow {
			return dev, ""
		}
		calls++
		if calls == 1 {
			return dev, "" // Do not cache a failed identity read.
		}
		return dev, fmt.Sprintf("%015d", calls)
	}
	cache := &qmiBootstrapIdentityCache{}
	resolve := cache.wrapResolver(t.Context(), probe)
	dev := QMIDevice{ControlPath: path}
	_, imei := resolve(dev, time.Second, true)
	if imei != "" {
		t.Fatal("first probe unexpectedly succeeded")
	}
	_, imei = resolve(dev, time.Second, true)
	_, again := resolve(dev, time.Second, true)
	if calls != 2 || imei != "000000000000002" || again != imei {
		t.Fatal("failed read was cached or successful read was not shared")
	}
	if _, suppressed := resolve(dev, time.Second, false); suppressed != "" || calls != 2 {
		t.Fatal("cache bypassed the caller's probe policy")
	}
	alias := path + "-alias"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, got := resolve(QMIDevice{ControlPath: alias}, time.Second, true); got != imei || calls != 2 {
		t.Fatal("control-port alias caused a duplicate probe")
	}
	// Keep the old inode alive so the replacement is unambiguously different.
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, got := resolve(dev, time.Second, true); got != "000000000000003" || calls != 3 {
		t.Fatal("replacement device reused the old IMEI")
	}
	nextBatch := &qmiBootstrapIdentityCache{}
	if _, got := nextBatch.wrapResolver(t.Context(), probe)(dev, time.Second, true); got != "000000000000004" || calls != 4 {
		t.Fatal("new startup batch reused a prior identity")
	}
}

func TestQMIBootstrapIdentityWaitCancellationAndIndependentPorts(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	firstDone := make(chan struct{})
	cache := &qmiBootstrapIdentityCache{}
	first := QMIDevice{ControlPath: filepath.Join(t.TempDir(), "first")}
	probe := func(dev QMIDevice, _ time.Duration, _ bool) (QMIDevice, string) {
		if dev.ControlPath == first.ControlPath {
			close(entered)
			<-release
		}
		return dev, "111111111111111"
	}
	go func() {
		defer close(firstDone)
		cache.wrapResolver(t.Context(), probe)(first, time.Second, true)
	}()
	defer func() { close(release); <-firstDone }()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan string, 1)
	go func() {
		_, imei := cache.wrapResolver(ctx, probe)(first, time.Second, true)
		done <- imei
	}()
	select {
	case imei := <-done:
		if imei != "" {
			t.Fatal("cancelled waiter returned an identity")
		}
	case <-time.After(time.Second):
		t.Fatal("identity wait ignored cancellation")
	}
	go func() {
		_, imei := cache.wrapResolver(t.Context(), probe)(QMIDevice{ControlPath: filepath.Join(t.TempDir(), "other")}, time.Second, true)
		done <- imei
	}()
	select {
	case imei := <-done:
		if imei == "" {
			t.Fatal("unrelated port did not resolve")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked probe serialized unrelated control ports")
	}
}
