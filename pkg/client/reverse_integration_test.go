package client

import (
	"crypto/tls"
	"testing"
	"time"

	"golang-https-rev/pkg/protocol"
	"golang-https-rev/pkg/server"
	"golang-https-rev/pkg/certs"
)

// TestClientConnect tests successful connection to listener
func TestClientConnect(t *testing.T) {
	listener := createServerForTest(t)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	addr := netListener.Addr().String()

	client := NewReverseClient(addr)
	if client == nil {
		t.Fatal("NewReverseClient returned nil")
	}
	if client.IsConnected() {
		t.Fatal("Client should not be connected after creation")
	}

	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Fatal("Client should be connected after Connect()")
	}

	t.Log("✓ Client connect test passed")
}

// TestClientConnectFailure tests connection to non-existent server
func TestClientConnectFailure(t *testing.T) {
	client := NewReverseClient("127.0.0.1:9999")
	err := client.Connect()
	if err == nil {
		t.Fatal("Expected connection error to non-existent server")
	}

	if client.IsConnected() {
		t.Fatal("Client should not be connected after failed connection attempt")
	}

	t.Log("✓ Client connect failure test passed")
}

// TestClientClose tests closing the connection
func TestClientClose(t *testing.T) {
	listener := createServerForTest(t)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	addr := netListener.Addr().String()

	client := NewReverseClient(addr)
	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if !client.IsConnected() {
		t.Fatal("Client should be connected")
	}

	err = client.Close()
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	if client.IsConnected() {
		t.Fatal("Client should not be connected after Close()")
	}

	t.Log("✓ Client close test passed")
}

// TestClientExecuteCommand tests command execution with output
func TestClientExecuteCommand(t *testing.T) {
	client := NewReverseClient("127.0.0.1:9999")
	
	// Test echo command which works cross-platform
	output := client.ExecuteCommand("echo test_output")
	if output == "" {
		t.Fatal("ExecuteCommand returned empty output")
	}
	
	if !containsStr(output, "test_output") {
		t.Fatalf("Expected 'test_output' in output, got: %s", output)
	}

	t.Log("✓ Client execute command test passed")
}

// TestClientExecuteCommandError tests command execution with error
func TestClientExecuteCommandError(t *testing.T) {
	client := NewReverseClient("127.0.0.1:9999")
	
	// Execute a command that will fail
	output := client.ExecuteCommand("false 2>&1 || true")
	if output == "" {
		t.Logf("Note: empty output from failed command is acceptable")
	}

	t.Log("✓ Client execute command error test passed")
}

// TestClientPingResponse tests that client receives and processes commands
// Note: This tests command reception, not PING specifically (requires HandleCommands to run)
func TestClientCommandReception(t *testing.T) {
	listener := createServerForTest(t)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	addr := netListener.Addr().String()

	client := NewReverseClient(addr)
	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Give listener time to register client
	time.Sleep(100 * time.Millisecond)
	
	clients := listener.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 connected client, got %d", len(clients))
	}

	t.Log("✓ Client command reception test passed")
}

// TestClientUploadFlow tests the upload file flow (connection and setup)
// Note: Full upload cycle requires HandleCommands goroutine; this tests setup
func TestClientUploadFlow(t *testing.T) {
	listener := createServerForTest(t)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	addr := netListener.Addr().String()

	client := NewReverseClient(addr)
	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Verify connection is established
	time.Sleep(100 * time.Millisecond)
	
	clients := listener.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 connected client, got %d", len(clients))
	}

	// Verify we can send commands to the client
	cmd := "test_command"
	err = listener.SendCommand(clients[0], cmd)
	if err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	t.Log("✓ Client upload flow setup test passed")
}

// TestClientExitCommand tests EXIT command handling
func TestClientExitCommand(t *testing.T) {
	listener := createServerForTest(t)
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	addr := netListener.Addr().String()

	client := NewReverseClient(addr)
	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	time.Sleep(100 * time.Millisecond)
	clients := listener.GetClients()
	clientAddr := clients[0]

	// Send EXIT command
	err = listener.SendCommand(clientAddr, protocol.CmdExit)
	if err != nil {
		t.Fatalf("Failed to send EXIT: %v", err)
	}

	// Client should disconnect
	time.Sleep(100 * time.Millisecond)
	clients = listener.GetClients()
	if len(clients) != 0 {
		t.Fatalf("Expected 0 clients after EXIT, got %d", len(clients))
	}

	t.Log("✓ Client exit command test passed")
}

// Helper functions

func createServerForTest(t *testing.T) *server.Listener {
	cert, _, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	return server.NewListener("0", "127.0.0.1", tlsConfig)
}

func containsStr(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
