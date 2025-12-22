package certs

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func TestGenerateSelfSignedCertInvalidKeySize(t *testing.T) {
	oldRandReader := rand.Reader
	defer func() { rand.Reader = oldRandReader }()

	// Use a small buffer to simulate key generation failure
	rand.Reader = &limitedReader{remaining: 10}
	
	_, _, err := GenerateSelfSignedCert()
	if err == nil {
		t.Fatal("expected error with insufficient random data")
	}
}

type limitedReader struct {
	remaining int
}

func (l *limitedReader) Read(p []byte) (n int, err error) {
	if l.remaining <= 0 {
		return 0, rsa.ErrVerification
	}
	if len(p) > l.remaining {
		n = l.remaining
		l.remaining = 0
		return n, rsa.ErrVerification
	}
	l.remaining -= len(p)
	return len(p), nil
}

func TestCertificateValidityPeriod(t *testing.T) {
	cert, _, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatal(err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("certificate should contain at least one cert")
	}

	// Parse the certificate to check validity period
	// Note: cert.Leaf should already be populated by GenerateSelfSignedCert
	if cert.Leaf == nil {
		t.Fatal("Leaf certificate should be parsed")
	}

	now := time.Now()
	if cert.Leaf.NotBefore.After(now) {
		t.Fatal("certificate should be valid from current time")
	}

	expectedExpiry := now.AddDate(1, 0, 0)
	timeDiff := cert.Leaf.NotAfter.Sub(expectedExpiry)
	if timeDiff < -time.Hour || timeDiff > time.Hour {
		t.Fatalf("certificate should expire in ~1 year, got %v", cert.Leaf.NotAfter)
	}
}

func TestCertificateOrganization(t *testing.T) {
	cert, _, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatal(err)
	}

	if cert.Leaf == nil {
		t.Fatal("Leaf certificate should be parsed")
	}

	if len(cert.Leaf.Subject.Organization) == 0 {
		t.Fatal("certificate should have organization set")
	}

	expectedOrg := "Reverse Shell Listener"
	if cert.Leaf.Subject.Organization[0] != expectedOrg {
		t.Fatalf("expected org %s, got %s", expectedOrg, cert.Leaf.Subject.Organization[0])
	}
}
