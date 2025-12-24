package main

import (
	"bytes"
	"net"
	"os"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/config"
	"github.com/frjcomp/gots/pkg/protocol"
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
	// Test with missing required flags - should fail in main() before reaching runListener
	// Since we validate in main(), we test the config validation instead
	
	// Invalid port should be caught
	_, err := config.LoadServerConfig("not-a-port", "0.0.0.0", false)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}

	// Valid config should succeed
	_, err = config.LoadServerConfig("9001", "127.0.0.1", false)
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
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

func (m *mockListener) Start() (net.Listener, error) {
	return nil, nil
}

func (m *mockListener) GetClientAddressesSorted() []string {
	return m.clients
}

func (m *mockListener) EnterPtyMode(clientAddr string) (chan []byte, error) {
	return make(chan []byte), nil
}

func (m *mockListener) ExitPtyMode(clientAddr string) error {
	return nil
}

func (m *mockListener) IsInPtyMode(clientAddr string) bool {
	return false
}

func (m *mockListener) GetPtyDataChan(clientAddr string) (chan []byte, bool) {
	return nil, false
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

// Additional tests for better coverage
func TestHandleUploadGlobalEmptyRemotePath(t *testing.T) {
	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		responses: []string{"OK"},
	}
	tmpfile := t.TempDir() + "/test.txt"
	
	// Create test file
	err := os.WriteFile(tmpfile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with empty remote path - should fail due to empty response
	result := handleUploadGlobal(ml, "192.168.1.2:1234", tmpfile, "")
	// Should fail (return false) because mock doesn't provide proper OK response
	if result {
		t.Error("expected false for upload without OK response")
	}
}

func TestHandleUploadGlobalSendCommandError(t *testing.T) {
	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		sendErr: bytes.ErrTooLarge,
	}
	tmpfile := t.TempDir() + "/test.txt"
	
	err := os.WriteFile(tmpfile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := handleUploadGlobal(ml, "192.168.1.2:1234", tmpfile, "/remote/path.txt")
	if result {
		t.Error("expected false when send command fails")
	}
}

func TestHandleUploadGlobalMultipleErrors(t *testing.T) {
	// Test multiple send errors in sequence
	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		sendErrs: []error{nil, nil, bytes.ErrTooLarge}, // Fail on 3rd send (END_UPLOAD)
	}
	tmpfile := t.TempDir() + "/test.txt"
	
	err := os.WriteFile(tmpfile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result := handleUploadGlobal(ml, "192.168.1.2:1234", tmpfile, "/remote/path.txt")
	if result {
		t.Error("expected false when END_UPLOAD command fails")
	}
}

func TestHandleDownloadGlobalInvalidRemotePath(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.2:1234"}}
	tmpfile := t.TempDir() + "/out.txt"
	
	// Test with empty remote path
	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "", tmpfile)
	// Should continue (true) as path validation doesn't fail the operation
	if !result {
		t.Error("expected true for download with empty remote path")
	}
}

func TestHandleDownloadGlobalSuccessfulDownload(t *testing.T) {
	testData := []byte("sample file content for download")
	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("Failed to compress test data: %v", err)
	}

	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		responses: []string{protocol.EndOfOutputMarker + protocol.DataPrefix + compressed + protocol.EndOfOutputMarker},
	}
	tmpfile := t.TempDir() + "/downloaded.txt"

	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "/remote/file.txt", tmpfile)
	if !result {
		t.Error("expected true for successful download")
	}

	// Verify file was created and contains correct data
	downloaded, err := os.ReadFile(tmpfile)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if !bytes.Equal(downloaded, testData) {
		t.Errorf("Downloaded content mismatch: got %d bytes, expected %d bytes", len(downloaded), len(testData))
	}
}

func TestHandleDownloadGlobalInvalidCompressedData(t *testing.T) {
	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		responses: []string{"invalid-hex-data!!!"},
	}
	tmpfile := t.TempDir() + "/out.txt"

	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "/remote/file.txt", tmpfile)
	// Should continue (true) on decompression error
	if !result {
		t.Error("expected true even with invalid compressed data")
	}
}

func TestHandleDownloadGlobalSendCommandFails(t *testing.T) {
	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		sendErr: bytes.ErrTooLarge,
	}
	tmpfile := t.TempDir() + "/out.txt"

	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "/remote/file.txt", tmpfile)
	if result {
		t.Error("expected false when send command fails")
	}
}

func TestHandleDownloadGlobalFileWriteError(t *testing.T) {
	testData := []byte("test content")
	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("Failed to compress test data: %v", err)
	}

	ml := &mockListener{
		clients: []string{"192.168.1.2:1234"},
		responses: []string{compressed},
	}

	// Try to write to invalid path (directory that doesn't exist and can't be created)
	result := handleDownloadGlobal(ml, "192.168.1.2:1234", "/remote/file.txt", "/nonexistent/dir/file.txt")
	// Should continue (true) even if write fails
	if !result {
		t.Error("expected true even when file write fails")
	}
}

func TestListClientsEmptyList(t *testing.T) {
	ml := &mockListener{clients: []string{}}
	listClients(ml)
	// Just verify it doesn't panic
}

func TestGetClientByIDNotFound(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.1:1234"}}
	result := getClientByID(ml, "999")
	if result != "" {
		t.Errorf("expected empty string for non-existent client ID, got %s", result)
	}
}

func TestGetClientByIDInvalidIndex(t *testing.T) {
	ml := &mockListener{clients: []string{"192.168.1.1:1234"}}
	result := getClientByID(ml, "abc")
	if result != "" {
		t.Errorf("expected empty string for invalid client ID, got %s", result)
	}
}
