package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestPtyReentry tests the scenario: enter PTY shell -> type commands -> exit with Ctrl-D -> immediately re-enter PTY shell
// This verifies that the client and listener properly reset state after PTY exit, allowing re-entry on the first try
func TestPtyReentry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}

	port := freePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, port, "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, fmt.Sprintf("127.0.0.1:%s", port), "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	// List clients
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)

	// --- First PTY session ---
	// Enter shell
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)

	// Send a command with a newline to interact
	send(listener, "echo test\n")
	time.Sleep(500 * time.Millisecond) // Give shell time to process

	// Exit shell with Ctrl-D (0x04)
	send(listener, "\x04")
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)
	waitForContains(t, listener, "listener>", 5*time.Second)

	// --- Second PTY session (should work on first try, not require a retry) ---
	// Immediately re-enter shell
	send(listener, "shell 1\n")
	
	// This should succeed on the first try, not give "Failed to enter PTY mode: Exited PTY mode"
	waitForContains(t, listener, "PTY shell active", 5*time.Second)
	if strings.Contains(listener.snapshot(), "Failed to enter PTY mode") {
		t.Fatalf("Second PTY session failed to enter on first try; output:\n%s", listener.snapshot())
	}

	// Exit again
	send(listener, "\x04")
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)
	waitForContains(t, listener, "listener>", 5*time.Second)

	// --- Third PTY session (should also work) ---
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)

	send(listener, "\x04")
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)

	// Verify listener is back at prompt
	waitForContains(t, listener, "listener>", 5*time.Second)
}
