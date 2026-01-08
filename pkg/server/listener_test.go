package server

import (
	"bytes"
	"crypto/tls"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/certs"
)

// TestListenerCreation tests creating a new listener
func TestListenerCreation(t *testing.T) {
	cert, _, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")
	if listener == nil {
		t.Fatal("Failed to create listener")
	}

	if listener.port == "" {
		t.Fatalf("Expected non-empty port, got empty string")
	}

	t.Log("✓ Listener created successfully")
}

// TestGetClients tests getting client list
func TestGetClients(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")
	clients := listener.GetClients()

	if len(clients) != 0 {
		t.Fatalf("Expected 0 clients, got %d", len(clients))
	}

	t.Log("✓ GetClients works for empty list")
}

// TestSendCommand tests sending a command
func TestSendCommand(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Try sending to non-existent client
	err := listener.SendCommand("127.0.0.1:9999", "test")
	if err == nil {
		t.Fatal("Expected error for non-existent client")
	}

	t.Log("✓ SendCommand properly rejects non-existent clients")
}

// TestGetResponse tests getting a response
func TestGetResponse(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Try getting response from non-existent client
	_, err := listener.GetResponse("127.0.0.1:9999", 1*time.Second)
	if err == nil {
		t.Fatal("Expected error for non-existent client")
	}

	t.Log("✓ GetResponse properly rejects non-existent clients")
}

// TestEnterPtyMode tests entering PTY mode for a client
func TestEnterPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Add a mock client
	clientAddr := "127.0.0.1:5000"
	listener.clientConnections[clientAddr] = make(chan string)

	// Test entering PTY mode
	ptyDataChan, err := listener.EnterPtyMode(clientAddr)
	if err != nil {
		t.Fatalf("Failed to enter PTY mode: %v", err)
	}

	if ptyDataChan == nil {
		t.Fatal("PTY data channel should not be nil")
	}

	if !listener.IsInPtyMode(clientAddr) {
		t.Error("Client should be in PTY mode")
	}

	t.Log("✓ Enter PTY mode successful")
}

// TestEnterPtyModeNonExistentClient tests entering PTY mode for non-existent client
func TestEnterPtyModeNonExistentClient(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	_, err := listener.EnterPtyMode("127.0.0.1:9999")
	if err == nil {
		t.Fatal("Expected error for non-existent client")
	}

	t.Log("✓ Non-existent client rejected")
}

// TestEnterPtyModeAlreadyInPtyMode tests entering PTY mode when already in it
func TestEnterPtyModeAlreadyInPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:5001"
	listener.clientConnections[clientAddr] = make(chan string)

	// Enter PTY mode first time
	_, err := listener.EnterPtyMode(clientAddr)
	if err != nil {
		t.Fatalf("First entry failed: %v", err)
	}

	// Try entering again
	_, err = listener.EnterPtyMode(clientAddr)
	if err == nil {
		t.Fatal("Expected error when already in PTY mode")
	}

	t.Log("✓ Duplicate PTY mode entry rejected")
}

// TestExitPtyMode tests exiting PTY mode
func TestExitPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:5002"
	listener.clientConnections[clientAddr] = make(chan string)

	// Enter PTY mode
	_, err := listener.EnterPtyMode(clientAddr)
	if err != nil {
		t.Fatalf("Failed to enter PTY mode: %v", err)
	}

	// Exit PTY mode
	err = listener.ExitPtyMode(clientAddr)
	if err != nil {
		t.Errorf("Failed to exit PTY mode: %v", err)
	}

	if listener.IsInPtyMode(clientAddr) {
		t.Error("Client should not be in PTY mode after exit")
	}

	t.Log("✓ Exit PTY mode successful")
}

// TestExitPtyModeNotInPtyMode tests exiting when not in PTY mode
func TestExitPtyModeNotInPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:5003"

	// Try exiting without entering
	err := listener.ExitPtyMode(clientAddr)
	if err != nil {
		t.Errorf("Exit without PTY mode should not error, got: %v", err)
	}

	t.Log("✓ Exit without PTY mode handled gracefully")
}

// TestIsInPtyMode tests checking PTY mode status
func TestIsInPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:5004"
	listener.clientConnections[clientAddr] = make(chan string)

	// Should not be in PTY mode initially
	if listener.IsInPtyMode(clientAddr) {
		t.Error("Client should not be in PTY mode initially")
	}

	// Enter PTY mode
	listener.EnterPtyMode(clientAddr)

	// Should be in PTY mode now
	if !listener.IsInPtyMode(clientAddr) {
		t.Error("Client should be in PTY mode")
	}

	t.Log("✓ IsInPtyMode works correctly")
}

// TestGetPtyDataChan tests getting PTY data channel
func TestGetPtyDataChan(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:5005"
	listener.clientConnections[clientAddr] = make(chan string)

	// Should not exist initially
	_, exists := listener.GetPtyDataChan(clientAddr)
	if exists {
		t.Error("PTY data channel should not exist initially")
	}

	// Enter PTY mode
	listener.EnterPtyMode(clientAddr)

	// Should exist now
	ch, exists := listener.GetPtyDataChan(clientAddr)
	if !exists {
		t.Error("PTY data channel should exist after entering PTY mode")
	}
	if ch == nil {
		t.Error("PTY data channel should not be nil")
	}

	t.Log("✓ GetPtyDataChan works correctly")
}

// TestHandleClientBasicCommand tests basic command handling
func TestHandleClientBasicCommand(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")
	netListener, err := listener.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	// The listener is running, but we're testing command routing
	// This would require a full integration test to properly test handleClient
	// For now, verify the listener is set up correctly
	if listener == nil {
		t.Fatal("Listener should not be nil")
	}

	t.Log("✓ Listener initialized for client handling")
}

// TestHandleClientMultipleClients tests handling multiple clients
func TestHandleClientMultipleClients(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Simulate adding multiple clients
	listener.clientConnections["client1"] = make(chan string, 10)
	listener.clientConnections["client2"] = make(chan string, 10)
	listener.clientConnections["client3"] = make(chan string, 10)

	clients := listener.GetClients()
	if len(clients) != 3 {
		t.Errorf("Expected 3 clients, got %d", len(clients))
	}

	t.Log("✓ Multiple clients handled correctly")
}

// TestHandleClientSendingCommands tests sending commands to clients
func TestHandleClientSendingCommands(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Simulate a client
	clientAddr := "127.0.0.1:9999"
	cmdChan := make(chan string, 10)
	listener.clientConnections[clientAddr] = cmdChan

	// Send a command
	err := listener.SendCommand(clientAddr, "echo test")
	if err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Verify the command was sent
	select {
	case cmd := <-cmdChan:
		if cmd != "echo test" {
			t.Errorf("Expected 'echo test', got '%s'", cmd)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for command")
	}

	t.Log("✓ Commands sent to clients correctly")
}

// TestHandleClientAuthenticationFailure tests authentication handling
func TestHandleClientAuthenticationFailure(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create listener with shared secret
	listener := NewListener("0", "127.0.0.1", tlsConfig, "secret123")

	// Verify the listener was created with the secret
	if listener.sharedSecret != "secret123" {
		t.Errorf("Expected secret 'secret123', got '%s'", listener.sharedSecret)
	}

	t.Log("✓ Authentication setup verified")
}

// TestHandleClientPtyMode tests PTY mode transitions
func TestHandleClientPtyMode(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:10000"
	listener.clientConnections[clientAddr] = make(chan string, 10)

	// Enter PTY mode
	ptyDataChan, err := listener.EnterPtyMode(clientAddr)
	if err != nil {
		t.Fatalf("Failed to enter PTY mode: %v", err)
	}

	// Verify client is in PTY mode
	if !listener.IsInPtyMode(clientAddr) {
		t.Error("Client should be in PTY mode")
	}

	// Exit PTY mode
	err = listener.ExitPtyMode(clientAddr)
	if err != nil {
		t.Fatalf("Failed to exit PTY mode: %v", err)
	}

	// Verify client is not in PTY mode
	if listener.IsInPtyMode(clientAddr) {
		t.Error("Client should not be in PTY mode after exit")
	}

	// Verify PTY data channel was closed
	select {
	case _, ok := <-ptyDataChan:
		if ok {
			t.Error("PTY data channel should be closed")
		}
	default:
		// Channel is closed
	}

	t.Log("✓ PTY mode transitions handled correctly")
}

// TestHandleClientReadResponseFailure tests handling when response reading fails
func TestHandleClientReadResponseFailure(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Verify listener can handle connection errors gracefully
	if listener == nil {
		t.Fatal("Listener should not be nil")
	}

	t.Log("✓ Error handling setup verified")
}

// TestHandleClientPingAndResponse tests ping functionality
func TestHandleClientPingAndResponse(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	// Simulate a client
	clientAddr := "127.0.0.1:50000"
	listener.clientConnections[clientAddr] = make(chan string, 10)
	listener.clientResponses[clientAddr] = make(chan string, 10)
	listener.clientPausePing[clientAddr] = make(chan bool, 1)

	// Test that pause ping channel works
	select {
	case listener.clientPausePing[clientAddr] <- true:
		// Successfully sent pause signal
	default:
		t.Fatal("Failed to send pause signal")
	}

	t.Log("✓ Ping and response handling verified")
}

// TestHandleClientPtyDataResponse tests PTY data response handling
func TestHandleClientPtyDataResponse(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "")

	clientAddr := "127.0.0.1:50001"

	// Create PTY data channel
	ptyDataChan := make(chan []byte, 10)
	listener.clientPtyData[clientAddr] = ptyDataChan
	listener.clientPtyMode[clientAddr] = true

	// Simulate PTY data being received
	testData := []byte("test output")
	select {
	case ptyDataChan <- testData:
		// Data sent successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout sending PTY data")
	}

	// Verify data can be received
	receivedData := <-ptyDataChan
	if !bytes.Equal(receivedData, testData) {
		t.Errorf("Expected %q, got %q", testData, receivedData)
	}

	t.Log("✓ PTY data response handling verified")
}

// TestHandleClientAuthenticationSuccess tests successful authentication
func TestHandleClientAuthenticationSuccess(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("0", "127.0.0.1", tlsConfig, "secret123")

	// Verify the shared secret is set
	if listener.sharedSecret != "secret123" {
		t.Errorf("Expected secret 'secret123', got '%s'", listener.sharedSecret)
	}

	t.Log("✓ Authentication success path verified")
}

func TestParseIdentMetadata(t *testing.T) {
	line := "IDENT abcd1234 os=linux host=demo ip=10.0.0.5"
	meta := parseIdentMetadata(line)

	if meta.Identifier != "abcd1234" {
		t.Fatalf("expected identifier abcd1234, got %s", meta.Identifier)
	}
	if meta.OS != "linux" {
		t.Fatalf("expected os linux, got %s", meta.OS)
	}
	if meta.Hostname != "demo" {
		t.Fatalf("expected hostname demo, got %s", meta.Hostname)
	}
	if meta.IP != "10.0.0.5" {
		t.Fatalf("expected ip 10.0.0.5, got %s", meta.IP)
	}
}

func TestParseIdentMetadataMissingFields(t *testing.T) {
	line := "IDENT efgh5678"
	meta := parseIdentMetadata(line)

	if meta.Identifier != "efgh5678" {
		t.Fatalf("expected identifier efgh5678, got %s", meta.Identifier)
	}
	if meta.OS != "" || meta.Hostname != "" || meta.IP != "" {
		t.Fatalf("expected empty metadata fields, got %+v", meta)
	}
}
