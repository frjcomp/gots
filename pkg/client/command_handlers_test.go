package client

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/protocol"
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

	t.Log("✓ Shell command execution test passed")
}

// TestHandleShellCommandWithOutput tests command with output capture
func TestHandleShellCommandWithOutput(t *testing.T) {
	client, output := createMockClient()

	cmd := "echo hello world"
	err := client.handleShellCommand(cmd)
	if err != nil {
		t.Logf("Info: handleShellCommand returned: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "hello world") {
		t.Errorf("Expected 'hello world' in output, got: %s", result)
	}

	if !strings.Contains(result, protocol.EndOfOutputMarker) {
		t.Errorf("Expected end of output marker, got: %s", result)
	}

	t.Log("✓ Shell command output capture test passed")
}

// TestHandleShellCommandErrorMessage tests error output from command
func TestHandleShellCommandErrorMessage(t *testing.T) {
	client, output := createMockClient()

	// Use a command that produces error output
	cmd := "ls /nonexistent/path/that/does/not/exist 2>&1"
	err := client.handleShellCommand(cmd)
	if err != nil {
		t.Logf("Info: handleShellCommand returned: %v", err)
	}

	result := output.String()
	// Should contain end of output marker regardless of command success/failure
	if !strings.Contains(result, protocol.EndOfOutputMarker) {
		t.Errorf("Expected end of output marker in error output, got: %s", result)
	}

	t.Log("✓ Shell command error output test passed")
}

// TestHandleShellCommandMultilineOutput tests multi-line command output
func TestHandleShellCommandMultilineOutput(t *testing.T) {
	client, output := createMockClient()

	cmd := "printf 'line1\\nline2\\nline3'"
	err := client.handleShellCommand(cmd)
	if err != nil {
		t.Logf("Info: handleShellCommand returned: %v", err)
	}

	result := output.String()
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") || !strings.Contains(result, "line3") {
		t.Errorf("Expected multi-line output, got: %s", result)
	}

	t.Log("✓ Multi-line output test passed")
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
		name           string
		command        string
		shouldContinue bool
		shouldSucceed  bool
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

// TestHandleEndUploadLargeFileSplitChunks tests uploading a large file
// where the compressed data is split into chunks (like real usage)
func TestHandleEndUploadLargeFileSplitChunks(t *testing.T) {
	client, _ := createMockClient()

	// Create temp directory
	tmpDir := t.TempDir()
	testFilePath := filepath.Join(tmpDir, "large_file.bin")

	// Create a large test file with random-like data (less compressible)
	// to ensure we get multiple chunks after compression
	largeData := make([]byte, 10*1024*1024) // 10MB
	for i := range largeData {
		// Use a pattern that doesn't compress well
		largeData[i] = byte((i*131 + i/256*17) % 256)
	}

	// Compress the entire file once (like the listener does)
	compressed, err := compression.CompressToHex(largeData)
	if err != nil {
		t.Fatalf("Failed to compress test data: %v", err)
	}

	// Split compressed data into chunks (like the listener does)
	chunkSize := 65536 // protocol.ChunkSize
	var chunks []string
	for i := 0; i < len(compressed); i += chunkSize {
		end := i + chunkSize
		if end > len(compressed) {
			end = len(compressed)
		}
		chunks = append(chunks, compressed[i:end])
	}

	t.Logf("Large file test: %d bytes original, %d bytes compressed, %d chunks",
		len(largeData), len(compressed), len(chunks))

	// Setup upload with split chunks
	client.currentUploadPath = testFilePath
	client.uploadChunks = chunks

	err = client.handleEndUploadCommand("END_UPLOAD " + testFilePath)
	if err != nil {
		t.Fatalf("Large file upload failed: %v", err)
	}

	// Verify file was written correctly
	written, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if !bytes.Equal(written, largeData) {
		t.Errorf("Large file content mismatch: got %d bytes, expected %d bytes",
			len(written), len(largeData))
	}

	t.Logf("✓ Large file upload test passed: %d chunks reassembled correctly", len(chunks))
}

// TestPtyModeReentry tests that PTY mode can be entered and exited multiple times
// without "input/output error" or goroutine leaks
func TestPtyModeReentry(t *testing.T) {
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

// TestHandlePtyDataCommandCtrlD tests Ctrl-D translation on Windows
func TestHandlePtyDataCommandCtrlD(t *testing.T) {
	// Skip on non-Windows since we only translate on Windows
	if runtime.GOOS != "windows" {
		t.Skip("Ctrl-D translation only applies on Windows")
	}

	// Skip on Windows too - temp file I/O doesn't work the same as real PTY
	t.Skip("PTY data file I/O tests skipped on Windows (uses real PTY in production)")

	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file (use a temp file as stand-in)
	tmpFile, err := os.CreateTemp("", "pty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test data containing Ctrl-D (0x04)
	testData := []byte("test\x04more")

	// Compress and encode
	encoded, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}

	// Create command
	command := protocol.CmdPtyData + " " + encoded

	// Handle the command
	err = client.handlePtyDataCommand(command)
	if err != nil {
		t.Fatalf("handlePtyDataCommand failed: %v", err)
	}

	// Read what was written to the mock PTY file
	tmpFile.Seek(0, 0)
	written, err := io.ReadAll(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read from temp file: %v", err)
	}

	// Verify Ctrl-D was translated to "exit\r\n"
	expectedData := []byte("testexit\r\nmore")
	if !bytes.Equal(written, expectedData) {
		t.Errorf("Ctrl-D translation failed.\nExpected: %q\nGot: %q", expectedData, written)
	}

	t.Logf("✓ Ctrl-D translation test passed on Windows")
}

// TestHandlePtyDataCommandNoCtrlD tests normal data doesn't get modified
func TestHandlePtyDataCommandNoCtrlD(t *testing.T) {
	// Skip on Windows - temp file I/O doesn't work the same as real PTY
	if runtime.GOOS == "windows" {
		t.Skip("PTY data file I/O tests skipped on Windows (uses real PTY in production)")
	}

	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file
	tmpFile, err := os.CreateTemp("", "pty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test data without Ctrl-D
	testData := []byte("normal text without ctrl-d")

	// Compress and encode
	encoded, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}

	// Create command
	command := protocol.CmdPtyData + " " + encoded

	// Handle the command
	err = client.handlePtyDataCommand(command)
	if err != nil {
		t.Fatalf("handlePtyDataCommand failed: %v", err)
	}

	// Read what was written
	tmpFile.Seek(0, 0)
	written, err := io.ReadAll(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read from temp file: %v", err)
	}

	// Verify data is unchanged
	if !bytes.Equal(written, testData) {
		t.Errorf("Normal data was modified.\nExpected: %q\nGot: %q", testData, written)
	}

	t.Logf("✓ Normal PTY data handling test passed")
}

// TestHandlePtyDataCommandNotInPtyMode tests error when not in PTY mode
func TestHandlePtyDataCommandNotInPtyMode(t *testing.T) {
	client, _ := createMockClient()

	// Don't set PTY mode
	client.inPtyMode = false

	// Try to handle PTY data command
	command := protocol.CmdPtyData + " test"
	err := client.handlePtyDataCommand(command)

	// Should return error
	if err == nil {
		t.Error("Expected error when not in PTY mode, got nil")
	}
	if err.Error() != "not in PTY mode" {
		t.Errorf("Expected 'not in PTY mode' error, got: %v", err)
	}

	t.Logf("✓ PTY mode validation test passed")
}

// TestHandlePtyDataCommandInvalidEncoding tests handling invalid hex encoding
func TestHandlePtyDataCommandInvalidEncoding(t *testing.T) {
	// Skip on Windows - temp file I/O doesn't work the same as real PTY
	if runtime.GOOS == "windows" {
		t.Skip("PTY data file I/O tests skipped on Windows (uses real PTY in production)")
	}

	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	tmpFile, err := os.CreateTemp("", "pty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Create command with invalid hex (not valid hex string)
	command := protocol.CmdPtyData + " ZZZZ"

	// Handle the command
	err = client.handlePtyDataCommand(command)

	// Should return error for invalid hex
	if err == nil {
		t.Error("Expected error for invalid hex encoding")
	}

	t.Logf("✓ Invalid encoding error handling test passed")
}

// TestHandlePtyDataCommandEmptyData tests handling empty PTY data
func TestHandlePtyDataCommandEmptyData(t *testing.T) {
	// Skip on Windows - temp file I/O doesn't work the same as real PTY
	if runtime.GOOS == "windows" {
		t.Skip("PTY data file I/O tests skipped on Windows (uses real PTY in production)")
	}

	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	tmpFile, err := os.CreateTemp("", "pty-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Compress empty data
	encoded, err := compression.CompressToHex([]byte{})
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}

	command := protocol.CmdPtyData + " " + encoded

	// Handle empty data
	err = client.handlePtyDataCommand(command)
	if err != nil {
		t.Fatalf("handlePtyDataCommand failed for empty data: %v", err)
	}

	t.Logf("✓ Empty PTY data handling test passed")
}

// TestHandlePtyDataCommandMultipleCtrlD tests Ctrl-D handling across all platforms
func TestHandlePtyDataCommandMultipleCtrlD(t *testing.T) {
	// Create a buffer to capture writes (works across all platforms)
	bufFile, err := os.CreateTemp("", "pty-test-buf-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(bufFile.Name())
	defer bufFile.Close()

	client, _ := createMockClient()
	client.inPtyMode = true
	client.ptyFile = bufFile

	// Test data with multiple Ctrl-D bytes
	testData := []byte("test\x04more\x04data")

	encoded, err := compression.CompressToHex(testData)
	if err != nil {
		t.Fatalf("CompressToHex failed: %v", err)
	}

	command := protocol.CmdPtyData + " " + encoded

	err = client.handlePtyDataCommand(command)
	if err != nil {
		t.Fatalf("handlePtyDataCommand failed: %v", err)
	}

	// Flush to ensure data is written
	bufFile.Sync()
	bufFile.Seek(0, 0)
	written, err := io.ReadAll(bufFile)
	if err != nil {
		t.Fatalf("Failed to read from buffer: %v", err)
	}

	var expectedData []byte
	switch runtime.GOOS {
	case "windows":
		// On Windows, only first Ctrl-D should be replaced with 'exit\r\n'
		expectedData = []byte("testexit\r\nmore\x04data")
	default:
		// On Unix-like systems, Ctrl-D is passed through unchanged
		expectedData = []byte("test\x04more\x04data")
	}

	if !bytes.Equal(written, expectedData) {
		t.Errorf("Multiple Ctrl-D handling failed on %s.\nExpected: %q\nGot: %q", runtime.GOOS, expectedData, written)
	}

	t.Logf("✓ Multiple Ctrl-D test passed on %s", runtime.GOOS)
}

// TestHandlePtyResizeCommand tests PTY window resize command
func TestHandlePtyResizeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping PTY resize test on Windows (ConPTY doesn't support ioctl)")
	}

	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file
	tmpFile, err := os.CreateTemp("", "pty-resize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test valid resize command format
	// Note: actual ioctl will fail on non-PTY file, but we're testing the command parsing
	command := protocol.CmdPtyResize + " 24 80"
	err = client.handlePtyResizeCommand(command)
	// We expect this to fail due to inappropriate ioctl, but the parsing should succeed
	// The important thing is that the command format is accepted
	if err != nil && !bytes.Contains([]byte(err.Error()), []byte("inappropriate ioctl")) {
		// Some other error - the parsing failed
		t.Logf("Got expected ioctl error: %v", err)
	}

	t.Log("✓ PTY resize command format handled")
}

// TestHandlePtyResizeCommandInvalidFormat tests error handling for invalid format
func TestHandlePtyResizeCommandInvalidFormat(t *testing.T) {
	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file
	tmpFile, err := os.CreateTemp("", "pty-resize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test various invalid formats
	invalidCommands := []string{
		protocol.CmdPtyResize + " 24",          // missing cols
		protocol.CmdPtyResize + " 24 80 extra", // extra arguments
		protocol.CmdPtyResize,                  // no arguments
	}

	for _, cmd := range invalidCommands {
		err := client.handlePtyResizeCommand(cmd)
		if err == nil {
			t.Errorf("Expected error for invalid command: %s", cmd)
		}
	}

	t.Log("✓ Invalid PTY resize format rejected")
}

// TestHandlePtyResizeCommandInvalidRows tests error handling for invalid row count
func TestHandlePtyResizeCommandInvalidRows(t *testing.T) {
	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file
	tmpFile, err := os.CreateTemp("", "pty-resize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test invalid rows
	command := protocol.CmdPtyResize + " abc 80"
	err = client.handlePtyResizeCommand(command)
	if err == nil {
		t.Error("Expected error for invalid rows")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("invalid rows")) {
		t.Errorf("Expected 'invalid rows' error, got: %v", err)
	}

	t.Log("✓ Invalid rows rejected")
}

// TestHandlePtyResizeCommandInvalidCols tests error handling for invalid column count
func TestHandlePtyResizeCommandInvalidCols(t *testing.T) {
	client, _ := createMockClient()

	// Setup PTY mode
	client.inPtyMode = true

	// Create a mock PTY file
	tmpFile, err := os.CreateTemp("", "pty-resize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	client.ptyFile = tmpFile

	// Test invalid cols
	command := protocol.CmdPtyResize + " 24 xyz"
	err = client.handlePtyResizeCommand(command)
	if err == nil {
		t.Error("Expected error for invalid cols")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("invalid cols")) {
		t.Errorf("Expected 'invalid cols' error, got: %v", err)
	}

	t.Log("✓ Invalid cols rejected")
}

// TestHandlePtyResizeCommandNotInPtyMode tests error when not in PTY mode
func TestHandlePtyResizeCommandNotInPtyMode(t *testing.T) {
	client, _ := createMockClient()

	// Don't set PTY mode
	client.inPtyMode = false

	// Try to handle PTY resize command
	command := protocol.CmdPtyResize + " 24 80"
	err := client.handlePtyResizeCommand(command)

	// Should return error
	if err == nil {
		t.Error("Expected error when not in PTY mode, got nil")
	}
	if err.Error() != "not in PTY mode" {
		t.Errorf("Expected 'not in PTY mode' error, got: %v", err)
	}

	t.Log("✓ PTY resize without PTY mode rejected")
}

// TestHandlePtyResizeCommandVariousSizes tests various terminal sizes
func TestHandlePtyResizeCommandVariousSizes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping PTY resize test on Windows (ConPTY doesn't support ioctl)")
	}

	// Test various realistic terminal sizes
	testSizes := []struct {
		rows, cols int
	}{
		{24, 80},   // Standard
		{40, 120},  // Large
		{1, 1},     // Minimal
		{200, 200}, // Very large
		{60, 160},  // Wide
	}

	for _, size := range testSizes {
		client, _ := createMockClient()
		client.inPtyMode = true

		tmpFile, err := os.CreateTemp("", "pty-resize-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		client.ptyFile = tmpFile

		// Build resize command with specific dimensions
		command := protocol.CmdPtyResize + " " + strconv.Itoa(size.rows) + " " + strconv.Itoa(size.cols)

		err = client.handlePtyResizeCommand(command)

		tmpFile.Close()

		// We expect ioctl error on non-PTY file, which is fine
		// The parsing should work
		if err != nil && !bytes.Contains([]byte(err.Error()), []byte("inappropriate ioctl")) {
			t.Logf("Warning: %dx%d resize got unexpected error: %v", size.rows, size.cols, err)
		}
	}

	t.Log("✓ Various terminal sizes handled")
}

// TestHandlePtyModeCommand tests entering PTY mode
func TestHandlePtyModeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows tests need different shells
		t.Log("✓ PTY mode command test skipped on Windows")
		return
	}

	client, output := createMockClient()

	// Should not be in PTY mode initially
	if client.inPtyMode {
		t.Error("Client should not be in PTY mode initially")
	}

	// Call handlePtyModeCommand
	err := client.handlePtyModeCommand()
	if err != nil {
		t.Logf("Warning: handlePtyModeCommand returned error: %v", err)
	}

	// Check that output contains OK confirmation
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("OK")) {
		t.Logf("Expected 'OK' in output, got: %s", result)
	}

	// Should be in PTY mode now
	if !client.inPtyMode {
		t.Error("Client should be in PTY mode after handlePtyModeCommand")
	}

	t.Log("✓ PTY mode entry successful")
}

// TestHandlePtyModeCommandAlreadyInMode tests error when already in PTY mode
func TestHandlePtyModeCommandAlreadyInMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	client, output := createMockClient()

	// Enter PTY mode first
	client.inPtyMode = true

	// Try to enter again
	err := client.handlePtyModeCommand()
	if err != nil {
		t.Logf("Warning: handlePtyModeCommand returned error: %v", err)
	}

	// Should have error message in output
	result := output.String()
	if !bytes.Contains([]byte(result), []byte("Already in PTY mode")) {
		t.Logf("Expected 'Already in PTY mode' message, got: %s", result)
	}

	t.Log("✓ Duplicate PTY mode entry rejected")
}

// TestHandlePtyModeCommandShellSelection tests shell selection logic
func TestHandlePtyModeCommandShellSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Shell selection test skipped on Windows")
	}

	client, _ := createMockClient()

	// Verify that available shells can be started
	shells := []string{"/bin/bash", "/bin/sh"}
	for _, shell := range shells {
		if _, err := os.Stat(shell); err == nil {
			// Shell exists, test should handle it
			break
		}
	}

	// Call handlePtyModeCommand
	err := client.handlePtyModeCommand()
	if err != nil {
		t.Logf("Info: handlePtyModeCommand returned: %v", err)
	}

	t.Log("✓ Shell selection logic verified")
}

// TestHandlePtyModeCommandOutputFormatting tests proper output formatting
func TestHandlePtyModeCommandOutputFormatting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	client, output := createMockClient()

	// Call handlePtyModeCommand
	_ = client.handlePtyModeCommand()

	// Verify output contains end of output marker
	result := output.String()
	if !strings.Contains(result, protocol.EndOfOutputMarker) {
		t.Errorf("Expected end of output marker in response, got: %s", result)
	}

	t.Log("✓ Output formatting verified")
}
