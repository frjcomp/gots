package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPtyExitCommand tests the scenario: enter PTY shell -> run exit command -> listener should stay responsive
// Previously, running 'exit' instead of Ctrl-D would crash the listener
func TestPtyExitCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
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

	// Enter shell
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)

	// Send exit command (should close the shell)
	send(listener, "exit\n")
	
	// The listener should detect the remote shell exited
	// Windows ConPTY may take longer to detect shell exit
	waitForContains(t, listener, "[Remote shell exited]", 10*time.Second)
	
	// The listener should return to the prompt and be responsive
	waitForContains(t, listener, "listener>", 5*time.Second)

	// Verify listener is still responsive by listing clients
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)

	// The client should still be connected
	if !strings.Contains(listener.snapshot(), "127.0.0.1") {
		t.Fatalf("Client should still be connected after exit command")
	}

	t.Logf("Test passed: listener remained responsive after exit command")
}
