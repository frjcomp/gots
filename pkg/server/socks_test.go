package server

import (
	"testing"
)

func TestSocksManager_StartSocks(t *testing.T) {
	sm := NewSocksManager()
	
	sendCalls := []string{}
	sendFunc := func(msg string) {
		sendCalls = append(sendCalls, msg)
	}
	
	err := sm.StartSocks("test1", "0", sendFunc)
	if err != nil {
		t.Fatalf("StartSocks failed: %v", err)
	}
	
	proxies := sm.ListSocks()
	if len(proxies) != 1 {
		t.Errorf("Expected 1 SOCKS proxy, got %d", len(proxies))
	}
	
	if proxies[0].ID != "test1" {
		t.Errorf("Expected ID 'test1', got %s", proxies[0].ID)
	}
	
	// Should have sent SOCKS_START command
	if len(sendCalls) == 0 {
		t.Error("Expected SOCKS_START to be sent")
	}
}

func TestSocksManager_StopSocks(t *testing.T) {
	sm := NewSocksManager()
	
	sendFunc := func(msg string) {}
	
	err := sm.StartSocks("test1", "0", sendFunc)
	if err != nil {
		t.Fatalf("StartSocks failed: %v", err)
	}
	
	err = sm.StopSocks("test1")
	if err != nil {
		t.Errorf("StopSocks failed: %v", err)
	}
	
	proxies := sm.ListSocks()
	if len(proxies) != 0 {
		t.Errorf("Expected 0 proxies, got %d", len(proxies))
	}
}

func TestSocksManager_DuplicateID(t *testing.T) {
	sm := NewSocksManager()
	
	sendFunc := func(msg string) {}
	
	err := sm.StartSocks("test1", "0", sendFunc)
	if err != nil {
		t.Fatalf("First StartSocks failed: %v", err)
	}
	
	err = sm.StartSocks("test1", "0", sendFunc)
	if err == nil {
		t.Error("Expected error for duplicate SOCKS ID, got nil")
	}
}

func TestSocksManager_StopAll(t *testing.T) {
	sm := NewSocksManager()
	
	sendFunc := func(msg string) {}
	
	_ = sm.StartSocks("test1", "0", sendFunc)
	_ = sm.StartSocks("test2", "0", sendFunc)
	
	sm.StopAll()
	
	proxies := sm.ListSocks()
	if len(proxies) != 0 {
		t.Errorf("Expected 0 proxies after StopAll, got %d", len(proxies))
	}
}
