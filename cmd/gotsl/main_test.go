package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/config"
)

// TestCompressDecompressRoundTrip verifies that data can be compressed to hex and decompressed back identically
func TestCompressDecompressRoundTrip(t *testing.T) {
	testData := []byte("Hello, this is test data for compression! " + string(bytes.Repeat([]byte("x"), 1000)))

	// Compress
	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}

	// Verify compressed is not empty
	if compressed == "" {
		t.Fatal("compressed hex should not be empty")
	}

	// Decompress
	decompressed, err := compression.DecompressHex(compressed)
	if err != nil {
		t.Fatalf("DecompressHex failed: %v", err)
	}

	// Verify round-trip
	if !bytes.Equal(decompressed, testData) {
		t.Fatalf("decompressed data does not match original: got %d bytes, expected %d bytes", len(decompressed), len(testData))
	}
}

// TestCompressEmptyData handles edge case of compressing empty data
func TestCompressEmptyData(t *testing.T) {
	testData := []byte{}

	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed on empty data: %v", err)
	}

	decompressed, err := compression.DecompressHex(compressed)
	if err != nil {
		t.Fatalf("DecompressHex failed on empty data: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Fatal("decompressed empty data should match original")
	}
}

// TestCompressLargeData ensures compression works with large payloads
func TestCompressLargeData(t *testing.T) {
	// Create 5MB of repetitive data
	testData := bytes.Repeat([]byte("large data payload "), 262144)

	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed on large data: %v", err)
	}

	decompressed, err := compression.DecompressHex(compressed)
	if err != nil {
		t.Fatalf("DecompressHex failed on large data: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Fatalf("large data round-trip failed: got %d bytes, expected %d bytes", len(decompressed), len(testData))
	}
}

// TestDecompressInvalidHex verifies that invalid hex input is handled gracefully
func TestDecompressInvalidHex(t *testing.T) {
	_, err := compression.DecompressHex("invalid!@#$%hex")
	if err == nil {
		t.Fatal("DecompressHex should return error for invalid hex input")
	}
}

// TestDecompressCorruptedGzip verifies that corrupted gzip data is detected
func TestDecompressCorruptedGzip(t *testing.T) {
	// Create valid hex that doesn't contain valid gzip data
	invalidGzip := "deadbeef"

	_, err := compression.DecompressHex(invalidGzip)
	if err == nil {
		t.Fatal("DecompressHex should return error for corrupted gzip data")
	}
}

func TestRunListenerArgValidation(t *testing.T) {
	// Valid call with full args should work (would fail on cert generation, but passes arg validation)
	// We don't test actual startup since it requires real certificates
	// Instead test that empty/invalid config is caught
	
	// Empty port and network interface should use defaults and validate
	_, err := config.LoadServerConfig("", "", false)
	if err != nil {
		t.Fatalf("LoadServerConfig with defaults failed: %v", err)
	}
	
	// Invalid port should be caught
	_, err = config.LoadServerConfig("not-a-port", "", false)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestListClientsEmpty(t *testing.T) {
	ml := &mockListener{clients: []string{}}
	listClients(ml)
}

func TestListClientsMultiple(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234", "10.0.0.5:5678"}}
	listClients(ml)
}

func TestGetClientByIDValid(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234", "10.0.0.5:5678"}}
	result := getClientByID(ml, "1")
	if result != "192.168.1.2:1234" {
		t.Fatalf("expected first client, got %s", result)
	}
}

func TestGetClientByIDInvalidID(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	result := getClientByID(ml, "5")
	if result != "" {
		t.Fatalf("expected empty for out-of-range ID, got %s", result)
	}
}

func TestGetClientByIDNonNumericID(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	result := getClientByID(ml, "abc")
	if result != "" {
		t.Fatalf("expected empty for non-numeric ID, got %s", result)
	}
}

type mockListener struct {
	clients      []string
	sentCommands []string
	responses    []string
	responseIdx  int
	sendErr      error
	sendErrs     []error // Multiple send errors for different calls
	getErr       error
}

func (m *mockListener) GetClients() []string {
	return m.clients
}

func (m *mockListener) SendCommand(client, cmd string) error {
	// Use sendErrs if available for per-call errors
	if len(m.sendErrs) > 0 {
		callNum := len(m.sentCommands)
		if callNum < len(m.sendErrs) && m.sendErrs[callNum] != nil {
			return m.sendErrs[callNum]
		}
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentCommands = append(m.sentCommands, cmd)
	return nil
}

func (m *mockListener) GetResponse(client string, timeout time.Duration) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	if m.responseIdx < len(m.responses) {
		resp := m.responses[m.responseIdx]
		m.responseIdx++
		return resp, nil
	}
	return "", nil
}

func TestPrintHelp(t *testing.T) {
	// Just call it to increase coverage - it only prints output
	printHelp()
}

func TestPrintHeader(t *testing.T) {
	// Call it to increase coverage - it only prints output
	printHeader()
}

func TestHandleUploadGlobalBadFile(t *testing.T) {
	ml := &mockListener{}
	result := handleUploadGlobal(ml, "192.168.1.2:1234", "/nonexistent/file.txt", "/remote/path.txt")
	// The function returns true (continue) on local errors, false (disconnect) on network errors
	if !result {
		t.Fatal("expected true for nonexistent file (continue connection)")
	}
}

func TestHandleDownloadGlobalGetResponseError(t *testing.T) {
	ml := &mockListener{getErr: bytes.ErrTooLarge}
	tmpfile := t.TempDir() + "/out.txt"
	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "/remote/file.txt", tmpfile)
	if result {
		t.Fatal("expected false when get response fails")
	}
}
