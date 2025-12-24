package client

import (
	"testing"
)

func TestSocksHandler_New(t *testing.T) {
	sendFunc := func(msg string) {}
	sh := NewSocksHandler(sendFunc)
	
	if sh == nil {
		t.Fatal("NewSocksHandler returned nil")
	}
	
	if sh.connections == nil {
		t.Error("connections map not initialized")
	}
}

func TestSocksHandler_HandleSocksStart(t *testing.T) {
	sendFunc := func(msg string) {}
	sh := NewSocksHandler(sendFunc)
	
	err := sh.HandleSocksStart("test1")
	if err != nil {
		t.Errorf("HandleSocksStart failed: %v", err)
	}
	
	// Should have created the connections map for this SOCKS ID
	sh.mu.RLock()
	_, exists := sh.connections["test1"]
	sh.mu.RUnlock()
	
	if !exists {
		t.Error("Expected SOCKS proxy map to be created")
	}
}

func TestSocksHandler_HandleSocksClose(t *testing.T) {
	sendFunc := func(msg string) {}
	sh := NewSocksHandler(sendFunc)
	
	// Should not panic even if connection doesn't exist
	sh.HandleSocksClose("nonexistent", "conn1")
}

func TestSocksHandler_Close(t *testing.T) {
	sendFunc := func(msg string) {}
	sh := NewSocksHandler(sendFunc)
	
	// Initialize a SOCKS proxy
	_ = sh.HandleSocksStart("test1")
	
	// Should not panic on close
	sh.Close()
	
	// Should have cleaned up
	sh.mu.RLock()
	count := len(sh.connections)
	sh.mu.RUnlock()
	
	if count != 0 {
		t.Errorf("Expected 0 connections after close, got %d", count)
	}
}
