package main

import (
	"bytes"
	"testing"
	"time"

	"golang-https-rev/pkg/compression"
	"golang-https-rev/pkg/protocol"
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
	if err := runListener([]string{}); err == nil {
		t.Fatal("expected error for missing args")
	}
	if err := runListener([]string{"8443"}); err == nil {
		t.Fatal("expected error for too few args")
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

func TestUseClientValid(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234", "10.0.0.5:5678"}}
	result := useClient(ml, []string{"use", "1"})
	if result != "192.168.1.2:1234" {
		t.Fatalf("expected first client, got %s", result)
	}
}

func TestUseClientInvalidID(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	result := useClient(ml, []string{"use", "5"})
	if result != "" {
		t.Fatalf("expected empty for out-of-range ID, got %s", result)
	}
}

func TestUseClientNonNumericID(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	result := useClient(ml, []string{"use", "abc"})
	if result != "" {
		t.Fatalf("expected empty for non-numeric ID, got %s", result)
	}
}

func TestUseClientMissingArg(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	result := useClient(ml, []string{"use"})
	if result != "" {
		t.Fatalf("expected empty when missing arg, got %s", result)
	}
}

type mockListener struct {
	clients       []string
	sentCommands  []string
	responses     []string
	responseIdx   int
	sendErr       error
	getErr        error
}

func (m *mockListener) GetClients() []string {
	return m.clients
}

func (m *mockListener) SendCommand(client, cmd string) error {
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

func TestSendShellCommandSuccess(t *testing.T) {
	ml := &mockListener{responses: []string{"output" + protocol.EndOfOutputMarker}}
	if !sendShellCommand(ml, "192.168.1.2:1234", "ls") {
		t.Fatal("expected success")
	}
	if len(ml.sentCommands) != 1 || ml.sentCommands[0] != "ls" {
		t.Fatalf("unexpected commands: %v", ml.sentCommands)
	}
}

func TestSendShellCommandSendError(t *testing.T) {
	ml := &mockListener{sendErr: bytes.ErrTooLarge}
	if sendShellCommand(ml, "192.168.1.2:1234", "ls") {
		t.Fatal("expected failure when send fails")
	}
}

func TestSendShellCommandGetError(t *testing.T) {
	ml := &mockListener{getErr: bytes.ErrTooLarge}
	if sendShellCommand(ml, "192.168.1.2:1234", "ls") {
		t.Fatal("expected failure when get response fails")
	}
}
