package console

import (
	"context"
	"testing"
	"time"
)

func TestRapidShellPtySwitch(t *testing.T) {
	arb, err := NewSessionArbiter(t.Logf)
	if err != nil {
		t.Fatalf("arbiter init failed: %v", err)
	}
	defer arb.Close()

	for i := 0; i < 10; i++ {
		if err := arb.EnterShell(); err != nil {
			t.Fatalf("iteration %d: enter shell failed: %v", i, err)
		}

		incoming := make(chan []byte, 10)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		cfg := PtySessionConfig{
			Incoming:          incoming,
			Send:              func([]byte) error { return nil },
			HeartbeatInterval: 20 * time.Millisecond,
			ReadDeadline:      10 * time.Millisecond,
			DisableLocalIO:    true,
		}

		go func() {
			for j := 0; j < 3; j++ {
				select {
				case incoming <- []byte("test\n"):
				case <-ctx.Done():
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()

		_ = arb.RunPtySession(ctx, cfg)
		cancel()

		if arb.State() != ModeIdle {
			t.Fatalf("iteration %d: expected idle after PTY, got %s", i, arb.State())
		}
	}

	if err := arb.EnterShell(); err != nil {
		t.Fatalf("final shell entry failed: %v", err)
	}
}

func TestBackpressureTriggersDetach(t *testing.T) {
	arb, err := NewSessionArbiter(t.Logf)
	if err != nil {
		t.Fatalf("arbiter init failed: %v", err)
	}
	defer arb.Close()

	incoming := make(chan []byte, 10)
	ctx := context.Background()

	cfg := PtySessionConfig{
		Incoming:          incoming,
		Send:              func([]byte) error { return nil },
		HeartbeatInterval: 100 * time.Millisecond,
		ReadDeadline:      10 * time.Millisecond,
		BackpressureLimit: 5,
		DisableLocalIO:    true,
	}

	go func() {
		for i := 0; i < 20; i++ {
			incoming <- []byte("data")
		}
	}()

	_ = arb.RunPtySession(ctx, cfg)

	if arb.State() != ModeIdle {
		t.Fatalf("expected idle after backpressure, got %s", arb.State())
	}
}

func TestConcurrentLeaseRequestsRejected(t *testing.T) {
	arb, err := NewSessionArbiter(t.Logf)
	if err != nil {
		t.Fatalf("arbiter init failed: %v", err)
	}
	defer arb.Close()

	incoming := make(chan []byte, 10)
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	cfg := PtySessionConfig{
		Incoming:          incoming,
		Send:              func([]byte) error { return nil },
		HeartbeatInterval: 200 * time.Millisecond,
		DisableLocalIO:    true,
	}

	done := make(chan error, 1)
	go func() {
		done <- arb.RunPtySession(ctx1, cfg)
	}()

	time.Sleep(10 * time.Millisecond)

	incoming2 := make(chan []byte)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	cfg2 := PtySessionConfig{
		Incoming:       incoming2,
		Send:           func([]byte) error { return nil },
		DisableLocalIO: true,
	}

	err = arb.RunPtySession(ctx2, cfg2)
	if err != ErrAlreadyActive {
		t.Fatalf("expected ErrAlreadyActive, got: %v", err)
	}

	cancel1()
	<-done
}
