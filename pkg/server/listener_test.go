package server

import (
	"crypto/tls"
	"testing"
	"time"

	"golang-https-rev/pkg/certs"
)

// TestListenerCreation tests creating a new listener
func TestListenerCreation(t *testing.T) {
	cert, _, err := certs.GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("19000", "127.0.0.1", tlsConfig)
	if listener == nil {
		t.Fatal("Failed to create listener")
	}

	if listener.port != "19000" {
		t.Fatalf("Expected port 19000, got %s", listener.port)
	}

	t.Log("✓ Listener created successfully")
}

// TestGetClients tests getting client list
func TestGetClients(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("19001", "127.0.0.1", tlsConfig)
	clients := listener.GetClients()

	if len(clients) != 0 {
		t.Fatalf("Expected 0 clients, got %d", len(clients))
	}

	t.Log("✓ GetClients works for empty list")
}

// TestSendCommand tests sending a command
func TestSendCommand(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("19002", "127.0.0.1", tlsConfig)

	// Try sending to non-existent client
	err := listener.SendCommand("127.0.0.1:9999", "test")
	if err == nil {
		t.Fatal("Expected error for non-existent client")
	}

	t.Log("✓ SendCommand properly rejects non-existent clients")
}

// TestGetResponse tests getting a response
func TestGetResponse(t *testing.T) {
	cert, _, _ := certs.GenerateSelfSignedCert()
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener := NewListener("19003", "127.0.0.1", tlsConfig)

	// Try getting response from non-existent client
	_, err := listener.GetResponse("127.0.0.1:9999", 1*time.Second)
	if err == nil {
		t.Fatal("Expected error for non-existent client")
	}

	t.Log("✓ GetResponse properly rejects non-existent clients")
}
