package certs

import (
	"testing"
	"time"
)

// TestGenerateSelfSignedCert tests certificate generation
func TestGenerateSelfSignedCert(t *testing.T) {
	cert, fingerprint, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("Certificate has no data")
	}

	// Fingerprint should be 64 hex characters (SHA256)
	if len(fingerprint) != 64 {
		t.Errorf("fingerprint should be 64 characters, got %d", len(fingerprint))
	}

	t.Log("✓ Self-signed certificate generated successfully")
}

// TestCertificateValidity tests certificate has required properties
func TestCertificateValidity(t *testing.T) {
	cert, _, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("Certificate data is empty")
	}

	if cert.PrivateKey == nil {
		t.Fatal("Private key is nil")
	}

	t.Log("✓ Certificate is valid and contains required data")
}

// TestMultipleCertificates tests that multiple certificates can be generated
func TestMultipleCertificates(t *testing.T) {
	cert1, _, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("failed to generate first certificate: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	cert2, _, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate second certificate: %v", err)
	}

	// Certificates should be different (different serial numbers)
	if string(cert1.Certificate[0]) == string(cert2.Certificate[0]) {
		t.Fatal("Two generated certificates are identical")
	}

	t.Log("✓ Multiple unique certificates can be generated")
}
