package main

import (
    "context"
    "fmt"
    "strings"
    "testing"
    "time"
)

// TestClientIdentifierEndToEnd verifies that gotsr announces a short session ID
// and that gotsl 'ls' displays the identifier in brackets next to ip:port.
func TestClientIdentifierEndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    port := freePort(t)
    ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
    defer cancel()

    listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
    reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

    listener := startProcess(ctx, t, listenerBin, "--port", port, "--interface", "127.0.0.1")
    t.Cleanup(listener.stop)
    waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

    reverse := startProcess(ctx, t, reverseBin, "--target", fmt.Sprintf("127.0.0.1:%s", port), "--retries", "1")
    t.Cleanup(reverse.stop)

    // Wait for connection and capture the session ID printed by gotsr
    waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)
    waitForContains(t, reverse, "Session ID:", 5*time.Second)

    // Extract the ID from reverse output snapshot
    rsnap := reverse.snapshot()
    var id string
    for _, line := range strings.Split(rsnap, "\n") {
        if strings.Contains(line, "Session ID:") {
            parts := strings.Split(line, ": ")
            if len(parts) == 2 {
                id = strings.TrimSpace(parts[1])
                break
            }
        }
    }
    if id == "" {
        t.Fatalf("failed to extract session ID from reverse output; snapshot:\n%s", rsnap)
    }

    // Ask listener to list clients and verify the identifier appears in brackets
    send(listener, "ls\n")
    waitForContains(t, listener, "Connected Clients:", 5*time.Second)
    waitForContains(t, listener, "["+id+"]", 5*time.Second)

    send(listener, "exit\n")
    waitForExit(t, listener, 5*time.Second)
}
