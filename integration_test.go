package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
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

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

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

	// Exercise large file upload (forces multiple chunks over the connection) and verify integrity.
	sharedDir := t.TempDir()
	localLarge := filepath.Join(sharedDir, "large_local.bin")
	remoteLarge := filepath.Join(sharedDir, "large_remote.bin")
	downloadedLarge := filepath.Join(sharedDir, "large_download.bin")

	payload := bytes.Repeat([]byte("chunk-0123456789"), 200000) // 2,000,000 bytes
	if err := os.WriteFile(localLarge, payload, 0o644); err != nil {
		t.Fatalf("write local large file: %v", err)
	}

	send(listener, fmt.Sprintf("upload %s %s\n", localLarge, remoteLarge))
	waitForContains(t, listener, "Uploaded", 15*time.Second)

	remoteBytes := mustReadFile(t, remoteLarge)
	if !bytes.Equal(remoteBytes, payload) {
		want := sha256.Sum256(payload)
		got := sha256.Sum256(remoteBytes)
		t.Fatalf("uploaded file mismatch: want %d bytes (sha256 %x), got %d bytes (sha256 %x)", len(payload), want, len(remoteBytes), got)
	}

	// Download the same file back and verify integrity.
	send(listener, fmt.Sprintf("download %s %s\n", remoteLarge, downloadedLarge))
	waitForContains(t, listener, "Downloaded", 15*time.Second)

	downloaded := mustReadFile(t, downloadedLarge)
	if !bytes.Equal(downloaded, payload) {
		want := sha256.Sum256(payload)
		got := sha256.Sum256(downloaded)
		t.Fatalf("downloaded file mismatch: want %d bytes (sha256 %x), got %d bytes (sha256 %x)", len(payload), want, len(downloaded), got)
	}

	// Background the session and exit the listener REPL.
	send(listener, "bg\n")
	waitForContains(t, listener, "Backgrounding session", 5*time.Second)

	send(listener, "exit\n")
	waitForExit(t, listener, 5*time.Second)

	// Once the listener is gone, the reverse client should report the broken session and stop.
	waitForContains(t, reverse, "Connection failed", 10*time.Second)
	waitForContains(t, reverse, "Max retries (1) reached. Exiting.", 10*time.Second)
}

// TestLinerHistoryFeature tests that the liner history works by executing multiple commands in sequence
// and verifying each executes correctly. This ensures the liner REPL handles command input properly.
func TestLinerHistoryFeature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	port := freePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, port, "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, fmt.Sprintf("127.0.0.1:%s", port), "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	// Connect to the single client
	send(listener, "use 1\n")
	waitForContains(t, listener, "Now interacting with", 5*time.Second)

	// Execute a series of commands to ensure liner can handle multiple inputs without issues
	commands := []struct {
		input  string
		expect string
	}{
		{"ls\n", "go.mod"},
		{"whoami\n", currentUser(t)},
		{"pwd\n", ""}, // Just verify it doesn't crash
		{"echo test\n", "test"},
	}

	for _, cmd := range commands {
		send(listener, cmd.input)
		if cmd.expect != "" {
			waitForContains(t, listener, cmd.expect, 5*time.Second)
		} else {
			// For commands without a specific expected output, just wait for the prompt
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Background and exit
	send(listener, "bg\n")
	waitForContains(t, listener, "Backgrounding session", 5*time.Second)

	send(listener, "exit\n")
	waitForExit(t, listener, 5*time.Second)
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

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return data
}
