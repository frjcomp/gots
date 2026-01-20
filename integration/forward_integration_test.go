package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/server"
)

func TestPortForwardBasic(t *testing.T) {
	// This is a basic smoke test for the port forwarding manager
	fm := server.NewForwardManager()

	// Test that we can create a forward manager
	if fm == nil {
		t.Fatal("Failed to create forward manager")
	}

	// Test starting a forward (will fail to connect, but tests the API)
	sendCalls := []string{}
	sendFunc := func(msg string) {
		sendCalls = append(sendCalls, msg)
	}

	err := fm.StartForward("test", "0", "localhost:1", sendFunc)
	if err != nil {
		t.Fatalf("Failed to start forward: %v", err)
	}

	// Give it a moment to start listening
	time.Sleep(100 * time.Millisecond)

	// List forwards
	forwards := fm.ListForwards()
	if len(forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(forwards))
	}

	// Stop forward
	err = fm.StopForward("test")
	if err != nil {
		t.Errorf("Failed to stop forward: %v", err)
	}

	// Verify it's stopped
	forwards = fm.ListForwards()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after stop, got %d", len(forwards))
	}
}

func TestSocksBasic(t *testing.T) {
	// This is a basic smoke test for the SOCKS5 proxy manager
	sm := server.NewSocksManager()

	// Test that we can create a SOCKS manager
	if sm == nil {
		t.Fatal("Failed to create SOCKS manager")
	}

	// Test starting a SOCKS proxy
	sendCalls := []string{}
	sendFunc := func(msg string) {
		sendCalls = append(sendCalls, msg)
	}

	err := sm.StartSocks("test", "0", sendFunc)
	if err != nil {
		t.Fatalf("Failed to start SOCKS: %v", err)
	}

	// Give it a moment to start listening
	time.Sleep(100 * time.Millisecond)

	// List SOCKS proxies
	proxies := sm.ListSocks()
	if len(proxies) != 1 {
		t.Errorf("Expected 1 SOCKS proxy, got %d", len(proxies))
	}

	// Should have sent SOCKS_START
	if len(sendCalls) == 0 {
		t.Error("Expected SOCKS_START to be sent")
	}

	// Stop SOCKS
	err = sm.StopSocks("test")
	if err != nil {
		t.Errorf("Failed to stop SOCKS: %v", err)
	}

	// Verify it's stopped
	proxies = sm.ListSocks()
	if len(proxies) != 0 {
		t.Errorf("Expected 0 proxies after stop, got %d", len(proxies))
	}
}

func TestForwardConnectionAcceptance(t *testing.T) {
	// Test that forward can actually accept connections
	fm := server.NewForwardManager()

	sendFunc := func(msg string) {
		// Capture sent messages
	}

	err := fm.StartForward("test", "0", "localhost:1", sendFunc)
	if err != nil {
		t.Fatalf("Failed to start forward: %v", err)
	}
	defer fm.StopForward("test")

	// Get the actual port
	forwards := fm.ListForwards()
	if len(forwards) == 0 {
		t.Fatal("No forwards found")
	}

	localAddr := forwards[0].LocalAddr

	// Try to connect to the forward port
	conn, err := net.DialTimeout("tcp", localAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to forward port: %v", err)
	}
	defer conn.Close()

	// Connection accepted - success!
	t.Logf("Successfully connected to forward at %s", localAddr)
}

func TestSocksConnectionAcceptance(t *testing.T) {
	// Test that SOCKS proxy can actually accept connections
	sm := server.NewSocksManager()

	sendFunc := func(msg string) {
		// Capture sent messages
	}

	err := sm.StartSocks("test", "0", sendFunc)
	if err != nil {
		t.Fatalf("Failed to start SOCKS: %v", err)
	}
	defer sm.StopSocks("test")

	// Get the actual port
	proxies := sm.ListSocks()
	if len(proxies) == 0 {
		t.Fatal("No SOCKS proxies found")
	}

	localAddr := proxies[0].LocalAddr

	// Try to connect to the SOCKS port
	conn, err := net.DialTimeout("tcp", localAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to SOCKS port: %v", err)
	}
	defer conn.Close()

	// Send SOCKS5 version negotiation
	// Version 5, 1 auth method, no auth (0x00)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to write version: %v", err)
	}

	// Read response (should be version 5, auth method 0)
	buf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if n != 2 {
		t.Errorf("Expected 2 bytes, got %d", n)
	}

	if buf[0] != 0x05 {
		t.Errorf("Expected SOCKS version 5, got %d", buf[0])
	}

	if buf[1] != 0x00 {
		t.Errorf("Expected auth method 0, got %d", buf[1])
	}

	t.Logf("Successfully negotiated SOCKS5 at %s", localAddr)
}

func TestMultipleForwards(t *testing.T) {
	fm := server.NewForwardManager()
	sendFunc := func(msg string) {}

	// Start multiple forwards
	ids := []string{"fwd1", "fwd2", "fwd3"}
	for _, id := range ids {
		err := fm.StartForward(id, "0", fmt.Sprintf("target-%s:80", id), sendFunc)
		if err != nil {
			t.Fatalf("Failed to start forward %s: %v", id, err)
		}
	}

	// Verify all are running
	forwards := fm.ListForwards()
	if len(forwards) != 3 {
		t.Errorf("Expected 3 forwards, got %d", len(forwards))
	}

	// Stop all
	fm.StopAll()

	// Verify all stopped
	forwards = fm.ListForwards()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after StopAll, got %d", len(forwards))
	}
}
