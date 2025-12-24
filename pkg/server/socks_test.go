package server

import (
	"encoding/base64"
	"net"
	"strings"
	"testing"
	"time"
)

// helper to capture sent commands
type cmdSink struct {
	msgs []string
	ch   chan string
}

func (c *cmdSink) send(s string) {
	c.msgs = append(c.msgs, s)
	select {
	case c.ch <- s:
	default:
	}
}

func TestRelayDataSendsSocksData(t *testing.T) {
	sm := NewSocksManager()
	proxy := &SocksProxy{
		ID:          "test-socks",
		LocalAddr:   "",
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
	}
	sink := &cmdSink{ch: make(chan string, 1)}
	proxy.sendFunc = sink.send

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	connID := "1"
	proxy.connections[connID] = server

	// start relay in background
	go sm.relayData(proxy, connID, server)

	// write some bytes from the local client (curl side)
	payload := []byte("hello world")
	if _, err := client.Write(payload); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	client.Close()

	// wait for relay to send message (with timeout)
	var msg string
	select {
	case m := <-sink.ch:
		msg = strings.TrimSpace(m)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no SOCKS_DATA sent by relayData within timeout")
	}
	if !strings.HasPrefix(msg, "SOCKS_DATA ") {
		t.Fatalf("expected SOCKS_DATA prefix, got: %q", msg)
	}

	parts := strings.Fields(msg)
	if len(parts) < 4 {
		t.Fatalf("expected 4 parts, got %d: %q", len(parts), msg)
	}
	encoded := parts[3]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("payload mismatch: got %q want %q", string(decoded), string(payload))
	}
}

func TestHandleSocksDataWritesToLocalConn(t *testing.T) {
	sm := NewSocksManager()
	proxy := &SocksProxy{
		ID:          "test-socks",
		LocalAddr:   "",
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
		sendFunc:    func(string) {},
	}

	sm.mu.Lock()
	sm.proxies[proxy.ID] = proxy
	sm.mu.Unlock()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	connID := "2"
	proxy.mu.Lock()
	proxy.connections[connID] = server
	proxy.mu.Unlock()

	data := []byte("response-from-remote")
	encoded := base64.StdEncoding.EncodeToString(data)

	if err := sm.HandleSocksData(proxy.ID, connID, encoded); err != nil {
		t.Fatalf("HandleSocksData error: %v", err)
	}

	buf := make([]byte, len(data))
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("client read error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("read size mismatch: got %d want %d", n, len(data))
	}
	if string(buf) != string(data) {
		t.Fatalf("data mismatch: got %q want %q", string(buf), string(data))
	}
}

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
