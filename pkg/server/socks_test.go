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

	// Write on server side in a separate goroutine to avoid net.Pipe deadlock
	done := make(chan error, 1)
	go func() {
		done <- sm.HandleSocksData(proxy.ID, connID, encoded)
	}()

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
	if writeErr := <-done; writeErr != nil {
		t.Fatalf("HandleSocksData error: %v", writeErr)
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

// TestHandleSocksConnectionIPv4Response tests that IPv4 address type is echoed in response
func TestHandleSocksConnectionIPv4Response(t *testing.T) {
	sm := NewSocksManager()
	sink := &cmdSink{ch: make(chan string, 10)}
	
	proxy := &SocksProxy{
		ID:          "test-proxy",
		LocalAddr:   "127.0.0.1:9050",
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
		sendFunc:    sink.send,
	}
	
	sm.mu.Lock()
	sm.proxies[proxy.ID] = proxy
	sm.mu.Unlock()
	
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Signal ready chan immediately to skip client connection wait
	readyChan := make(chan bool, 1)
	proxy.mu.Lock()
	proxy.connReady["conn1"] = readyChan
	proxy.mu.Unlock()
	
	// Handle connection in background
	go sm.handleSocksConnection(proxy, "conn1", server)
	
	// Send SOCKS5 greeting
	_, err := client.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to send greeting: %v", err)
	}
	
	// Read greeting response: [version, auth_method]
	buf := make([]byte, 2)
	_, err = client.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read greeting response: %v", err)
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		t.Fatalf("Unexpected greeting response: %v", buf)
	}
	
	// Send SOCKS5 IPv4 connect request to 192.0.2.1:80
	// [version, cmd, reserved, addr_type, ip[4], port[2]]
	request := []byte{
		0x05, 0x01, 0x00, 0x01,           // version, connect, reserved, IPv4
		192, 0, 2, 1,                     // 192.0.2.1
		0x00, 0x50,                       // port 80
	}
	_, err = client.Write(request)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	
	// Signal ready to allow connection to proceed
	go func() {
		time.Sleep(100 * time.Millisecond)
		select {
		case readyChan <- true:
		default:
		}
	}()
	
	// Read response and check it contains IPv4 address type
	response := make([]byte, 10)
	_, err = client.Read(response)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	
	// Response should be: [version, status, reserved, addr_type, addr[4], port[2]]
	if response[0] != 0x05 {
		t.Errorf("Expected version 0x05, got 0x%02x", response[0])
	}
	if response[3] != 0x01 {
		t.Errorf("Expected IPv4 address type 0x01, got 0x%02x", response[3])
	}
}

// TestHandleSocksConnectionIPv6Response tests that IPv6 address type is echoed in response
func TestHandleSocksConnectionIPv6Response(t *testing.T) {
	sm := NewSocksManager()
	sink := &cmdSink{ch: make(chan string, 10)}
	
	proxy := &SocksProxy{
		ID:          "test-proxy",
		LocalAddr:   "127.0.0.1:9050",
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
		sendFunc:    sink.send,
	}
	
	sm.mu.Lock()
	sm.proxies[proxy.ID] = proxy
	sm.mu.Unlock()
	
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	connID := "conn2"
	
	// Handle connection in background
	go func() {
		// Signal ready chan after a delay to allow request to be processed
		time.Sleep(50 * time.Millisecond)
		proxy.mu.Lock()
		readyChan, exists := proxy.connReady[connID]
		proxy.mu.Unlock()
		if exists && readyChan != nil {
			select {
			case readyChan <- true:
			default:
			}
		}
	}()
	
	go sm.handleSocksConnection(proxy, connID, server)
	
	// Send SOCKS5 greeting
	_, err := client.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to send greeting: %v", err)
	}
	
	// Read greeting response
	buf := make([]byte, 2)
	_, err = client.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read greeting response: %v", err)
	}
	
	// Send SOCKS5 IPv6 connect request to [2001:db8::1]:443
	// [version, cmd, reserved, addr_type, ip[16], port[2]]
	request := []byte{
		0x05, 0x01, 0x00, 0x04,                           // version, connect, reserved, IPv6
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,  // 2001:db8:0:0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,  // ::1
		0x01, 0xbb,                                       // port 443
	}
	_, err = client.Write(request)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	
	// Read response and check it contains IPv6 address type
	response := make([]byte, 22)
	_, err = client.Read(response)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	
	// Response should be: [version, status, reserved, addr_type, addr[16], port[2]]
	if response[0] != 0x05 {
		t.Errorf("Expected version 0x05, got 0x%02x", response[0])
	}
	if response[3] != 0x04 {
		t.Errorf("Expected IPv6 address type 0x04, got 0x%02x", response[3])
	}
	// Verify the IPv6 address bytes are echoed back
	if !net.IP(response[4:20]).Equal(net.ParseIP("2001:db8::1")) {
		t.Errorf("IPv6 address not echoed correctly in response, got %v", net.IP(response[4:20]))
	}
}

// TestHandleSocksConnectionDomainResponse tests that domain address type is handled in response
func TestHandleSocksConnectionDomainResponse(t *testing.T) {
	sm := NewSocksManager()
	sink := &cmdSink{ch: make(chan string, 10)}
	
	proxy := &SocksProxy{
		ID:          "test-proxy",
		LocalAddr:   "127.0.0.1:9050",
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
		sendFunc:    sink.send,
	}
	
	sm.mu.Lock()
	sm.proxies[proxy.ID] = proxy
	sm.mu.Unlock()
	
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	connID := "conn3"
	
	// Handle connection in background
	go func() {
		// Signal ready chan after a delay to allow request to be processed
		time.Sleep(50 * time.Millisecond)
		proxy.mu.Lock()
		readyChan, exists := proxy.connReady[connID]
		proxy.mu.Unlock()
		if exists && readyChan != nil {
			select {
			case readyChan <- true:
			default:
			}
		}
	}()
	
	go sm.handleSocksConnection(proxy, connID, server)
	
	// Send SOCKS5 greeting
	_, err := client.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to send greeting: %v", err)
	}
	
	// Read greeting response
	buf := make([]byte, 2)
	_, err = client.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read greeting response: %v", err)
	}
	
	// Send SOCKS5 domain connect request to example.com:443
	// [version, cmd, reserved, addr_type, domain_len, domain, port[2]]
	domain := "example.com"
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(domain))}
	request = append(request, []byte(domain)...)
	request = append(request, 0x01, 0xbb) // port 443
	
	_, err = client.Write(request)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	
	// Read response and check it contains domain address type
	response := make([]byte, 256)
	n, err := client.Read(response)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
	response = response[:n]
	
	// Response should be: [version, status, reserved, addr_type, domain_len, domain, port[2]]
	if response[0] != 0x05 {
		t.Errorf("Expected version 0x05, got 0x%02x", response[0])
	}
	if response[3] != 0x03 {
		t.Errorf("Expected domain address type 0x03, got 0x%02x", response[3])
	}
	// Verify the domain is echoed back
	if response[4] != byte(len(domain)) {
		t.Errorf("Domain length not correct, expected %d got %d", len(domain), response[4])
	}
}
