package server

import (
	"net"
	"strings"
	"testing"
)

func TestForwardManager_StartForward(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendCalls := []string{}
	sendFunc := func(msg string) {
		sendCalls = append(sendCalls, msg)
	}
	
	err := fm.StartForward("test1", "0", "example.com:80", sendFunc)
	if err != nil {
		t.Fatalf("StartForward failed: %v", err)
	}
	
	forwards := fm.ListForwards()
	if len(forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(forwards))
	}
	
	if forwards[0].ID != "test1" {
		t.Errorf("Expected ID 'test1', got %s", forwards[0].ID)
	}
	
	if forwards[0].RemoteAddr != "example.com:80" {
		t.Errorf("Expected RemoteAddr 'example.com:80', got %s", forwards[0].RemoteAddr)
	}
}

func TestForwardManager_StopForward(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	err := fm.StartForward("test1", "0", "example.com:80", sendFunc)
	if err != nil {
		t.Fatalf("StartForward failed: %v", err)
	}
	
	err = fm.StopForward("test1")
	if err != nil {
		t.Errorf("StopForward failed: %v", err)
	}
	
	forwards := fm.ListForwards()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after stop, got %d", len(forwards))
	}
}

func TestForwardManager_DuplicateForwardID(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	err := fm.StartForward("test1", "0", "example.com:80", sendFunc)
	if err != nil {
		t.Fatalf("First StartForward failed: %v", err)
	}
	
	err = fm.StartForward("test1", "0", "example.com:443", sendFunc)
	if err == nil {
		t.Error("Expected error for duplicate forward ID, got nil")
	}
	
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestForwardManager_StopAll(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	_ = fm.StartForward("test1", "0", "example.com:80", sendFunc)
	_ = fm.StartForward("test2", "0", "example.com:443", sendFunc)
	
	fm.StopAll()
	
	forwards := fm.ListForwards()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after StopAll, got %d", len(forwards))
	}
}

// TestForwardManager_HandleForwardData_RoutesDataToCorrectConnection verifies that
// response data from the remote server is properly written to the correct curl connection
func TestForwardManager_HandleForwardData_RoutesDataToCorrectConnection(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	err := fm.StartForward("fwd-1", "0", "example.com:80", sendFunc)
	if err != nil {
		t.Fatalf("StartForward failed: %v", err)
	}
	
	// Get the forward info
	fm.mu.RLock()
	info := fm.forwards["fwd-1"]
	fm.mu.RUnlock()
	
	// Create a pipe to simulate a curl connection
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Store it in the forward
	info.mu.Lock()
	info.connections["1"] = server
	info.mu.Unlock()
	
	// Send response data via HandleForwardData
	testData := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nHello"
	encoded := "SFRUUC8xLjEgMjAwIE9LDQpDb250ZW50LUxlbmd0aDogNQ0KDQpIZWxsbw=="
	
	// Read in a goroutine to avoid deadlock on net.Pipe
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(testData))
		n, _ := client.Read(buf)
		readDone <- buf[:n]
	}()
	
	err = fm.HandleForwardData("fwd-1", "1", encoded)
	if err != nil {
		t.Fatalf("HandleForwardData failed: %v", err)
	}
	
	// Get the data that was read
	readData := <-readDone
	if string(readData) != testData {
		t.Errorf("Data mismatch: got %q, expected %q", string(readData), testData)
	}
}

// TestForwardManager_HandleForwardData_WrongConnID verifies proper error handling
// when data arrives for a non-existent connection
func TestForwardManager_HandleForwardData_WrongConnID(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	_ = fm.StartForward("fwd-1", "0", "example.com:80", sendFunc)
	
	// Try to send data for a connection that doesn't exist
	err := fm.HandleForwardData("fwd-1", "999", "dGVzdA==")
	if err == nil {
		t.Error("Expected error for non-existent connection, got nil")
	}
	
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestForwardManager_HandleForwardData_WrongForwardID verifies proper error handling
// when data arrives for a non-existent forward
func TestForwardManager_HandleForwardData_WrongForwardID(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	// Try to send data for a forward that doesn't exist
	err := fm.HandleForwardData("fwd-999", "1", "dGVzdA==")
	if err == nil {
		t.Error("Expected error for non-existent forward, got nil")
	}
	
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestForwardManager_ForwardStartIncludesConnID verifies that the ForwardInfo
// structure is properly initialized with connections map for tracking
func TestForwardManager_ForwardStartIncludesConnID(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	err := fm.StartForward("fwd-test", "0", "127.0.0.1:8080", sendFunc)
	if err != nil {
		t.Fatalf("StartForward failed: %v", err)
	}
	
	// Get the forward info
	fm.mu.RLock()
	info := fm.forwards["fwd-test"]
	fm.mu.RUnlock()
	
	if info == nil {
		t.Fatal("Forward info not created")
	}
	
	// Verify the structure is as expected
	if info.ID != "fwd-test" {
		t.Errorf("Expected ID 'fwd-test', got %s", info.ID)
	}
	
	if info.RemoteAddr != "127.0.0.1:8080" {
		t.Errorf("Expected RemoteAddr '127.0.0.1:8080', got %s", info.RemoteAddr)
	}
	
	// Verify connections map exists for tracking
	if info.connections == nil {
		t.Error("Expected connections map to be initialized")
	}
}

// TestForwardManager_ConnectionCleanup verifies that connections are properly
// cleaned up when the forward is stopped
func TestForwardManager_ConnectionCleanup(t *testing.T) {
	fm := NewForwardManager()
	defer fm.StopAll()
	
	sendFunc := func(msg string) {}
	
	err := fm.StartForward("fwd-cleanup", "0", "example.com:80", sendFunc)
	if err != nil {
		t.Fatalf("StartForward failed: %v", err)
	}
	
	fm.mu.RLock()
	info := fm.forwards["fwd-cleanup"]
	fm.mu.RUnlock()
	
	// Add some fake connections
	client1, server1 := net.Pipe()
	client2, server2 := net.Pipe()
	defer client1.Close()
	defer client2.Close()
	defer server1.Close()
	defer server2.Close()
	
	info.mu.Lock()
	info.connections["1"] = server1
	info.connections["2"] = server2
	info.mu.Unlock()
	
	// Stop the forward
	err = fm.StopForward("fwd-cleanup")
	if err != nil {
		t.Fatalf("StopForward failed: %v", err)
	}
	
	// Verify forward is deleted
	fm.mu.RLock()
	_, exists := fm.forwards["fwd-cleanup"]
	fm.mu.RUnlock()
	
	if exists {
		t.Error("Expected forward to be deleted after StopForward")
	}
}
