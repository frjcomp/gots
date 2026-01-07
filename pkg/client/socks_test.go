package client

import (
	"net"
	"strings"
	"testing"
	"time"
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

// TestSocksHandler_RaceConditionOnClose verifies that closing a connection
// while readFromTarget is running doesn't cause "use of closed network connection" errors
func TestSocksHandler_RaceConditionOnClose(t *testing.T) {
	sendMessages := []string{}
	sendFunc := func(msg string) {
		sendMessages = append(sendMessages, msg)
	}
	sh := NewSocksHandler(sendFunc)
	
	// Start a SOCKS proxy
	_ = sh.HandleSocksStart("test-socks")
	
	// Create a pipe to simulate a remote connection
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Manually set up a connection (simulating HandleSocksConn)
	sh.mu.Lock()
	sh.connections["test-socks"]["conn1"] = server
	stopChan := make(chan struct{})
	sh.stopChans["test-socks"]["conn1"] = stopChan
	sh.mu.Unlock()
	
	// Start the read loop
	go sh.readFromTarget("test-socks", "conn1", server, stopChan)
	
	// Allow read goroutine to start
	time.Sleep(10 * time.Millisecond)
	
	// Now close the connection via HandleSocksClose while read is active
	// This should NOT cause errors in the logs
	sh.HandleSocksClose("test-socks", "conn1")
	
	// Give the read goroutine time to finish
	time.Sleep(50 * time.Millisecond)
	
	// Verify the connection was closed cleanly
	sh.mu.RLock()
	conns, exists := sh.connections["test-socks"]
	sh.mu.RUnlock()
	
	if exists && len(conns) != 0 {
		t.Error("Expected connection to be removed after HandleSocksClose")
	}
}

// TestSocksHandler_CloseSignalsReadGoroutine verifies that stop channels
// properly signal read goroutines to exit
func TestSocksHandler_CloseSignalsReadGoroutine(t *testing.T) {
	sendMessages := []string{}
	sendFunc := func(msg string) {
		sendMessages = append(sendMessages, msg)
	}
	sh := NewSocksHandler(sendFunc)
	
	// Start a SOCKS proxy
	_ = sh.HandleSocksStart("test-socks")
	
	// Create a pipe
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	
	// Set up connection
	sh.mu.Lock()
	sh.connections["test-socks"]["conn2"] = server
	stopChan := make(chan struct{})
	sh.stopChans["test-socks"]["conn2"] = stopChan
	sh.mu.Unlock()
	
	// Track if read goroutine exits
	readDone := make(chan struct{})
	
	// Start read with wrapper to track completion
	go func() {
		sh.readFromTarget("test-socks", "conn2", server, stopChan)
		close(readDone)
	}()
	
	// Give it time to start
	time.Sleep(10 * time.Millisecond)
	
	// Close via HandleSocksClose
	sh.HandleSocksClose("test-socks", "conn2")
	
	// Wait for read goroutine to exit (with timeout)
	select {
	case <-readDone:
		// Success - goroutine exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Error("read goroutine did not exit after close signal")
	}
}

// TestSocksHandler_HandleSocksConnThenClose verifies the full connection lifecycle
func TestSocksHandler_HandleSocksConnThenClose(t *testing.T) {
	sendMessages := []string{}
	sendFunc := func(msg string) {
		sendMessages = append(sendMessages, msg)
	}
	sh := NewSocksHandler(sendFunc)
	
	// Start SOCKS proxy
	_ = sh.HandleSocksStart("test-socks")
	
	// Create a listener to accept the outbound connection
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()
	
	listenerAddr := listener.Addr().String()
	
	// Accept connections in a goroutine
	connDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			connDone <- err
			return
		}
		defer conn.Close()
		
		// Read some data then close
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		if n > 0 {
			conn.Write(buf[:n])
		}
		connDone <- nil
	}()
	
	// Connect to the listener via HandleSocksConn
	err = sh.HandleSocksConn("test-socks", "conn3", listenerAddr)
	if err != nil {
		t.Fatalf("HandleSocksConn failed: %v", err)
	}
	
	// Give connection time to establish
	time.Sleep(50 * time.Millisecond)
	
	// Verify connection exists
	sh.mu.RLock()
	conns, _ := sh.connections["test-socks"]
	sh.mu.RUnlock()
	
	if len(conns) != 1 {
		t.Errorf("Expected 1 connection, got %d", len(conns))
	}
	
	// Close the connection
	sh.HandleSocksClose("test-socks", "conn3")
	
	// Wait for listener goroutine to finish
	<-connDone
	
	// Verify connection is cleaned up
	sh.mu.RLock()
	conns, _ = sh.connections["test-socks"]
	sh.mu.RUnlock()
	
	if len(conns) != 0 {
		t.Errorf("Expected 0 connections after close, got %d", len(conns))
	}
	
	// Verify SOCKS_OK was sent
	var foundOk bool
	for _, msg := range sendMessages {
		if strings.Contains(msg, "SOCKS_OK") {
			foundOk = true
			break
		}
	}
	if !foundOk {
		t.Error("Expected SOCKS_OK message to be sent")
	}
}

// TestSocksHandler_MultipleConnectionsClose verifies that Close() properly
// signals all read goroutines
func TestSocksHandler_MultipleConnectionsClose(t *testing.T) {
	sendFunc := func(msg string) {}
	sh := NewSocksHandler(sendFunc)
	
	// Start SOCKS proxy
	_ = sh.HandleSocksStart("test-socks")
	
	// Create multiple pipes
	clients := make([]net.Conn, 3)
	servers := make([]net.Conn, 3)
	stopChans := make([]chan struct{}, 3)
	readDones := make([]chan struct{}, 3)
	
	for i := 0; i < 3; i++ {
		c, s := net.Pipe()
		clients[i] = c
		servers[i] = s
		
		sh.mu.Lock()
		stopChan := make(chan struct{})
		stopChans[i] = stopChan
		sh.connections["test-socks"][string(rune(i))] = s
		sh.stopChans["test-socks"][string(rune(i))] = stopChan
		sh.mu.Unlock()
		
		// Start read goroutine
		done := make(chan struct{})
		readDones[i] = done
		go func(idx int) {
			sh.readFromTarget("test-socks", string(rune(idx)), servers[idx], stopChans[idx])
			close(done)
		}(i)
	}
	
	// Give read goroutines time to start
	time.Sleep(50 * time.Millisecond)
	
	// Call Close() - should signal all read goroutines
	sh.Close()
	
	// Wait for all read goroutines to exit
	for i := 0; i < 3; i++ {
		select {
		case <-readDones[i]:
			// Success
		case <-time.After(500 * time.Millisecond):
			t.Errorf("read goroutine %d did not exit", i)
		}
		clients[i].Close()
	}
	
	// Verify cleanup
	sh.mu.RLock()
	if len(sh.connections) != 0 {
		t.Errorf("Expected 0 SOCKS proxies, got %d", len(sh.connections))
	}
	sh.mu.RUnlock()
}
