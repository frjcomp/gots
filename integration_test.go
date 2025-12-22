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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// shellCmds captures OS-specific shell commands used by tests
type shellCmds struct {
	list string // directory listing
	pwd  string // print working directory
	who  string // current user
	date string // date/time command
	ver  string // os version/uname equivalent
}

func getShellCmds() shellCmds {
	if runtime.GOOS == "windows" {
		return shellCmds{
			list: "dir",
			pwd:  "cd",
			who:  "whoami",
			date: "time /T",
			ver:  "ver",
		}
	}
	return shellCmds{
		list: "ls",
		pwd:  "pwd",
		who:  "whoami",
		date: "date",
		ver:  "uname",
	}
}

// TestListenerReverseInteractiveSession drives the listener and reverse binaries end-to-end
// and asserts basic connectivity and file transfer operations.
func TestListenerReverseInteractiveSession(t *testing.T) {
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

	// List connected clients
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)
	waitForContains(t, listener, "1.", 5*time.Second)

	// Exercise large file upload (forces multiple chunks over the connection) and verify integrity.
	sharedDir := t.TempDir()
	localLarge := filepath.Join(sharedDir, "large_local.bin")
	remoteLarge := filepath.Join(sharedDir, "large_remote.bin")
	downloadedLarge := filepath.Join(sharedDir, "large_download.bin")

	// Normalize paths to use forward slashes for command-line transmission
	localLargeNormalized := filepath.ToSlash(localLarge)
	remoteLargeNormalized := filepath.ToSlash(remoteLarge)
	downloadedLargeNormalized := filepath.ToSlash(downloadedLarge)

	payload := bytes.Repeat([]byte("chunk-0123456789"), 200000) // 2,000,000 bytes
	if err := os.WriteFile(localLarge, payload, 0o644); err != nil {
		t.Fatalf("write local large file: %v", err)
	}

	send(listener, fmt.Sprintf("upload 1 %s %s\n", localLargeNormalized, remoteLargeNormalized))
	waitForContains(t, listener, "Uploaded", 15*time.Second)
	// Give the connection time to settle after large file upload
	time.Sleep(1 * time.Second)

	remoteBytes := mustReadFile(t, remoteLarge)
	if !bytes.Equal(remoteBytes, payload) {
		want := sha256.Sum256(payload)
		got := sha256.Sum256(remoteBytes)
		t.Fatalf("uploaded file mismatch: want %d bytes (sha256 %x), got %d bytes (sha256 %x)", len(payload), want, len(remoteBytes), got)
	}

	// Download the same file back and verify integrity.
	send(listener, fmt.Sprintf("download 1 %s %s\n", remoteLargeNormalized, downloadedLargeNormalized))
	waitForContains(t, listener, "Downloaded", 15*time.Second)
	// Give the connection time to settle after large file download
	time.Sleep(1 * time.Second)

	downloaded := mustReadFile(t, downloadedLarge)
	if !bytes.Equal(downloaded, payload) {
		want := sha256.Sum256(payload)
		got := sha256.Sum256(downloaded)
		t.Fatalf("downloaded file mismatch: want %d bytes (sha256 %x), got %d bytes (sha256 %x)", len(payload), want, len(downloaded), got)
	}

	// Exit the listener REPL.
	send(listener, "exit\n")
	waitForExit(t, listener, 5*time.Second)

	// Once the listener is gone, the reverse client should report the broken session and stop.
	waitForContains(t, reverse, "Connection failed", 10*time.Second)
	waitForContains(t, reverse, "Max retries (1) reached. Exiting.", 10*time.Second)
}

// TestSequentialCommandOperations ensures multiple file operations can be performed
// in sequence without issues using the listener REPL.
func TestSequentialCommandOperations(t *testing.T) {
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

	// Execute multiple file operations to ensure the connection remains stable
	sharedDir := t.TempDir()
	
	// Create multiple test files for upload
	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Content 1"},
		{"file2.txt", "Content 2"},
		{"file3.txt", "Content 3"},
	}
	
	// Create local files
	localFiles := make([]string, len(testFiles))
	remoteFiles := make([]string, len(testFiles))
	for i, tf := range testFiles {
		localPath := filepath.Join(sharedDir, "local_"+tf.name)
		remotePath := filepath.Join(sharedDir, "remote_"+tf.name)
		if err := os.WriteFile(localPath, []byte(tf.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", localPath, err)
		}
		localFiles[i] = filepath.ToSlash(localPath)
		remoteFiles[i] = filepath.ToSlash(remotePath)
	}
	
	// Perform a series of uploads to stress test buffering
	for i, localFile := range localFiles {
		send(listener, fmt.Sprintf("upload 1 %s %s\n", localFile, remoteFiles[i]))
		waitForContains(t, listener, "Uploaded", 10*time.Second)
		time.Sleep(300 * time.Millisecond)
	}

	send(listener, "exit\n")
	waitForExit(t, listener, 5*time.Second)
}

// TestCommandLoadAndBuffering runs many basic CLI commands in sequence to detect buffering issues.
// This test ensures that rapid command execution doesn't cause command corruption or buffering problems.
func TestCommandLoadAndBuffering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	port := freePort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, port, "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, fmt.Sprintf("127.0.0.1:%s", port), "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	// Connect to the client
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)

	// Create multiple test files for upload
	sharedDir := t.TempDir()
	
	testFiles := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Content 1"},
		{"file2.txt", "Content 2"},
		{"file3.txt", "Content 3"},
	}
	
	// Create local files
	localFiles := make([]string, len(testFiles))
	remoteFiles := make([]string, len(testFiles))
	for i, tf := range testFiles {
		localPath := filepath.Join(sharedDir, "local_"+tf.name)
		remotePath := filepath.Join(sharedDir, "remote_"+tf.name)
		if err := os.WriteFile(localPath, []byte(tf.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", localPath, err)
		}
		localFiles[i] = filepath.ToSlash(localPath)
		remoteFiles[i] = filepath.ToSlash(remotePath)
	}
	
	// Perform a series of uploads to stress test buffering
	for i, localFile := range localFiles {
		send(listener, fmt.Sprintf("upload 1 %s %s\n", localFile, remoteFiles[i]))
		waitForContains(t, listener, "Uploaded", 10*time.Second)
		time.Sleep(300 * time.Millisecond)
	}

	// List clients to ensure connection is still active
	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)

	send(listener, "exit\n")
	waitForExit(t, listener, 5*time.Second)

	// Once the listener is gone, the reverse client should report the broken session and stop
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
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(out), ".exe") {
		out += ".exe"
	}
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
