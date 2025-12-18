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
// and asserts both sides observe the expected commands and disconnect handling.
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

	// List connected clients and pick the first (and only) one.
	sc := getShellCmds()
	send(listener, sc.list+"\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)
	waitForContains(t, listener, "1.", 5*time.Second)

	send(listener, "use 1\n")
	waitForContains(t, listener, "Now interacting with", 5*time.Second)

	// Run directory list on the client and ensure both sides see activity.
	send(listener, sc.list+"\n")
	waitForContains(t, listener, "go.mod", 5*time.Second)
	waitForContains(t, reverse, fmt.Sprintf("Received command: %s", sc.list), 5*time.Second)

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

	// Normalize paths to use forward slashes for command-line transmission
	localLargeNormalized := filepath.ToSlash(localLarge)
	remoteLargeNormalized := filepath.ToSlash(remoteLarge)
	downloadedLargeNormalized := filepath.ToSlash(downloadedLarge)

	payload := bytes.Repeat([]byte("chunk-0123456789"), 200000) // 2,000,000 bytes
	if err := os.WriteFile(localLarge, payload, 0o644); err != nil {
		t.Fatalf("write local large file: %v", err)
	}

	send(listener, fmt.Sprintf("upload %s %s\n", localLargeNormalized, remoteLargeNormalized))
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
	send(listener, fmt.Sprintf("download %s %s\n", remoteLargeNormalized, downloadedLargeNormalized))
	waitForContains(t, listener, "Downloaded", 15*time.Second)
	// Give the connection time to settle after large file download
	time.Sleep(1 * time.Second)

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
	sc := getShellCmds()
	commands := []struct {
		input  string
		expect string
	}{
		{sc.list + "\n", "go.mod"},
		{sc.who + "\n", currentUser(t)},
		{sc.pwd + "\n", ""}, // Just verify it doesn't crash
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
	send(listener, "use 1\n")
	waitForContains(t, listener, "Now interacting with", 5*time.Second)

	// Run a series of basic commands to stress test buffering and command handling
	// Tests include rapid-fire commands, commands with output, and commands with no output
	// Reduced set for Windows compatibility (full set on other platforms)
	sc := getShellCmds()
	testCases := []struct {
		name     string
		cmd      string
		contains string // expected output substring
	}{
		{"echo simple", "echo hello\n", "hello"},
		{"echo with spaces", "echo hello world\n", "hello world"},
		{"pwd/cd basic", sc.pwd + "\n", ""},
		{"list basic", sc.list + "\n", "go.mod"},
		{"whoami", sc.who + "\n", currentUser(t)},
		{"echo number", "echo 42\n", "42"},
		{"echo test1", "echo test1\n", "test1"},
		{"echo test2", "echo test2\n", "test2"},
		{"echo x", "echo x\n", "x"},
		{"echo y", "echo y\n", "y"},
	}
	
	// Add more commands on non-Windows platforms
	if runtime.GOOS != "windows" {
		testCases = append(testCases, []struct {
			name     string
			cmd      string
			contains string
		}{
			{"echo multiword", "echo one two three four five\n", "one two three four five"},
			{"date/time", sc.date + "\n", ""},
			{"uname/ver", sc.ver + "\n", ""},
			{"echo test3", "echo test3\n", "test3"},
			{"list again", sc.list + "\n", "go.mod"},
			{"whoami again", sc.who + "\n", currentUser(t)},
			{"pwd/cd again", sc.pwd + "\n", ""},
			{"echo z", "echo z\n", "z"},
		}...)
	}

	for i, tc := range testCases {
		send(listener, tc.cmd)

		// For commands with expected output, wait for it
		if tc.contains != "" {
			// Give more time for the last command or file-transfer-heavy commands
			waitTime := 5 * time.Second
			if i == len(testCases)-1 { // Last command
				waitTime = 10 * time.Second
			}
			waitForContains(t, listener, tc.contains, waitTime)
			// For the very last command, add extra time for output to fully appear
			if i == len(testCases)-1 {
				time.Sleep(1 * time.Second)
			}
		} else {
			// For commands without specific output, just give it time to execute
			time.Sleep(300 * time.Millisecond)
		}

		// Verify no errors occurred in output (like "not found" or "exit status 127")
		snapshot := listener.snapshot()
		if strings.Contains(snapshot, "not found") {
			t.Fatalf("test case %d (%s): command not found in output:\n%s", i, tc.name, snapshot)
		}
		if strings.Contains(snapshot, "exit status 127") {
			t.Fatalf("test case %d (%s): command execution failed with exit 127 (not found):\n%s", i, tc.name, snapshot)
		}
	}

	// Verify that all commands executed without buffering issues by checking
	// the reverse client received all commands
	send(listener, "bg\n")
	waitForContains(t, listener, "Backgrounding session", 5*time.Second)

	// Check that reverse client received a good number of commands
	reverseOutput := reverse.snapshot()
	commandCount := strings.Count(reverseOutput, "Received command:")
	if commandCount < len(testCases)-3 { // Allow some margin for timing
		t.Fatalf("reverse client only received %d commands out of %d; output:\n%s", commandCount, len(testCases), reverseOutput)
	}

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
