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
	connections map[string]map[string]net.Conn // fwdID -> connID -> conn
	mu          sync.RWMutex
	sendFunc    func(string)
}

// NewForwardHandler creates a new forward handler
func NewForwardHandler(sendFunc func(string)) *ForwardHandler {
	return &ForwardHandler{
		connections: make(map[string]map[string]net.Conn),
		sendFunc:    sendFunc,
	}
}

// HandleForwardStart handles a FORWARD_START command
func (fh *ForwardHandler) HandleForwardStart(fwdID, connID, targetAddr string) error {
	// Validate that targetAddr is in host:port format
	if !strings.Contains(targetAddr, ":") {
		err := fmt.Errorf("invalid target address format: %s (expected host:port, e.g., 127.0.0.1:8080)", targetAddr)
		logging.Warnf("[-] %v", err)
		fh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdForwardStop, fwdID, connID))
		return err
	}

	fh.mu.Lock()
	if _, exists := fh.connections[fwdID]; !exists {
		fh.connections[fwdID] = make(map[string]net.Conn)
	}
	// If same connID exists, close it before replacing
	if existing, exists := fh.connections[fwdID][connID]; exists {
		existing.Close()
		delete(fh.connections[fwdID], connID)
	}
	fh.mu.Unlock()

	// Connect to target
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logging.Warnf("[-] Failed to connect to %s: %v", targetAddr, err)
		fh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdForwardStop, fwdID, connID))
		return fmt.Errorf("failed to connect to %s: %w", targetAddr, err)
	}

	fh.mu.Lock()
	fh.connections[fwdID][connID] = conn
	fh.mu.Unlock()
	logging.Debugf("[+] Forward %s: connected to %s", fwdID, targetAddr)

	// Start reading from target and sending back
	go fh.readFromTarget(fwdID, connID, conn)

	return nil
}

// readFromTarget reads data from the target connection and sends it back
func (fh *ForwardHandler) readFromTarget(fwdID, connID string, conn net.Conn) {
	defer func() {
		fh.mu.Lock()
		if conns, ok := fh.connections[fwdID]; ok {
			delete(conns, connID)
			if len(conns) == 0 {
				delete(fh.connections, fwdID)
			}
		}
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
			fh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdForwardStop, fwdID, connID))
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
	conns, exists := fh.connections[fwdID]
	if exists {
		conn, ok := conns[connID]
		if ok {
			fh.mu.RUnlock()
			data, err := base64.StdEncoding.DecodeString(encodedData)
			if err != nil {
				return fmt.Errorf("failed to decode data: %w", err)
			}

			_, err = conn.Write(data)
			if err != nil {
				logging.Warnf("[-] Forward %s conn %s write error: %v", fwdID, connID, err)
				fh.HandleForwardStop(fwdID, connID)
				return err
			}
			return nil
		}
	}
	fh.mu.RUnlock()

	return fmt.Errorf("forward %s conn %s not found", fwdID, connID)
}

// HandleForwardStop handles FORWARD_STOP command
func (fh *ForwardHandler) HandleForwardStop(fwdID, connID string) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.closeConnection(fwdID, connID)
}

// closeConnection closes a connection (must be called with lock held)
func (fh *ForwardHandler) closeConnection(fwdID, connID string) {
	if conns, ok := fh.connections[fwdID]; ok {
		if conn, exists := conns[connID]; exists {
			conn.Close()
			delete(conns, connID)
			logging.Debugf("[+] Closed forward %s conn %s", fwdID, connID)
		}
		if len(conns) == 0 {
			delete(fh.connections, fwdID)
		}
	}
}

// Close closes all connections
func (fh *ForwardHandler) Close() {
	fh.mu.Lock()
	defer fh.mu.Unlock()

	for fwdID, conns := range fh.connections {
		for connID, conn := range conns {
			conn.Close()
			delete(conns, connID)
		}
		delete(fh.connections, fwdID)
	}
}

// benign close detection moved to logutil.go
