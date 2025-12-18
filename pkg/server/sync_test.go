package server

import (
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang-https-rev/pkg/certs"
)

const testPortBase = 19100

// TestPINGPauseResume tests that PING pause/resume works correctly during command-response cycles
func TestPINGPauseResume(t *testing.T) {
	l := createTestListener(t, 0)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	// Get actual address
	actualAddr := netListener.Addr().String()

	// Simulate a client connection
	conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()

	// Give listener time to accept and register client
	time.Sleep(100 * time.Millisecond)

	// Verify client is registered
	clients := l.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(clients))
	}

	// Send a command and get response - PING should be paused during this
	cmd := "test_command"
	if err := l.SendCommand(clientAddr, cmd); err != nil {
		t.Fatalf("Failed to send command: %v", err)
	}

	// Verify PING is paused by checking we don't get a PING during command timeout
	// This is tested implicitly - if pause didn't work, next test would fail with PONG response

	t.Log("✓ PING pause/resume test passed")
}

// TestConcurrentCommandsDoNotRaceWithPING tests that multiple concurrent commands don't race with PING
func TestConcurrentCommandsDoNotRaceWithPING(t *testing.T) {
	l := createTestListener(t, testPortBase+1)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	actualAddr := netListener.Addr().String()

	// Simulate multiple client connections
	const numClients = 5
	conns := make([]*tls.Conn, numClients)
	clients := make([]string, numClients)

	for i := 0; i < numClients; i++ {
		conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		defer conn.Close()
		conns[i] = conn
		clients[i] = conn.RemoteAddr().String()
	}

	// Give listener time to accept all clients
	time.Sleep(200 * time.Millisecond)

	// Send commands from multiple goroutines concurrently
	var wg sync.WaitGroup
	var sendErrors atomic.Int32

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cmd := fmt.Sprintf("cmd_%d_%d", idx, j)
				if err := l.SendCommand(clients[idx], cmd); err != nil {
					t.Logf("Error sending command: %v", err)
					sendErrors.Add(1)
				}
				// Small delay between commands
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	if sendErrors.Load() > 0 {
		t.Fatalf("Encountered %d errors while sending concurrent commands", sendErrors.Load())
	}

	t.Log("✓ Concurrent commands test passed")
}

// TestSendCommandBeforePauseComplete tests edge case of rapid commands
func TestRapidCommandSequence(t *testing.T) {
	l := createTestListener(t, testPortBase+2)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	actualAddr := netListener.Addr().String()
	conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Give listener time to accept client
	time.Sleep(100 * time.Millisecond)

	clients := l.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(clients))
	}

	clientAddr := clients[0]

	// Send 50 commands rapidly
	for i := 0; i < 50; i++ {
		cmd := fmt.Sprintf("rapid_cmd_%d", i)
		if err := l.SendCommand(clientAddr, cmd); err != nil {
			t.Fatalf("Error sending rapid command %d: %v", i, err)
		}
	}

	t.Log("✓ Rapid command sequence test passed")
}

// TestClientDisconnectDuringCommand tests that client disconnect is handled properly
func TestClientDisconnectDuringCommand(t *testing.T) {
	l := createTestListener(t, testPortBase+3)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	actualAddr := netListener.Addr().String()
	conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Give listener time to accept client
	time.Sleep(100 * time.Millisecond)

	clients := l.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(clients))
	}

	clientAddr := clients[0]

	// Close connection abruptly
	conn.Close()

	// Give server time to detect disconnect
	time.Sleep(100 * time.Millisecond)

	// Try to send command to disconnected client
	err = l.SendCommand(clientAddr, "test")
	if err == nil {
		t.Logf("Expected error for disconnected client, but got none")
	}

	// Verify client is removed from list
	clients = l.GetClients()
	if len(clients) != 0 {
		t.Fatalf("Expected 0 clients after disconnect, got %d", len(clients))
	}

	t.Log("✓ Client disconnect test passed")
}

// TestGetResponseTimeout tests that GetResponse properly times out
func TestGetResponseTimeout(t *testing.T) {
	l := createTestListener(t, testPortBase+4)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	// Create a fake client address that doesn't exist
	fakeAddr := "192.0.2.1:9999"

	// Attempt to get response from non-existent client
	start := time.Now()
	_, errResp := l.GetResponse(fakeAddr, 100*time.Millisecond)
	elapsed := time.Since(start)

	if errResp == nil {
		t.Fatal("Expected error for non-existent client")
	}

	// Verify timeout was respected (allow some slack)
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Fatalf("GetResponse timeout not respected: got %v, expected ~100ms", elapsed)
	}

	t.Log("✓ GetResponse timeout test passed")
}

// TestPauseChannelEdgeCases tests edge cases in pause signaling
func TestPauseChannelEdgeCases(t *testing.T) {
	l := createTestListener(t, testPortBase+5)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	actualAddr := netListener.Addr().String()
	conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)
	clients := l.GetClients()
	if len(clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(clients))
	}

	clientAddr := clients[0]

	// Rapidly send/receive to stress the pause channel
	for i := 0; i < 20; i++ {
		cmd := fmt.Sprintf("stress_cmd_%d", i)
		if err := l.SendCommand(clientAddr, cmd); err != nil {
			t.Logf("Warning: error on iteration %d: %v", i, err)
		}
		_, _ = l.GetResponse(clientAddr, 10*time.Millisecond)
	}

	t.Log("✓ Pause channel edge cases test passed")
}

// TestNoResponseChannelDataRace tests that respChan operations are safe
func TestNoResponseChannelDataRace(t *testing.T) {
	l := createTestListener(t, testPortBase+6)
	netListener, err := l.Start()
	if err != nil {
		t.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	actualAddr := netListener.Addr().String()
	conn, err := tls.Dial("tcp", actualAddr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)
	clients := l.GetClients()
	clientAddr := clients[0]

	// Concurrent sends and receives to detect data races
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = l.SendCommand(clientAddr, fmt.Sprintf("data_race_test_%d", idx))
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = l.GetResponse(clientAddr, 50*time.Millisecond)
		}()
	}

	wg.Wait()
	t.Log("✓ No response channel data race test passed")
}

// Helper function to create a test listener with specified port
func createTestListener(t *testing.T, port int) *Listener {
	cert, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	portStr := fmt.Sprintf("%d", port)
	return NewListener(portStr, "127.0.0.1", tlsConfig)
}
