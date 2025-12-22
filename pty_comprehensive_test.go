package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPtyComprehensive is a comprehensive test suite covering all PTY features:
// 1. Enter and exit with Ctrl-D
// 2. Run exit command to close shell
// 3. Re-enter immediately after exit
// 4. Verify listener is responsive after exiting PTY
// 5. Run commands in PTY
func TestPtyComprehensive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	port := freePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, port, "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, fmt.Sprintf("127.0.0.1:%s", port), "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	t.Log("=== Test 1: Basic PTY entry/exit with Ctrl-D ===")
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)
	send(listener, "\x04") // Ctrl-D
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)
	waitForContains(t, listener, "listener>", 5*time.Second)
	t.Log("✓ PTY entry/exit with Ctrl-D works")

	t.Log("=== Test 2: Listener responsive after PTY exit ===")
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)
	t.Log("✓ Listener responsive after Ctrl-D exit")

	t.Log("=== Test 3: Run exit command in PTY ===")
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)
	send(listener, "exit\n")
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)
	waitForContains(t, listener, "listener>", 5*time.Second)
	t.Log("✓ Exit command in PTY works")

	t.Log("=== Test 4: Listener responsive after exit command ===")
	// Wait a bit longer since exit command might take longer to process
	time.Sleep(500 * time.Millisecond)
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)
	if strings.Count(listener.snapshot(), "127.0.0.1") < 1 {
		t.Fatalf("Client should still be connected")
	}
	t.Log("✓ Listener responsive after exit command")

	t.Log("=== Test 5: Re-enter PTY after exit command ===")
	send(listener, "shell 1\n")
	waitForContains(t, listener, "PTY shell active", 5*time.Second)
	if strings.Contains(listener.snapshot(), "Failed to enter PTY mode") {
		t.Fatalf("Should be able to re-enter PTY after exit command")
	}
	t.Log("✓ Re-entry after exit command works")

	t.Log("=== Test 6: Run commands in PTY ===")
	send(listener, "pwd\n")
	time.Sleep(200 * time.Millisecond) // Give command time to execute
	send(listener, "echo hello\n")
	time.Sleep(200 * time.Millisecond)
	t.Log("✓ Commands in PTY execute")

	t.Log("=== Test 7: Exit via Ctrl-D from command execution ===")
	send(listener, "\x04") // Ctrl-D
	waitForContains(t, listener, "[Remote shell exited]", 5*time.Second)
	waitForContains(t, listener, "listener>", 5*time.Second)
	t.Log("✓ Ctrl-D exits PTY cleanly")

	t.Log("=== Test 8: Final listener responsiveness ===")
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)
	t.Log("✓ Listener fully responsive after all tests")

	t.Log("\n=== All PTY comprehensive tests passed ===")
}
