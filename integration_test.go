package main

import (
    "bufio"
    "bytes"
    "context"
    "fmt"
    "io"
    "net"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "testing"
    "time"
)

// TestListenerReverseInteractiveSession drives the listener and reverse binaries end-to-end
// and asserts both sides observe the expected commands and disconnect handling.
func TestListenerReverseInteractiveSession(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    port := freePort(t)
    ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
    defer cancel()

    listenerBin := buildBinary(t, "listener", "./cmd/listener")
    reverseBin := buildBinary(t, "reverse", "./cmd/reverse")

    listener := startProcess(ctx, t, listenerBin, port, "127.0.0.1")
    t.Cleanup(listener.stop)
    waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

    reverse := startProcess(ctx, t, reverseBin, fmt.Sprintf("127.0.0.1:%s", port), "1")
    t.Cleanup(reverse.stop)
    waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

    // List connected clients and pick the first (and only) one.
    send(listener, "ls\n")
    waitForContains(t, listener, "Connected Clients:", 5*time.Second)
    waitForContains(t, listener, "1.", 5*time.Second)

    send(listener, "use 1\n")
    waitForContains(t, listener, "Now interacting with", 5*time.Second)

    // Run ls on the client and ensure both sides see activity.
    send(listener, "ls\n")
    waitForContains(t, listener, "go.mod", 5*time.Second)
    waitForContains(t, reverse, "Received command: ls", 5*time.Second)

    // Run whoami on the client and assert output on both sides.
    user := currentUser(t)
    send(listener, "whoami\n")
    waitForContains(t, listener, user, 5*time.Second)
    waitForContains(t, reverse, "Received command: whoami", 5*time.Second)

    // Background the session and exit the listener REPL.
    send(listener, "background\n")
    waitForContains(t, listener, "Backgrounding session", 5*time.Second)

    send(listener, "exit\n")
    waitForExit(t, listener, 5*time.Second)

    // Once the listener is gone, the reverse client should report the broken session and stop.
    waitForContains(t, reverse, "Connection failed", 10*time.Second)
    waitForContains(t, reverse, "Max retries (1) reached. Exiting.", 10*time.Second)
}

type proc struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    output bytes.Buffer
    lines  chan string
    mu     sync.Mutex
}

func startProcess(ctx context.Context, t *testing.T, name string, args ...string) *proc {
    t.Helper()
    cmd := exec.CommandContext(ctx, name, args...)
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        t.Fatalf("stdout pipe: %v", err)
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        t.Fatalf("stderr pipe: %v", err)
    }
    stdin, err := cmd.StdinPipe()
    if err != nil {
        t.Fatalf("stdin pipe: %v", err)
    }

    p := &proc{cmd: cmd, stdin: stdin, lines: make(chan string, 100)}

    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        captureLines(p, stdout)
    }()

    go func() {
        defer wg.Done()
        captureLines(p, stderr)
    }()

    go func() {
        wg.Wait()
        close(p.lines)
    }()

    if err := cmd.Start(); err != nil {
        t.Fatalf("start %s: %v", name, err)
    }

    return p
}

func captureLines(p *proc, r io.Reader) {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := scanner.Text()
        p.mu.Lock()
        p.output.WriteString(line)
        p.output.WriteByte('\n')
        p.mu.Unlock()
        p.lines <- line
    }
}

func (p *proc) stop() {
    _ = p.stdin.Close()
    if p.cmd.ProcessState == nil || !p.cmd.ProcessState.Exited() {
        _ = p.cmd.Process.Kill()
        _ = p.cmd.Wait()
    }
}

func send(p *proc, data string) {
    _, _ = io.WriteString(p.stdin, data)
}

func waitForLine(t *testing.T, p *proc, substr string, timeout time.Duration) {
    t.Helper()
    deadline := time.After(timeout)
    for {
        select {
        case line, ok := <-p.lines:
            if !ok {
                t.Fatalf("channel closed before finding %q; output so far:\n%s", substr, p.snapshot())
            }
            if strings.Contains(line, substr) {
                return
            }
        case <-deadline:
            t.Fatalf("timeout waiting for %q; output so far:\n%s", substr, p.snapshot())
        }
    }
}

func waitForContains(t *testing.T, p *proc, substr string, timeout time.Duration) {
    t.Helper()
    deadline := time.After(timeout)
    ticker := time.NewTicker(50 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if strings.Contains(p.snapshot(), substr) {
                return
            }
        case <-deadline:
            t.Fatalf("timeout waiting for output containing %q; output so far:\n%s", substr, p.snapshot())
        }
    }
}

func waitForExit(t *testing.T, p *proc, timeout time.Duration) {
    t.Helper()
    done := make(chan error, 1)
    go func() {
        done <- p.cmd.Wait()
    }()

    select {
    case err := <-done:
        if err != nil {
            t.Fatalf("process exited with error: %v; output:\n%s", err, p.snapshot())
        }
    case <-time.After(timeout):
        t.Fatalf("timeout waiting for process exit; output so far:\n%s", p.snapshot())
    }
}

func (p *proc) snapshot() string {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.output.String()
}

func freePort(t *testing.T) string {
    t.Helper()
    l, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatalf("listen: %v", err)
    }
    defer l.Close()
    return fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
}

func buildBinary(t *testing.T, name, pkg string) string {
    t.Helper()
    out := filepath.Join(t.TempDir(), name)
    cmd := exec.Command("go", "build", "-o", out, pkg)
    var buf bytes.Buffer
    cmd.Stdout = &buf
    cmd.Stderr = &buf
    if err := cmd.Run(); err != nil {
        t.Fatalf("build %s failed: %v; output: %s", name, err, buf.String())
    }
    return out
}

func currentUser(t *testing.T) string {
    t.Helper()
    out, err := exec.Command("whoami").Output()
    if err != nil {
        t.Fatalf("whoami failed: %v", err)
    }
    return strings.TrimSpace(string(out))
}
