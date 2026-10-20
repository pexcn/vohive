package vowifihost

import (
	"context"
	"errors"
	"testing"
	"time"
)

type switchCleanupFailure struct{ fail bool }

func (s *switchCleanupFailure) Stop(context.Context) error {
	if s.fail {
		return errors.New("logical channel still owned")
	}
	return nil
}

func TestSwitchBeginRejectsUnfinishedCleanup(t *testing.T) {
	m := NewManager()
	pending := &switchCleanupFailure{fail: true}
	m.retainCleanup("dev", pending)
	if err := m.SwitchBegin(context.Background(), "dev"); err == nil {
		t.Fatal("switch preparation succeeded with unreleased resources")
	}
	pending.fail = false
	if err := m.SwitchBegin(context.Background(), "dev"); err != nil {
		t.Fatalf("switch still blocked after cleanup succeeded: %v", err)
	}
}

func TestSwitchBeginStopsWaitingAtDeadline(t *testing.T) {
	c := NewLifecycleController()
	lifecycle := c.device("dev")
	lifecycle.runMu.Lock()
	defer lifecycle.runMu.Unlock()
	c.TestRun = func(context.Context, LifecycleCommand) error {
		t.Fatal("switch started while the old run still owned resources")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.Submit(ctx, LifecycleCommand{DeviceID: "dev", Kind: LifecycleCommandSwitchBegin}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("switch did not respect cleanup deadline: %v", err)
	}
}
