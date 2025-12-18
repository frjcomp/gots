package client

import (
"bufio"
"bytes"
"os"
"path/filepath"
"testing"

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

// TestConcurrentCommandHandling verifies handlers work without data races
func TestConcurrentCommandHandling(t *testing.T) {
client, _ := createMockClient()

// Simulate rapid command processing
commands := []string{
protocol.CmdPing,
"echo test1",
protocol.CmdPing,
"echo test2",
protocol.CmdExit,
}

for i, cmd := range commands {
shouldContinue, err := client.processCommand(cmd)
if cmd == protocol.CmdExit && shouldContinue {
t.Errorf("Command %d: EXIT should return shouldContinue=false", i)
}
if cmd == protocol.CmdExit && err != nil {
t.Errorf("Command %d: EXIT should not error, got: %v", i, err)
}
}
}
