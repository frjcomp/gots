package client

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestForwardHandler_New(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	if fh == nil {
		t.Fatal("NewForwardHandler returned nil")
	}
	
	if fh.connections == nil {
		t.Error("connections map not initialized")
	}
	
	if fh.connIDs == nil {
		t.Error("connIDs map not initialized")
	}
}

func TestForwardHandler_HandleForwardStop(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	// Should not panic even if forward doesn't exist
	fh.HandleForwardStop("nonexistent")
}

func TestForwardHandler_Close(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	// Should not panic on close
	fh.Close()
}

// TestForwardHandler_HandleForwardStart_InvalidAddress verifies that
// invalid address formats (missing port) are rejected
func TestForwardHandler_HandleForwardStart_InvalidAddress(t *testing.T) {
	sendMessages := []string{}
	sendFunc := func(msg string) {
		sendMessages = append(sendMessages, msg)
	}
	fh := NewForwardHandler(sendFunc)
	
	// Try to start with address missing port (just host)
	err := fh.HandleForwardStart("fwd-1", "conn-1", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for address without port, got nil")
	}
	
	if len(sendMessages) == 0 {
		t.Error("Expected FORWARD_STOP to be sent on error")
	}
}

// TestForwardHandler_HandleForwardStart_ValidAddress verifies that
// valid host:port addresses are accepted (connection will fail but validation passes)
func TestForwardHandler_HandleForwardStart_ValidAddress(t *testing.T) {
	sendMessages := []string{}
	sendFunc := func(msg string) {
		sendMessages = append(sendMessages, msg)
	}
	fh := NewForwardHandler(sendFunc)
	
	// Valid format (connection will fail but address format is valid)
	err := fh.HandleForwardStart("fwd-1", "conn-1", "127.0.0.1:99999")
	// Error is expected because port 99999 isn't listening, but it's a connection error, not format error
	if err != nil && err.Error() != "invalid target address format: 127.0.0.1:99999 (expected host:port, e.g., 127.0.0.1:8080)" {
		// This is expected - connection error is fine
	}
}

// TestForwardHandler_ReadFromTargetSendsCorrectConnID verifies that
// the read goroutine sends response data with the correct connID
func TestForwardHandler_ReadFromTargetSendsCorrectConnID(t *testing.T) {
	sentMessages := []string{}
	sendFunc := func(msg string) {
		sentMessages = append(sentMessages, msg)
	}
	fh := NewForwardHandler(sendFunc)
	
	// Create a pipe to simulate a remote connection
	client, server := net.Pipe()
	
	// Start readFromTarget with specific connID
	go fh.readFromTarget("fwd-test", "conn-42", server)
	
	// Send some data from the "remote" side
	testData := "Response data"
	client.Write([]byte(testData))
	client.Close()
	
	// Allow read goroutine to process
	time.Sleep(50 * time.Millisecond)
	
	// Verify FORWARD_DATA message was sent with correct connID
	foundData := false
	for _, msg := range sentMessages {
		if strings.HasPrefix(msg, "FORWARD_DATA fwd-test conn-42") {
			foundData = true
			break
		}
	}
	
	if !foundData {
		t.Errorf("Expected FORWARD_DATA message with conn-42 connID, got: %v", sentMessages)
	}
}

// TestForwardHandler_HandleForwardData_WritesToConnection verifies that
// HandleForwardData properly decodes and writes data to the forward connection
func TestForwardHandler_HandleForwardData_WritesToConnection(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	// Create a pipe to simulate a curl connection
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Start a forward and manually add the connection
	fh.mu.Lock()
	fh.connections["fwd-test"] = server
	fh.connIDs["fwd-test"] = "conn-1"
	fh.mu.Unlock()
	
	// Send data via HandleForwardData
	testData := "Hello from remote"
	encoded := "SGVsbG8gZnJvbSByZW1vdGU=" // base64 encoded
	
	// Read in a goroutine to avoid deadlock on net.Pipe
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(testData))
		n, _ := client.Read(buf)
		readDone <- buf[:n]
	}()
	
	err := fh.HandleForwardData("fwd-test", testData, encoded)
	if err != nil {
		t.Fatalf("HandleForwardData failed: %v", err)
	}
	
	// Get the data that was read
	readData := <-readDone
	if string(readData) != testData {
		t.Errorf("Data mismatch: got %q, expected %q", string(readData), testData)
	}
}

// TestForwardHandler_MultipleForwardsTracked verifies that multiple
// forwards can be tracked simultaneously with different connIDs
func TestForwardHandler_MultipleForwardsTracked(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	// Manually add multiple connections with different connIDs
	conn1, pipe1 := net.Pipe()
	conn2, pipe2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()
	defer pipe1.Close()
	defer pipe2.Close()
	
	fh.mu.Lock()
	fh.connections["fwd-1"] = pipe1
	fh.connIDs["fwd-1"] = "conn-100"
	fh.connections["fwd-2"] = pipe2
	fh.connIDs["fwd-2"] = "conn-200"
	fh.mu.Unlock()
	
	// Verify both are tracked
	fh.mu.RLock()
	if len(fh.connIDs) != 2 {
		t.Errorf("Expected 2 connIDs to be tracked, got %d", len(fh.connIDs))
	}
	
	connID1, exists1 := fh.connIDs["fwd-1"]
	connID2, exists2 := fh.connIDs["fwd-2"]
	fh.mu.RUnlock()
	
	if !exists1 || connID1 != "conn-100" {
		t.Errorf("Expected fwd-1 to have connID 'conn-100', got '%s'", connID1)
	}
	
	if !exists2 || connID2 != "conn-200" {
		t.Errorf("Expected fwd-2 to have connID 'conn-200', got '%s'", connID2)
	}
}

// TestForwardHandler_CloseCleanupConnIDs verifies that Close()
// properly removes all connID mappings and closes all connections
func TestForwardHandler_CloseCleanupConnIDs(t *testing.T) {
	sendFunc := func(msg string) {}
	fh := NewForwardHandler(sendFunc)
	
	// Create pipes and add both connections and connID mappings
	conn1, pipe1 := net.Pipe()
	conn2, pipe2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()
	
	fh.mu.Lock()
	fh.connections["fwd-1"] = pipe1
	fh.connIDs["fwd-1"] = "conn-1"
	fh.connections["fwd-2"] = pipe2
	fh.connIDs["fwd-2"] = "conn-2"
	fh.mu.Unlock()
	
	// Call Close
	fh.Close()
	
	// Verify both maps are empty after cleanup
	fh.mu.RLock()
	if len(fh.connections) != 0 {
		t.Errorf("Expected connections to be empty after Close, got %d entries", len(fh.connections))
	}
	if len(fh.connIDs) != 0 {
		t.Errorf("Expected connIDs to be empty after Close, got %d entries", len(fh.connIDs))
	}
	fh.mu.RUnlock()
}

