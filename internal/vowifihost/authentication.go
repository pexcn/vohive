package vowifihost

import (
	"github.com/iniwex5/vowifi-go/engine/eap"
	"time"
)

type authenticationContext struct {
	subscription string
	context      *eap.ReauthContext
	used         time.Time
}

// Credentials stay in memory and belong to a device/subscription. A card change
// invalidates the previous context, including late commits by an older session.
func (m *Manager) authenticationContext(deviceID, imsi string) *eap.ReauthContext {
	if imsi == "" {
		return nil
	}
	m.authMu.Lock()
	defer m.authMu.Unlock()
	if m.authContexts == nil {
		m.authContexts = make(map[string]authenticationContext)
	}
	now := time.Now()
	for id, entry := range m.authContexts {
		if now.Sub(entry.used) > 30*time.Minute {
			entry.context.Clear()
			delete(m.authContexts, id)
		}
	}
	if entry, ok := m.authContexts[deviceID]; ok {
		if entry.subscription == imsi {
			entry.used = now
			m.authContexts[deviceID] = entry
			return entry.context
		}
		entry.context.Clear()
		delete(m.authContexts, deviceID)
	}
	if len(m.authContexts) >= 32 {
		return nil
	} // Full AKA remains available.
	context := &eap.ReauthContext{}
	m.authContexts[deviceID] = authenticationContext{subscription: imsi, context: context, used: now}
	return context
}
