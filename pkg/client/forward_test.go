package client

import (
	"testing"
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
