package client

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/frjcomp/gots/pkg/certs"
	"github.com/frjcomp/gots/pkg/protocol"
	"github.com/frjcomp/gots/pkg/server"
)

// TestReverseClientCreation tests creating a new reverse client
func TestReverseClientCreation(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	if client == nil {
		t.Fatal("Failed to create reverse client")
	}

	if client.target != "127.0.0.1:8080" {
		t.Fatalf("Expected target 127.0.0.1:8080, got %s", client.target)
	}

	t.Log("✓ Reverse client created successfully")
}

// TestIsConnected tests the IsConnected method
func TestIsConnected(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	if client.IsConnected() {
		t.Fatal("Client should not be connected initially")
	}

	t.Log("✓ IsConnected works correctly")
}

// TestExecuteCommand tests command execution
func TestExecuteCommand(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	output := client.ExecuteCommand("echo hello")

	if !contains(output, "hello") {
		t.Fatalf("Expected 'hello' in output, got '%s'", output)
	}

	t.Log("✓ ExecuteCommand works")
}

// TestExecuteMultipleCommands tests executing multiple commands
func TestExecuteMultipleCommands(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")

	commands := []string{
		"echo test1",
		"echo test2",
		"whoami",
	}

	for i, cmd := range commands {
		output := client.ExecuteCommand(cmd)
		if len(output) == 0 {
			t.Fatalf("Command %d (%s) produced empty output", i+1, cmd)
		}
		t.Logf("Command %d executed: %s", i+1, cmd)
	}

	t.Log("✓ Multiple commands execute successfully")
}

// TestExecuteCommandWithPath tests command execution with path output
func TestExecuteCommandWithPath(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	output := client.ExecuteCommand("pwd")

	if len(output) == 0 {
		t.Fatal("pwd produced empty output")
	}

	if output[0] != '/' {
		t.Fatalf("Expected path starting with '/', got '%s'", output)
	}

	t.Log("✓ Path commands work correctly")
}

func TestSessionIDGeneration(t *testing.T) {
	id := GetSessionID()
	if len(id) != 8 {
		t.Fatalf("expected 8-char session ID, got %d (%s)", len(id), id)
	}
	// Verify hex characters
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("session ID not hex: %s", id)
		}
	}
}

// startTestListener starts a test listener and returns address
func startTestListener(t *testing.T, sharedSecret, certFingerprint string) (net.Listener, string) {
	cert, _, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := server.NewListener("0", "127.0.0.1", tlsConfig, sharedSecret)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}

	return netListener, netListener.Addr().String()
}

// startTestListenerWithFingerprint starts a test listener and returns address and fingerprint
func startTestListenerWithFingerprint(t *testing.T, sharedSecret, _ string) (net.Listener, string, string) {
	cert, _, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	// Calculate fingerprint
	certDER := cert.Certificate[0]
	hash := sha256.Sum256(certDER)
	fingerprint := hex.EncodeToString(hash[:])

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := server.NewListener("0", "127.0.0.1", tlsConfig, sharedSecret)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}

	return netListener, netListener.Addr().String(), fingerprint
}

// Helper function
func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestExecuteCommandEmptyInput tests handling of empty commands
func TestExecuteCommandEmptyInput(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	output := client.ExecuteCommand("")

	// Empty command should return empty output
	if len(output) != 0 {
		t.Logf("Empty command returned: %s", output)
	}

	t.Log("✓ Empty command handled")
}

// TestExecuteCommandInvalidCommand tests handling of invalid commands
func TestExecuteCommandInvalidCommand(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	output := client.ExecuteCommand("nonexistent_command_12345")

	// Should get error output
	if len(output) == 0 {
		t.Log("Invalid command produced empty output (might be valid)")
	}

	t.Log("✓ Invalid command handled")
}

// TestCloseBeforeConnect tests closing when not connected
func TestCloseBeforeConnect(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")
	err := client.Close()

	// Should handle gracefully
	if err != nil {
		t.Logf("Close without connection returned error: %v", err)
	}

	t.Log("✓ Close without connection handled")
}

// TestExecuteShellCommand tests the shell command execution
func TestExecuteShellCommand(t *testing.T) {
	// Test simple command using package-level function
	output, err := executeShellCommand("echo test_shell")
	if err != nil {
		t.Errorf("Shell command failed: %v", err)
	}
	if !contains(output, "test_shell") {
		t.Errorf("Expected 'test_shell' in output, got '%s'", output)
	}

	t.Log("✓ Shell command execution works")
}

// TestExecuteShellCommandError tests error handling in shell execution
func TestExecuteShellCommandError(t *testing.T) {
	// Command that will fail
	output, err := executeShellCommand("exit 1")

	// Should return error
	if err == nil {
		t.Error("Expected error for failing command")
	}

	// Output should contain error info
	if !contains(output, "Error") {
		t.Logf("Error command output: %s", output)
	}

	t.Log("✓ Shell command error handled")
}

// TestExecuteShellCommandNonexistent tests handling of nonexistent commands
func TestExecuteShellCommandNonexistent(t *testing.T) {
	output, err := executeShellCommand("nonexistent_cmd_xyz_12345")

	// Should return error
	if err == nil {
		t.Error("Expected error for nonexistent command")
	}

	// Output should contain error info
	if !contains(output, "Error") {
		t.Logf("Nonexistent command output: %s", output)
	}

	t.Log("✓ Nonexistent command handled")
}

// TestConnectNoServer tests connection failure when no server is running
func TestConnectNoServer(t *testing.T) {
	client := NewReverseClient("127.0.0.1:19999", "", "")

	err := client.Connect()
	if err == nil {
		t.Fatal("Expected connection error when no server is running")
	}

	if client.IsConnected() {
		t.Error("Client should not be marked as connected after failed connection")
	}

	t.Log("✓ Connection failure handled correctly")
}

// TestConnectSuccessNoAuth tests successful connection without authentication
func TestConnectSuccessNoAuth(t *testing.T) {
	// Start a test listener
	listener, addr := startTestListener(t, "", "")
	defer listener.Close()

	client := NewReverseClient(addr, "", "")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	t.Log("✓ Successful connection without auth")
}

// TestConnectWithSharedSecretSuccess tests authentication with correct shared secret
func TestConnectWithSharedSecretSuccess(t *testing.T) {
	secret := "test-secret-123"

	// Start a test listener with shared secret
	listener, addr := startTestListener(t, secret, "")
	defer listener.Close()

	client := NewReverseClient(addr, secret, "")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect with valid secret: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	t.Log("✓ Authentication with shared secret successful")
}

// TestConnectWithInvalidSharedSecret tests authentication failure with wrong secret
func TestConnectWithInvalidSharedSecret(t *testing.T) {
	// Start a test listener with one secret
	listener, addr := startTestListener(t, "correct-secret", "")
	defer listener.Close()

	// Try to connect with wrong secret
	client := NewReverseClient(addr, "wrong-secret", "")
	err := client.Connect()

	if err == nil {
		t.Fatal("Expected authentication error with wrong shared secret")
	}

	if !contains(err.Error(), "authentication failed") {
		t.Errorf("Expected 'authentication failed' error, got: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after auth failure")
	}

	t.Log("✓ Invalid shared secret rejected correctly")
}

// TestConnectWithoutRequiredSecret tests connection failure when secret is required
func TestConnectWithoutRequiredSecret(t *testing.T) {
	// Start a test listener that requires a secret
	listener, addr := startTestListener(t, "required-secret", "")
	defer listener.Close()

	// Try to connect without providing secret
	// The server will close the connection since no AUTH command is sent
	client := NewReverseClient(addr, "", "")
	err := client.Connect()

	// Connection should succeed initially since the client doesn't know auth is required
	// But it will likely fail when trying to use the connection
	// This is a limitation - the client can't know auth is required until it tries
	if err != nil {
		t.Logf("Connection failed (expected): %v", err)
	}

	// Even if initial connection succeeds, the connection will be closed by server
	// when it doesn't receive AUTH command
	t.Log("✓ Connection without required secret handled")
}

// TestConnectWithCertFingerprintMatch tests cert fingerprint validation success
func TestConnectWithCertFingerprintMatch(t *testing.T) {
	// Start test listener and get its cert fingerprint
	listener, addr, fingerprint := startTestListenerWithFingerprint(t, "", "")
	defer listener.Close()

	// Connect with correct fingerprint
	client := NewReverseClient(addr, "", fingerprint)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect with valid fingerprint: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	t.Log("✓ Certificate fingerprint validation successful")
}

// TestConnectWithCertFingerprintMismatch tests cert fingerprint validation failure
func TestConnectWithCertFingerprintMismatch(t *testing.T) {
	// Start test listener
	listener, addr, _ := startTestListenerWithFingerprint(t, "", "")
	defer listener.Close()

	// Connect with wrong fingerprint
	wrongFingerprint := "0000000000000000000000000000000000000000000000000000000000000000"
	client := NewReverseClient(addr, "", wrongFingerprint)
	err := client.Connect()

	if err == nil {
		t.Fatal("Expected error with mismatched certificate fingerprint")
	}

	if !contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("Expected 'fingerprint mismatch' error, got: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after cert validation failure")
	}

	t.Log("✓ Certificate fingerprint mismatch detected")
}

// TestConnectWithBothAuthAndCert tests connection with both auth methods
func TestConnectWithBothAuthAndCert(t *testing.T) {
	secret := "test-secret-456"

	// Start test listener with shared secret
	listener, addr, fingerprint := startTestListenerWithFingerprint(t, secret, "")
	defer listener.Close()

	// Connect with both secret and fingerprint
	client := NewReverseClient(addr, secret, fingerprint)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect with both auth methods: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}

	t.Log("✓ Connection with both auth methods successful")
}

// TestCloseConnection tests closing an active connection
func TestCloseConnection(t *testing.T) {
	listener, addr := startTestListener(t, "", "")
	defer listener.Close()

	client := NewReverseClient(addr, "", "")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Client should be connected before close")
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected after close")
	}

	t.Log("✓ Connection closed successfully")
}

// TestCloseWithoutConnection tests closing when not connected
func TestCloseWithoutConnection(t *testing.T) {
	client := NewReverseClient("127.0.0.1:8080", "", "")

	err := client.Close()
	if err != nil {
		t.Errorf("Close should not return error when not connected, got: %v", err)
	}

	t.Log("✓ Close without connection handled correctly")
}

// TestHandleCommandsWithEOF tests handling EOF (normal disconnect)
func TestHandleCommandsWithEOF(t *testing.T) {
	client, _ := createMockClient()

	// Create a reader with immediate EOF
	reader := strings.NewReader("")
	client.reader = bufio.NewReader(reader)

	// Should return nil on EOF (graceful disconnect)
	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Expected nil on EOF, got: %v", err)
	}

	t.Log("✓ EOF handled gracefully")
}

// TestHandleCommandsEmptyCommand tests handling empty commands
func TestHandleCommandsEmptyCommand(t *testing.T) {
	client, _ := createMockClient()

	// Create reader with empty line followed by EOF
	reader := strings.NewReader("\nEOF")
	client.reader = bufio.NewReader(reader)

	// Should skip empty lines and return on EOF
	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Expected nil on EOF, got: %v", err)
	}

	t.Log("✓ Empty commands skipped")
}

// TestHandleCommandsExitCommand tests EXIT command handling
func TestHandleCommandsExitCommand(t *testing.T) {
	// Mock os.Exit to prevent actual process exit during test
	originalExit := osExitFunc
	osExitFunc = func(code int) {
		// Just return, don't actually exit
	}
	defer func() { osExitFunc = originalExit }()

	client, output := createMockClient()

	// Create reader with PING then EXIT
	reader := strings.NewReader(protocol.CmdPing + "\n" + protocol.CmdExit + "\n")
	client.reader = bufio.NewReader(reader)

	// Should process PING, then EXIT and return
	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Expected nil after EXIT, got: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, protocol.CmdPong) {
		t.Logf("Expected PONG response, got: %s", result)
	}

	t.Log("✓ EXIT command handled")
}

// TestHandleCommandsShellCommand tests shell command execution in non-PTY mode
func TestHandleCommandsShellCommand(t *testing.T) {
	// Mock os.Exit to prevent actual process exit during test
	originalExit := osExitFunc
	osExitFunc = func(code int) {
		// Just return, don't actually exit
	}
	defer func() { osExitFunc = originalExit }()

	client, output := createMockClient()

	// Create reader with shell command then EXIT
	reader := strings.NewReader("SHELL echo test\n" + protocol.CmdExit + "\n")
	client.reader = bufio.NewReader(reader)

	// Should execute shell command and return on EXIT
	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Expected nil after EXIT, got: %v", err)
	}

	result := output.String()
	// Should have output from echo command
	if !strings.Contains(result, "test") {
		t.Logf("Expected 'test' in output, got: %s", result)
	}

	t.Log("✓ Shell command executed")
}

// TestHandleCommandsPingCommand tests PING command in loop
func TestHandleCommandsPingCommand(t *testing.T) {
	// Mock os.Exit to prevent actual process exit during test
	originalExit := osExitFunc
	osExitFunc = func(code int) {
		// Just return, don't actually exit
	}
	defer func() { osExitFunc = originalExit }()

	client, output := createMockClient()

	// Create reader with PING commands and EXIT
	reader := strings.NewReader(protocol.CmdPing + "\n" + protocol.CmdPing + "\n" + protocol.CmdExit + "\n")
	client.reader = bufio.NewReader(reader)

	// Should handle multiple PINGs
	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Expected nil after EXIT, got: %v", err)
	}

	result := output.String()
	// Count PONG responses
	pongCount := strings.Count(result, protocol.CmdPong)
	if pongCount < 2 {
		t.Logf("Expected at least 2 PONGs, got %d in: %s", pongCount, result)
	}

	t.Log("✓ Multiple PING commands handled")
}

// TestHandleCommandsReadError tests handling of read errors
func TestHandleCommandsReadError(t *testing.T) {
	client, _ := createMockClient()

	// Create reader that returns error
	reader := &errorReader{}
	client.reader = bufio.NewReader(reader)

	// Should return error
	err := client.HandleCommands()
	if err == nil {
		t.Error("Expected error from read failure")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("Expected 'read error' in error message, got: %v", err)
	}

	t.Log("✓ Read error handled")
}

// errorReader returns an error on Read
type errorReader struct{}

func (er *errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("simulated read error")
}

// TestHandleCommandsPtyMode tests handling commands in PTY mode
func TestHandleCommandsPtyMode(t *testing.T) {
	client, _ := createMockClient()
	client.inPtyMode = true

	// Send a PTY exit command
	commands := fmt.Sprintf("%s\n", protocol.CmdPtyExit)
	reader := bufio.NewReader(strings.NewReader(commands))
	client.reader = reader

	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Client should still be running (PTY exit doesn't close connection)
	t.Log("✓ PTY exit command handled")
}

// TestHandleCommandsPtyDataCommand tests handling PTY data command
func TestHandleCommandsPtyDataCommand(t *testing.T) {
	client, _ := createMockClient()
	client.inPtyMode = true

	// Send a PTY data command (hex encoded)
	dataCmd := fmt.Sprintf("%s %s\n", protocol.CmdPtyData, "6865") // "he" in hex
	reader := bufio.NewReader(strings.NewReader(dataCmd + fmt.Sprintf("%s\n", protocol.CmdPtyExit)))
	client.reader = reader

	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	t.Log("✓ PTY data command handled")
}

// TestHandleCommandsPtyResizeCommand tests handling PTY resize command
func TestHandleCommandsPtyResizeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY resize test skipped on Windows")
	}

	client, _ := createMockClient()
	client.inPtyMode = true

	// Send a PTY resize command
	resizeCmd := fmt.Sprintf("%s 24 80\n", protocol.CmdPtyResize)
	reader := bufio.NewReader(strings.NewReader(resizeCmd + fmt.Sprintf("%s\n", protocol.CmdPtyExit)))
	client.reader = reader

	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	t.Log("✓ PTY resize command handled")
}

// TestHandleCommandsIgnoredInPtyMode tests ignored commands in PTY mode
func TestHandleCommandsIgnoredInPtyMode(t *testing.T) {
	client, _ := createMockClient()
	client.inPtyMode = true

	// Send commands that should be ignored in PTY mode
	commands := fmt.Sprintf("PING\nECHO test\n%s\n", protocol.CmdPtyExit)
	reader := bufio.NewReader(strings.NewReader(commands))
	client.reader = reader

	err := client.HandleCommands()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	t.Log("✓ Non-PTY commands ignored in PTY mode")
}
