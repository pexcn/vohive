package device

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

type qmiControlLock struct {
	gate chan struct{}
	refs int
}

var qmiControlLocks = struct {
	sync.Mutex
	ports map[string]*qmiControlLock
}{ports: make(map[string]*qmiControlLock)}

// acquireQMIControl serializes temporary discovery clients with worker startup.
// Hold it from transport selection until the client is closed or the worker's
// control connection is established. Different control ports remain parallel.
func acquireQMIControl(ctx context.Context, path string) (func(), error) {
	path = strings.TrimSpace(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	qmiControlLocks.Lock()
	lock := qmiControlLocks.ports[path]
	if lock == nil {
		lock = &qmiControlLock{gate: make(chan struct{}, 1)}
		qmiControlLocks.ports[path] = lock
	}
	lock.refs++
	qmiControlLocks.Unlock()

	unref := func() {
		qmiControlLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(qmiControlLocks.ports, path)
		}
		qmiControlLocks.Unlock()
	}
	select {
	case lock.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock.gate
			unref()
			return nil, err
		}
		return func() { <-lock.gate; unref() }, nil
	case <-ctx.Done():
		unref()
		return nil, ctx.Err()
	}
}
