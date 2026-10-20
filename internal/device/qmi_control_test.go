package device

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	qmiq "github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/iniwex5/vohive/internal/config"
)

func stubQMIProbe(t *testing.T) {
	t.Helper()
	originalProbe, originalOptions := probeIMEIViaQMIFn, qmiDiscoveryClientOptionsFn
	t.Cleanup(func() {
		probeIMEIViaQMIFn, qmiDiscoveryClientOptionsFn = originalProbe, originalOptions
	})
	qmiDiscoveryClientOptionsFn = func(string) (qmiq.ClientOptions, bool) {
		return qmiq.DefaultClientOptions(), true
	}
}

func TestQMIControlSerializesDiscoveryCandidates(t *testing.T) {
	stubQMIProbe(t)
	entered := make(chan string, 3)
	release := make(chan struct{})
	var wg sync.WaitGroup
	defer func() { close(release); wg.Wait() }()
	probeIMEIViaQMIFn = func(path string, opts qmiq.ClientOptions) (string, error) {
		if opts.UseProxy {
			t.Error("temporary probes incorrectly selected proxy")
		}
		entered <- path
		<-release
		return "000000000000001", nil
	}
	start := func(path string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolveDiscoveredQMIDeviceWithConfig(QMIDevice{ControlPath: path}, time.Second, true,
				config.DeviceConfig{DeviceBackend: "qmi", ControlDevice: "/stale/configured/path"})
		}()
	}
	first, other := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "other")
	start(first)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first discovery probe did not start")
	}
	start(first)
	start(other)
	select {
	case path := <-entered:
		if path != other {
			t.Fatal("two devices probed the same candidate simultaneously")
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated control port was blocked")
	}
	select {
	case <-entered:
		t.Fatal("second probe entered before first client closed")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestQMIControlProbeRechecksOccupancyAfterBootstrap(t *testing.T) {
	stubQMIProbe(t)
	path := filepath.Join(t.TempDir(), "control")
	release, err := acquireQMIControl(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var occupied atomic.Bool
	qmiDiscoveryClientOptionsFn = func(actual string) (qmiq.ClientOptions, bool) {
		if actual != path {
			t.Errorf("checked %q instead of actual candidate %q", actual, path)
		}
		return qmiq.DefaultClientOptions(), !occupied.Load()
	}
	probeIMEIViaQMIFn = func(string, qmiq.ClientOptions) (string, error) {
		t.Error("opened a temporary client on a control port already taken by bootstrap")
		return "", nil
	}
	done := make(chan string, 1)
	go func() {
		_, imei := resolveDiscoveredQMIDeviceWithConfig(QMIDevice{ControlPath: path}, time.Second, true,
			config.DeviceConfig{DeviceBackend: "qmi", ControlDevice: "/stale/configured/path"})
		done <- imei
	}()
	// The worker now owns a raw connection when its bootstrap releases the gate.
	occupied.Store(true)
	release()
	select {
	case imei := <-done:
		if imei != "" {
			t.Fatalf("discovery returned an IMEI for the occupied port: %q", imei)
		}
	case <-time.After(time.Second):
		t.Fatal("discovery did not finish after bootstrap")
	}
}

func TestQMIControlBootstrapWaitsForProbeAndRechecksOwner(t *testing.T) {
	stubQMIProbe(t)
	path := filepath.Join(t.TempDir(), "control")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	p := NewPool(&config.Config{})
	defer p.cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	probeDone := make(chan struct{})
	probeIMEIViaQMIFn = func(string, qmiq.ClientOptions) (string, error) {
		close(entered)
		<-release
		p.mu.Lock()
		p.workers["owner"] = &Worker{ID: "owner", Config: config.DeviceConfig{ControlDevice: path}}
		p.mu.Unlock()
		return "000000000000001", nil
	}
	go func() {
		defer close(probeDone)
		_, _ = ProbeIMEIViaQMI(path)
	}()
	<-entered
	done := make(chan error, 1)
	go func() {
		_, err := p.AddWorkerFromConfig(config.DeviceConfig{ID: "new", DeviceBackend: "qmi", ControlDevice: path, Interface: "wwan-test"})
		done <- err
	}()
	select {
	case err := <-done:
		close(release)
		<-probeDone
		t.Fatalf("bootstrap opened the candidate during discovery: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	<-probeDone
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "已由设备 owner 接管") {
			t.Fatalf("bootstrap failed to recheck current owner: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap did not finish after probe closed")
	}
}

func TestQMIControlWaitCancellationAndAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := path + "-alias"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	release, err := acquireQMIControl(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if unlock, err := acquireQMIControl(ctx, alias); err != context.DeadlineExceeded {
		if unlock != nil {
			unlock()
		}
		t.Fatalf("alias did not wait for same port with cancellation: %v", err)
	}
}
