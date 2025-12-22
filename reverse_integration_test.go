package main

import (
    "crypto/tls"
    "testing"
    "time"

    "golang-https-rev/pkg/client"
    "golang-https-rev/pkg/certs"
    "golang-https-rev/pkg/protocol"
    "golang-https-rev/pkg/server"
)

// TestClientConnect tests successful connection to listener
func TestClientConnect(t *testing.T) {
    listener := createServerForTest(t)
    netListener, err := listener.Start()
    if err != nil {
        t.Fatalf("Failed to start listener: %v", err)
    }
    defer netListener.Close()

    addr := netListener.Addr().String()

    c := client.NewReverseClient(addr, "", "")
    if c == nil {
        t.Fatal("NewReverseClient returned nil")
    }
    if c.IsConnected() {
        t.Fatal("Client should not be connected after creation")
    }

    err = c.Connect()
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }
    defer c.Close()

    if !c.IsConnected() {
        t.Fatal("Client should be connected after Connect()")
    }

    t.Log("✓ Client connect test passed")
}

// TestClientConnectFailure tests connection to non-existent server
func TestClientConnectFailure(t *testing.T) {
    c := client.NewReverseClient("127.0.0.1:9999", "", "")
    err := c.Connect()
    if err == nil {
        t.Fatal("Expected connection error to non-existent server")
    }

    if c.IsConnected() {
        t.Fatal("Client should not be connected after failed connection attempt")
    }

    t.Log("✓ Client connect failure test passed")
}

// TestClientClose tests closing the connection
func TestClientClose(t *testing.T) {
    listener := createServerForTest(t)
    netListener, err := listener.Start()
    if err != nil {
        t.Fatalf("Failed to start listener: %v", err)
    }
    defer netListener.Close()

    addr := netListener.Addr().String()

    c := client.NewReverseClient(addr, "", "")
    err = c.Connect()
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }

    if !c.IsConnected() {
        t.Fatal("Client should be connected")
    }

    err = c.Close()
    if err != nil {
        t.Fatalf("Close() failed: %v", err)
    }

    if c.IsConnected() {
        t.Fatal("Client should not be connected after Close()")
    }

    t.Log("✓ Client close test passed")
}

// TestClientCommandReception
func TestClientCommandReception(t *testing.T) {
    listener := createServerForTest(t)
    netListener, err := listener.Start()
    if err != nil {
        t.Fatalf("Failed to start listener: %v", err)
    }
    defer netListener.Close()

    addr := netListener.Addr().String()

    c := client.NewReverseClient(addr, "", "")
    err = c.Connect()
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }
    defer c.Close()

    time.Sleep(100 * time.Millisecond)
    clients := listener.GetClients()
    if len(clients) != 1 {
        t.Fatalf("Expected 1 connected client, got %d", len(clients))
    }
}

// TestClientUploadFlow (setup only)
func TestClientUploadFlow(t *testing.T) {
    listener := createServerForTest(t)
    netListener, err := listener.Start()
    if err != nil {
        t.Fatalf("Failed to start listener: %v", err)
    }
    defer netListener.Close()

    addr := netListener.Addr().String()

    c := client.NewReverseClient(addr, "", "")
    err = c.Connect()
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }
    defer c.Close()

    time.Sleep(100 * time.Millisecond)
    clients := listener.GetClients()
    if len(clients) != 1 {
        t.Fatalf("Expected 1 connected client, got %d", len(clients))
    }

    cmd := "test_command"
    err = listener.SendCommand(clients[0], cmd)
    if err != nil {
        t.Fatalf("Failed to send command: %v", err)
    }
}

// TestClientExitCommand
func TestClientExitCommand(t *testing.T) {
    listener := createServerForTest(t)
    netListener, err := listener.Start()
    if err != nil {
        t.Fatalf("Failed to start listener: %v", err)
    }
    defer netListener.Close()

    addr := netListener.Addr().String()

    c := client.NewReverseClient(addr, "", "")
    err = c.Connect()
    if err != nil {
        t.Fatalf("Failed to connect: %v", err)
    }
    defer c.Close()

    time.Sleep(100 * time.Millisecond)
    clients := listener.GetClients()
    clientAddr := clients[0]

    err = listener.SendCommand(clientAddr, protocol.CmdExit)
    if err != nil {
        t.Fatalf("Failed to send EXIT: %v", err)
    }

    time.Sleep(100 * time.Millisecond)
    clients = listener.GetClients()
    if len(clients) != 0 {
        t.Fatalf("Expected 0 clients after EXIT, got %d", len(clients))
    }
}

// Helper functions
func createServerForTest(t *testing.T) *server.Listener {
    cert, _, err := certs.GenerateSelfSignedCert()
    if err != nil {
        t.Fatalf("Failed to generate certificate: %v", err)
    }

    tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
    return server.NewListener("0", "127.0.0.1", tlsConfig, "")
}
