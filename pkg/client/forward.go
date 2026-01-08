package client

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/frjcomp/gots/pkg/logging"
	"github.com/frjcomp/gots/pkg/protocol"
)

// ForwardHandler manages port forwarding on the client side
type ForwardHandler struct {
	connections map[string]net.Conn // key: fwdID
	connIDs     map[string]string    // fwdID -> connID
	mu          sync.RWMutex
	sendFunc    func(string)
}

// NewForwardHandler creates a new forward handler
func NewForwardHandler(sendFunc func(string)) *ForwardHandler {
	return &ForwardHandler{
		connections: make(map[string]net.Conn),
		connIDs:     make(map[string]string),
		sendFunc:    sendFunc,
	}
}

// HandleForwardStart handles a FORWARD_START command
func (fh *ForwardHandler) HandleForwardStart(fwdID, connID, targetAddr string) error {
	// Validate that targetAddr is in host:port format
	if !strings.Contains(targetAddr, ":") {
		err := fmt.Errorf("invalid target address format: %s (expected host:port, e.g., 127.0.0.1:8080)", targetAddr)
		logging.Warnf("[-] %v", err)
		fh.sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdForwardStop, fwdID))
		return err
	}

	fh.mu.Lock()
	defer fh.mu.Unlock()

	// Check if already exists
	if _, exists := fh.connections[fwdID]; exists {
		logging.Warnf("[-] Forward %s already exists, closing old connection", fwdID)
		fh.closeConnection(fwdID)
	}

	// Connect to target
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logging.Warnf("[-] Failed to connect to %s: %v", targetAddr, err)
		fh.sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdForwardStop, fwdID))
		return fmt.Errorf("failed to connect to %s: %w", targetAddr, err)
	}

	fh.connections[fwdID] = conn
	fh.connIDs[fwdID] = connID
	logging.Debugf("[+] Forward %s: connected to %s", fwdID, targetAddr)

	// Start reading from target and sending back
	go fh.readFromTarget(fwdID, connID, conn)

	return nil
}

// readFromTarget reads data from the target connection and sends it back
func (fh *ForwardHandler) readFromTarget(fwdID, connID string, conn net.Conn) {
	defer func() {
		fh.mu.Lock()
		delete(fh.connections, fwdID)
		delete(fh.connIDs, fwdID)
		fh.mu.Unlock()
		conn.Close()
	}()

	buffer := make([]byte, 32768)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF && !isBenignCloseError(err) {
				logging.Warnf("[-] Forward %s read error: %v", fwdID, err)
			} else {
				logging.Debugf("[-] Forward %s read error: %v", fwdID, err)
			}
			// Notify server that connection is closed
			fh.sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdForwardStop, fwdID))
			return
		}

		if n > 0 {
			// Encode and send data back with the correct connID
			encoded := base64.StdEncoding.EncodeToString(buffer[:n])
			fh.sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdForwardData, fwdID, connID, encoded))
		}
	}
}

// HandleForwardData handles incoming FORWARD_DATA from server
func (fh *ForwardHandler) HandleForwardData(fwdID, connID, encodedData string) error {
	fh.mu.RLock()
	conn, exists := fh.connections[fwdID]
	fh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("forward %s not found", fwdID)
	}

	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		logging.Warnf("[-] Forward %s write error: %v", fwdID, err)
		fh.sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdForwardStop, fwdID))
		fh.mu.Lock()
		fh.closeConnection(fwdID)
		fh.mu.Unlock()
		return err
	}

	return nil
}

// HandleForwardStop handles FORWARD_STOP command
func (fh *ForwardHandler) HandleForwardStop(fwdID string) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.closeConnection(fwdID)
}

// closeConnection closes a connection (must be called with lock held)
func (fh *ForwardHandler) closeConnection(fwdID string) {
	if conn, exists := fh.connections[fwdID]; exists {
		conn.Close()
		delete(fh.connections, fwdID)
		delete(fh.connIDs, fwdID)
		logging.Debugf("[+] Closed forward %s", fwdID)
	}
}

// Close closes all connections
func (fh *ForwardHandler) Close() {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	for fwdID, conn := range fh.connections {
		conn.Close()
		delete(fh.connections, fwdID)
		delete(fh.connIDs, fwdID)
	}
}

// benign close detection moved to logutil.go
