package vowifihost

import (
	"context"
	"errors"
	"sync"
)

type runtimeCleanup interface{ Stop(context.Context) error }
type retainedCleanup struct {
	mu        sync.Mutex
	instances []runtimeCleanup
}

func (m *Manager) cleanupFor(deviceID string, create bool) *retainedCleanup {
	m.cleanupMu.Lock()
	defer m.cleanupMu.Unlock()
	if m.failedCleanup == nil {
		if !create {
			return nil
		}
		m.failedCleanup = make(map[string]*retainedCleanup)
	}
	entry := m.failedCleanup[deviceID]
	if entry == nil && create {
		entry = &retainedCleanup{}
		m.failedCleanup[deviceID] = entry
	}
	return entry
}
func (m *Manager) retainCleanup(deviceID string, instance runtimeCleanup) {
	entry := m.cleanupFor(deviceID, true)
	entry.mu.Lock()
	entry.instances = append(entry.instances, instance)
	entry.mu.Unlock()
}

// A failed startup is not an active runtime, but its unclosed resources still
// have an owner. Retry them before a replacement may acquire the same device.
func (m *Manager) retryCleanup(ctx context.Context, deviceID string) error {
	entry := m.cleanupFor(deviceID, false)
	if entry == nil {
		return nil
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	var pending []runtimeCleanup
	var errs []error
	for _, instance := range entry.instances {
		if err := instance.Stop(ctx); err != nil {
			pending = append(pending, instance)
			errs = append(errs, err)
		}
	}
	entry.instances = pending
	return errors.Join(errs...)
}
