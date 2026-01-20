package console

import (
	"context"
	"testing"
	"time"
)

func TestStateTransitionsShellToPty(t *testing.T) {
	arb, err := NewSessionArbiter(func(string, ...interface{}) {})
	if err != nil {
		t.Fatalf("arbiter init failed: %v", err)
	}

	if err := arb.EnterShell(); err != nil {
		t.Fatalf("enter shell: %v", err)
	}
	if arb.State() != ModeShell {
		t.Fatalf("expected shell state, got %s", arb.State())
	}

	incoming := make(chan []byte)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := PtySessionConfig{
		Incoming:          incoming,
		Send:              func([]byte) error { return nil },
		HeartbeatInterval: 100 * time.Millisecond,
		ReadDeadline:      20 * time.Millisecond,
		DisableLocalIO:    true,
	}

	_ = arb.RunPtySession(ctx, cfg)

	if arb.State() != ModeIdle {
		t.Fatalf("expected idle after PTY session, got %s", arb.State())
	}
}

func TestHeartbeatTimeoutTriggersDetach(t *testing.T) {
	arb, err := NewSessionArbiter(func(string, ...interface{}) {})
	if err != nil {
		t.Fatalf("arbiter init failed: %v", err)
	}

	incoming := make(chan []byte)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	cfg := PtySessionConfig{
		Incoming:          incoming,
		Send:              func([]byte) error { return nil },
		HeartbeatInterval: 50 * time.Millisecond,
		ReadDeadline:      20 * time.Millisecond,
		DisableLocalIO:    true,
	}

	_ = arb.RunPtySession(ctx, cfg)

	if arb.State() != ModeIdle {
		t.Fatalf("expected idle after watchdog, got %s", arb.State())
	}
}
