package client

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestForwardHandler_New(t *testing.T) {
	fh := NewForwardHandler(func(string) {})
	if fh == nil {
		t.Fatal("NewForwardHandler returned nil")
	}
	if fh.connections == nil {
		t.Fatal("connections map not initialized")
	}
}

func TestForwardHandler_HandleForwardStop(t *testing.T) {
	fh := NewForwardHandler(func(string) {})
	fh.HandleForwardStop("nonexistent", "1")
}

func TestForwardHandler_HandleForwardStart_InvalidAddress(t *testing.T) {
	sent := []string{}
	fh := NewForwardHandler(func(msg string) { sent = append(sent, msg) })

	err := fh.HandleForwardStart("fwd-1", "conn-1", "127.0.0.1")
	if err == nil {
		t.Error("expected error for address without port")
	}
	if len(sent) == 0 {
		t.Error("expected FORWARD_STOP to be sent on error")
	}
}

func TestForwardHandler_ReadFromTargetSendsCorrectConnID(t *testing.T) {
	msgCh := make(chan string, 1)
	fh := NewForwardHandler(func(msg string) { msgCh <- msg })

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go fh.readFromTarget("fwd-test", "conn-42", server)

	client.Write([]byte("x"))

	select {
	case msg := <-msgCh:
		if !strings.HasPrefix(msg, "FORWARD_DATA fwd-test conn-42") {
			t.Fatalf("unexpected message: %s", msg)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for FORWARD_DATA")
	}
}

func TestForwardHandler_HandleForwardData_WritesToConnection(t *testing.T) {
	fh := NewForwardHandler(func(string) {})

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	fh.mu.Lock()
	fh.connections["fwd-test"] = map[string]net.Conn{"conn-1": server}
	fh.mu.Unlock()

	encoded := "SGVsbG8gZnJvbSByZW1vdGU="

	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := client.Read(buf)
		readDone <- buf[:n]
	}()

	if err := fh.HandleForwardData("fwd-test", "conn-1", encoded); err != nil {
		t.Fatalf("HandleForwardData failed: %v", err)
	}

	data := <-readDone
	if string(data) != "Hello from remote" {
		t.Fatalf("unexpected data: %q", string(data))
	}
}

func TestForwardHandler_CloseCleansAll(t *testing.T) {
	fh := NewForwardHandler(func(string) {})

	c1, p1 := net.Pipe()
	c2, p2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	fh.mu.Lock()
	fh.connections["fwd-1"] = map[string]net.Conn{"conn-1": p1}
	fh.connections["fwd-2"] = map[string]net.Conn{"conn-2": p2}
	fh.mu.Unlock()

	fh.Close()

	fh.mu.RLock()
	if len(fh.connections) != 0 {
		t.Fatalf("expected no connections after Close, got %d", len(fh.connections))
	}
	fh.mu.RUnlock()
}
