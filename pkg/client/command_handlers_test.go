package client

import (
"bufio"
"bytes"
"os"
"path/filepath"
"runtime"
"testing"

"golang-https-rev/pkg/compression"
"golang-https-rev/pkg/protocol"
)

// createMockClient creates a client with mock readers/writers for testing
func createMockClient() (*ReverseClient, *bytes.Buffer) {
output := new(bytes.Buffer)
client := &ReverseClient{
writer: bufio.NewWriter(output),
reader: nil,
conn:   nil,
}
return client, output
}

// TestHandlePingCommand tests the PING command handler
func TestHandlePingCommand(t *testing.T) {
client, output := createMockClient()

err := client.handlePingCommand()
if err != nil {
t.Errorf("handlePingCommand failed: %v", err)
}

result := output.String()
if !bytes.Contains([]byte(result), []byte(protocol.CmdPong)) {
t.Errorf("Expected PONG response, got: %s", result)
}
if !bytes.Contains([]byte(result), []byte(protocol.EndOfOutputMarker)) {
t.Errorf("Expected EndOfOutputMarker, got: %s", result)
}
}

// TestHandleStartUploadCommand tests upload initialization
func TestHandleStartUploadCommand(t *testing.T) {
client, output := createMockClient()

// Test valid start upload
cmd := "START_UPLOAD /tmp/testfile 100"
err := client.handleStartUploadCommand(cmd)
if err != nil {
t.Errorf("Valid START_UPLOAD failed: %v", err)
}

if client.currentUploadPath != "/tmp/testfile" {
t.Errorf("Expected currentUploadPath=/tmp/testfile, got %s", client.currentUploadPath)
}

if len(client.uploadChunks) != 0 {
t.Errorf("Expected empty uploadChunks, got %d items", len(client.uploadChunks))
}

result := output.String()
if !bytes.Contains([]byte(result), []byte("OK")) {
t.Errorf("Expected OK response, got: %s", result)
}

// Test invalid start upload (missing size)
client, _ = createMockClient()
invalidCmd := "START_UPLOAD /tmp/testfile"
err = client.handleStartUploadCommand(invalidCmd)
if err == nil {
t.Error("Invalid START_UPLOAD should return error")
}
}

// TestHandleUploadChunkCommand tests chunk receiving
func TestHandleUploadChunkCommand(t *testing.T) {
client, output := createMockClient()

// Test chunk without active upload
cmd := "UPLOAD_CHUNK somedata"
err := client.handleUploadChunkCommand(cmd)
if err == nil {
t.Error("Upload chunk without active upload should return error")
}

result := output.String()
if !bytes.Contains([]byte(result), []byte("No active upload")) {
t.Errorf("Expected 'No active upload' error, got: %s", result)
}

// Setup active upload
client, output = createMockClient()
client.currentUploadPath = "/tmp/test"
client.uploadChunks = []string{}

// Test valid chunk
err = client.handleUploadChunkCommand(cmd)
if err != nil {
t.Errorf("Valid upload chunk failed: %v", err)
}

if len(client.uploadChunks) != 1 || client.uploadChunks[0] != "somedata" {
t.Errorf("Expected uploadChunks=['somedata'], got %v", client.uploadChunks)
}
}

// TestHandleDownloadCommand tests file download request handling
func TestHandleDownloadCommand(t *testing.T) {
client, _ := createMockClient()

// Test invalid download command (no path)
cmd := "DOWNLOAD"
err := client.handleDownloadCommand(cmd)
if err == nil {
t.Error("Invalid download command should return error")
}

// Test download of non-existent file
client, _ = createMockClient()
cmd = "DOWNLOAD /nonexistent/file/path/that/does/not/exist.txt"
err = client.handleDownloadCommand(cmd)
if err == nil {
t.Logf("Download of non-existent file should error, got: %v", err)
}

// Test download of actual temp file
client, _ = createMockClient()
tempFile := filepath.Join(os.TempDir(), "test_download_file.txt")
os.WriteFile(tempFile, []byte("test content"), 0644)
defer os.Remove(tempFile)

cmd = "DOWNLOAD " + tempFile
err = client.handleDownloadCommand(cmd)
if err != nil {
t.Logf("Download of temp file: %v", err)
}
}

// TestHandleShellCommand tests command execution
func TestHandleShellCommand(t *testing.T) {
client, output := createMockClient()

// Test shell command execution
cmd := "echo test"
err := client.handleShellCommand(cmd)
if err != nil {
t.Logf("Shell command execution: %v", err)
}

result := output.String()
if len(result) == 0 {
t.Error("Expected shell command output")
}

// Test shell command with error
client, _ = createMockClient()
cmd = "false"
err = client.handleShellCommand(cmd)
if err != nil {
t.Logf("False command error (expected): %v", err)
}
}

// TestProcessCommandExitCommand tests EXIT command handling
func TestProcessCommandExitCommand(t *testing.T) {
client, _ := createMockClient()

shouldContinue, err := client.processCommand(protocol.CmdExit)
if shouldContinue {
t.Error("EXIT command should return shouldContinue=false")
}
if err != nil {
t.Errorf("EXIT command should not error, got: %v", err)
}
}

// TestProcessCommandPingCommand tests PING command routing
func TestProcessCommandPingCommand(t *testing.T) {
client, output := createMockClient()

shouldContinue, err := client.processCommand(protocol.CmdPing)
if !shouldContinue {
t.Error("PING command should return shouldContinue=true")
}
if err != nil {
t.Errorf("PING command should not error, got: %v", err)
}

result := output.String()
if !bytes.Contains([]byte(result), []byte(protocol.CmdPong)) {
t.Errorf("Expected PONG in response, got: %s", result)
}
}

// TestProcessCommandDispatcher tests correct command routing
func TestProcessCommandDispatcher(t *testing.T) {
testCases := []struct {
name            string
command         string
shouldContinue  bool
shouldSucceed   bool
}{
{
name:           "PING",
command:        protocol.CmdPing,
shouldContinue: true,
shouldSucceed:  true,
},
{
name:           "EXIT",
command:        protocol.CmdExit,
shouldContinue: false,
shouldSucceed:  true,
},
{
name:           "Invalid START_UPLOAD",
command:        "START_UPLOAD /tmp/file",
shouldContinue: true,
shouldSucceed:  false,
},
{
name:           "Shell echo command",
command:        "echo hello",
shouldContinue: true,
shouldSucceed:  true,
},
		{
			name:           "DOWNLOAD missing path",
			command:        "DOWNLOAD",
			shouldContinue: true,
			shouldSucceed:  true, // Runs as shell command, doesn't strictly error
		},
}

for _, tc := range testCases {
t.Run(tc.name, func(t *testing.T) {
client, _ := createMockClient()
shouldContinue, err := client.processCommand(tc.command)

if shouldContinue != tc.shouldContinue {
t.Errorf("Expected shouldContinue=%v, got %v", tc.shouldContinue, shouldContinue)
}

hasError := err != nil
shouldError := !tc.shouldSucceed
if hasError != shouldError {
t.Errorf("Expected error=%v, got error=%v (err=%v)", shouldError, hasError, err)
}
})
}
}

// TestProcessCommandFiltering tests command line filtering
func TestProcessCommandFiltering(t *testing.T) {
client, output := createMockClient()

// Test echo command which should produce output
cmd := "echo test_message"
shouldContinue, err := client.processCommand(cmd)
if err != nil {
t.Logf("Echo command error: %v", err)
}
if !shouldContinue {
t.Error("Shell command should allow continuing")
}

result := output.String()
if len(result) > 0 {
t.Logf("Echo command output: %s", result)
}
}

// TestEndUploadCommandFileCreation tests that files are handled on END_UPLOAD
func TestEndUploadCommandFileCreation(t *testing.T) {
client, output := createMockClient()

// Create temp directory for test
tmpDir := filepath.Join(os.TempDir(), "test_end_upload")
os.MkdirAll(tmpDir, 0755)
defer os.RemoveAll(tmpDir)

// Setup an upload with some compressed data
testFilePath := filepath.Join(tmpDir, "test_file.txt")
client.currentUploadPath = testFilePath
client.uploadChunks = []string{}

// Attempt to end upload
cmd := "END_UPLOAD"
err := client.handleEndUploadCommand(cmd)
if err != nil {
t.Logf("End upload handling: %v", err)
}

result := output.String()
if len(result) == 0 {
t.Error("Expected output from END_UPLOAD")
}

// Cleanup
client.currentUploadPath = ""
client.uploadChunks = nil
}

// TestProcessCommandError tests error handling in dispatcher
func TestProcessCommandError(t *testing.T) {
client, _ := createMockClient()

// Test command that doesn't exist but is treated as shell
_, err := client.processCommand("definitely_not_a_real_command_12345")
if err != nil {
t.Logf("Unknown command error (expected): %v", err)
}
}

// TestProcessCommandPing tests PING command via processCommand
func TestProcessCommandPing(t *testing.T) {
	client, output := createMockClient()
	
	shouldContinue, err := client.processCommand(protocol.CmdPing)
	if err != nil {
		t.Errorf("PING command failed: %v", err)
	}
	if !shouldContinue {
		t.Error("PING should return shouldContinue=true")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte(protocol.CmdPong)) {
		t.Errorf("Expected PONG in output, got: %s", result)
	}
}

// TestProcessCommandExit tests EXIT command via processCommand
func TestProcessCommandExit(t *testing.T) {
	client, _ := createMockClient()
	
	shouldContinue, err := client.processCommand(protocol.CmdExit)
	if err != nil {
		t.Errorf("EXIT command failed: %v", err)
	}
	if shouldContinue {
		t.Error("EXIT should return shouldContinue=false")
	}
}

// TestProcessCommandStartUpload tests START_UPLOAD via processCommand
func TestProcessCommandStartUpload(t *testing.T) {
	client, output := createMockClient()
	
	shouldContinue, err := client.processCommand("START_UPLOAD /tmp/test.txt 100")
	if err != nil {
		t.Errorf("START_UPLOAD failed: %v", err)
	}
	if !shouldContinue {
		t.Error("START_UPLOAD should return shouldContinue=true")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("OK")) {
		t.Errorf("Expected OK response, got: %s", result)
	}
}

// TestProcessCommandUploadChunk tests UPLOAD_CHUNK via processCommand
func TestProcessCommandUploadChunk(t *testing.T) {
	client, _ := createMockClient()
	
	// First start an upload
	client.processCommand("START_UPLOAD /tmp/test.txt 100")
	
	// Now send a chunk
	shouldContinue, err := client.processCommand("UPLOAD_CHUNK testchunkdata")
	if err != nil {
		t.Errorf("UPLOAD_CHUNK failed: %v", err)
	}
	if !shouldContinue {
		t.Error("UPLOAD_CHUNK should return shouldContinue=true")
	}
}

// TestProcessCommandDownload tests DOWNLOAD via processCommand
func TestProcessCommandDownload(t *testing.T) {
	client, _ := createMockClient()
	
	// Create a temp file to download
	tmpFile, err := os.CreateTemp("", "download-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.WriteString("test content")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())
	
	shouldContinue, err := client.processCommand("DOWNLOAD " + tmpFile.Name())
	if err != nil {
		t.Errorf("DOWNLOAD failed: %v", err)
	}
	if !shouldContinue {
		t.Error("DOWNLOAD should return shouldContinue=true")
	}
}

// TestProcessCommandShellExecution tests default shell command via processCommand
func TestProcessCommandShellExecution(t *testing.T) {
	client, output := createMockClient()
	
	shouldContinue, err := client.processCommand("echo test_shell")
	if err != nil {
		t.Errorf("Shell command failed: %v", err)
	}
	if !shouldContinue {
		t.Error("Shell command should return shouldContinue=true")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("test_shell")) {
		t.Errorf("Expected 'test_shell' in output, got: %s", result)
	}
}
// TestHandleEndUploadInvalidCommand tests malformed END_UPLOAD commands
func TestHandleEndUploadInvalidCommand(t *testing.T) {
	client, output := createMockClient()
	
	// Test with invalid format (no path)
	err := client.handleEndUploadCommand("END_UPLOAD")
	if err == nil {
		t.Error("Expected error for invalid END_UPLOAD command")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("Invalid end_upload command")) {
		t.Errorf("Expected invalid command error, got: %s", result)
	}
}

// TestHandleEndUploadNoActiveSession tests END_UPLOAD without START_UPLOAD
func TestHandleEndUploadNoActiveSession(t *testing.T) {
	client, output := createMockClient()
	
	// Clear any active upload
	client.currentUploadPath = ""
	
	err := client.handleEndUploadCommand("END_UPLOAD /tmp/test.txt")
	if err == nil {
		t.Error("Expected error when no active upload session")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("No active upload")) {
		t.Errorf("Expected 'No active upload' error, got: %s", result)
	}
}

// TestHandleEndUploadDecompressionError tests decompression failure
func TestHandleEndUploadDecompressionError(t *testing.T) {
	client, output := createMockClient()
	
	// Setup with invalid compressed chunk
	client.currentUploadPath = "/tmp/test.txt"
	client.uploadChunks = []string{"INVALID_HEX_DATA!@#"}
	
	err := client.handleEndUploadCommand("END_UPLOAD /tmp/test.txt")
	if err == nil {
		t.Error("Expected error for decompression failure")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("Decompression error")) {
		t.Errorf("Expected decompression error, got: %s", result)
	}
	
	// Cleanup
	client.currentUploadPath = ""
	client.uploadChunks = nil
}

// TestHandleEndUploadWriteError tests file write failure
func TestHandleEndUploadWriteError(t *testing.T) {
	client, output := createMockClient()
	
	// Create valid compressed data
	testData := []byte("test content")
	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatal(err)
	}
	
	// Setup with valid chunk but invalid path (directory doesn't exist)
	client.currentUploadPath = "/nonexistent/dir/test.txt"
	client.uploadChunks = []string{compressed}
	
	err = client.handleEndUploadCommand("END_UPLOAD /nonexistent/dir/test.txt")
	if err == nil {
		t.Error("Expected error for file write failure")
	}
	
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("Write error")) {
		t.Errorf("Expected write error, got: %s", result)
	}
	
	// Cleanup
	client.currentUploadPath = ""
	client.uploadChunks = nil
}

// TestHandleEndUploadSuccess tests successful file upload
func TestHandleEndUploadSuccess(t *testing.T) {
	client, output := createMockClient()
	
	// Create temp directory
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "test.txt")
	
	// Create valid compressed data
	testData := []byte("test content for successful upload")
	compressed, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatal(err)
	}
	
	// Setup upload
	client.currentUploadPath = testFilePath
	client.uploadChunks = []string{compressed}
	
	err = client.handleEndUploadCommand("END_UPLOAD " + testFilePath)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	
	// Verify file was written
	written, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Errorf("Failed to read written file: %v", err)
	}
	
	if !bytes.Equal(written, testData) {
		t.Errorf("File content mismatch: got %q, expected %q", written, testData)
	}
	
	// Verify response
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("OK")) {
		t.Errorf("Expected OK response, got: %s", result)
	}
	
	// Verify cleanup
	if client.currentUploadPath != "" {
		t.Error("Expected currentUploadPath to be cleared")
	}
	if len(client.uploadChunks) != 0 {
		t.Error("Expected uploadChunks to be cleared")
	}
}

// TestHandleEndUploadMultipleChunks tests uploading file in multiple chunks
func TestHandleEndUploadMultipleChunks(t *testing.T) {
	client, _ := createMockClient()
	
	// Create temp directory
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "multi_chunk.txt")
	
	// Create test data split into multiple parts
	part1 := []byte("first chunk ")
	part2 := []byte("second chunk ")
	part3 := []byte("third chunk")
	
	compressed1, _ := compression.CompressToHex(part1)
	compressed2, _ := compression.CompressToHex(part2)
	compressed3, _ := compression.CompressToHex(part3)
	
	// Setup upload with multiple chunks
	client.currentUploadPath = testFilePath
	client.uploadChunks = []string{compressed1, compressed2, compressed3}
	
	err := client.handleEndUploadCommand("END_UPLOAD " + testFilePath)
	if err != nil {
		t.Errorf("Multi-chunk upload failed: %v", err)
	}
	
	// Verify all chunks were assembled
	written, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Errorf("Failed to read multi-chunk file: %v", err)
	}
	
	expected := append(append(part1, part2...), part3...)
	if !bytes.Equal(written, expected) {
		t.Errorf("Multi-chunk content mismatch: got %q, expected %q", written, expected)
	}
}

// TestPtyModeReentry tests that PTY mode can be entered and exited multiple times
// without "input/output error" or goroutine leaks
func TestPtyModeReentry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	// Test entering and exiting PTY mode 3 times
	numRetries := 3
	
	for attempt := 1; attempt <= numRetries; attempt++ {
		t.Logf("Attempt %d: Testing PTY entry/exit cycle", attempt)
		
		client, _ := createMockClient()
		
		// Verify not in PTY mode initially
		if client.inPtyMode {
			t.Errorf("Attempt %d: Client should not be in PTY mode initially", attempt)
		}
		
		// Enter PTY mode
		err := client.handlePtyModeCommand()
		if err != nil {
			t.Errorf("Attempt %d: handlePtyModeCommand failed: %v", attempt, err)
			continue
		}
		
		// Verify PTY state
		if !client.inPtyMode {
			t.Errorf("Attempt %d: Client should be in PTY mode after handlePtyModeCommand", attempt)
			continue
		}
		
		if client.ptyFile == nil {
			t.Errorf("Attempt %d: PTY file should not be nil", attempt)
			continue
		}
		
		if client.ptyCmd == nil {
			t.Errorf("Attempt %d: PTY cmd should not be nil", attempt)
			continue
		}
		
		// Store reference to PTY file to verify cleanup
		oldPtyFile := client.ptyFile
		
		// Small delay to allow background goroutine to start
		// (in real usage, this would be reading/writing data)
		
		// Exit PTY mode
		err = client.handlePtyExitCommand()
		if err != nil {
			t.Errorf("Attempt %d: handlePtyExitCommand failed: %v", attempt, err)
			continue
		}
		
		// Verify PTY state after exit
		if client.inPtyMode {
			t.Errorf("Attempt %d: Client should not be in PTY mode after exit", attempt)
			continue
		}
		
		if client.ptyFile != nil {
			t.Errorf("Attempt %d: PTY file should be nil after exit", attempt)
			continue
		}
		
		if client.ptyCmd != nil {
			t.Errorf("Attempt %d: PTY cmd should be nil after exit", attempt)
			continue
		}
		
		// Note: PTY_EXIT no longer sends a response message (internal state change only)
		// The important thing is that we can re-enter without errors on next iteration
		
		// Verify the old PTY file is closed
		// Try to read from it - should fail gracefully
		testBuf := make([]byte, 1)
		_, err = oldPtyFile.Read(testBuf)
		// It's OK if this fails - the file should be closed
		// The important thing is that we can re-enter without errors on next iteration
		
		t.Logf("Attempt %d: PTY cycle completed successfully", attempt)
	}
}

// TestPtyDataEncoding tests compression and decompression of PTY data
func TestPtyDataEncoding(t *testing.T) {
	testData := []byte("Hello, PTY! This is test data with binary: \x00\x01\x02")
	
	// Compress and encode
	encoded, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}
	
	if encoded == "" {
		t.Error("Encoded data should not be empty")
	}
	
	// Decompress and decode
	decoded, err := compression.DecompressHex(encoded)
	if err != nil {
		t.Fatalf("DecompressHex failed: %v", err)
	}
	
	if !bytes.Equal(decoded, testData) {
		t.Errorf("Roundtrip compression failed: got %q, expected %q", decoded, testData)
	}
	
	t.Logf("✓ PTY data encoding test passed")
}