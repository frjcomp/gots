package client

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/frjcomp/gots/pkg/protocol"
)

// SocksHandler manages SOCKS5 connections on the client side
type SocksHandler struct {
	connections map[string]map[string]net.Conn // socksID -> connID -> connection
	mu          sync.RWMutex
	sendFunc    func(string)
}

// NewSocksHandler creates a new SOCKS handler
func NewSocksHandler(sendFunc func(string)) *SocksHandler {
	return &SocksHandler{
		connections: make(map[string]map[string]net.Conn),
		sendFunc:    sendFunc,
	}
}

// HandleSocksStart handles a SOCKS_START command
func (sh *SocksHandler) HandleSocksStart(socksID string) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, exists := sh.connections[socksID]; !exists {
		sh.connections[socksID] = make(map[string]net.Conn)
		log.Printf("[+] SOCKS proxy %s started", socksID)
	}
	return nil
}

// HandleSocksConn handles a SOCKS_CONN command - connect to target
func (sh *SocksHandler) HandleSocksConn(socksID, connID, targetAddr string) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Ensure SOCKS proxy exists
	if _, exists := sh.connections[socksID]; !exists {
		sh.connections[socksID] = make(map[string]net.Conn)
	}

	// Connect to target
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("[-] SOCKS %s conn %s: failed to connect to %s: %v", socksID, connID, targetAddr, err)
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
		return fmt.Errorf("failed to connect to %s: %w", targetAddr, err)
	}

	sh.connections[socksID][connID] = conn
	log.Printf("[+] SOCKS %s conn %s: connected to %s", socksID, connID, targetAddr)

	// Signal server that connection is ready
	sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksOk, socksID, connID))
	log.Printf("[+] SOCKS %s conn %s: sent SOCKS_OK, starting relay", socksID, connID)

	// Start reading from target and sending back
	go sh.readFromTarget(socksID, connID, conn)

	return nil
}

// readFromTarget reads data from the target connection and sends it back
func (sh *SocksHandler) readFromTarget(socksID, connID string, conn net.Conn) {
	defer func() {
		sh.mu.Lock()
		if conns, exists := sh.connections[socksID]; exists {
			delete(conns, connID)
		}
		sh.mu.Unlock()
		conn.Close()
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
	}()

	buffer := make([]byte, 32768)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("[-] SOCKS %s conn %s read error: %v", socksID, connID, err)
			}
			return
		}

		if n > 0 {
			log.Printf("[*] SOCKS %s conn %s: relaying %d bytes from target", socksID, connID, n)
			// Encode and send data back
			encoded := base64.StdEncoding.EncodeToString(buffer[:n])
			sh.sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdSocksData, socksID, connID, encoded))
		}
	}
}

// HandleSocksData handles incoming SOCKS_DATA from server
func (sh *SocksHandler) HandleSocksData(socksID, connID, encodedData string) error {
	sh.mu.RLock()
	conns, exists := sh.connections[socksID]
	if !exists {
		sh.mu.RUnlock()
		return fmt.Errorf("SOCKS proxy %s not found", socksID)
	}

	conn, exists := conns[connID]
	sh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("SOCKS connection %s not found", connID)
	}

	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		log.Printf("[-] SOCKS %s conn %s write error: %v", socksID, connID, err)
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
		sh.mu.Lock()
		sh.closeConnection(socksID, connID)
		sh.mu.Unlock()
		return err
	}

	return nil
}

// HandleSocksClose handles SOCKS_CLOSE command
func (sh *SocksHandler) HandleSocksClose(socksID, connID string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.closeConnection(socksID, connID)
}

// closeConnection closes a connection (must be called with lock held)
func (sh *SocksHandler) closeConnection(socksID, connID string) {
	if conns, exists := sh.connections[socksID]; exists {
		if conn, exists := conns[connID]; exists {
			conn.Close()
			delete(conns, connID)
			log.Printf("[+] Closed SOCKS %s conn %s", socksID, connID)
		}
	}
}

// Close closes all connections
func (sh *SocksHandler) Close() {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	for socksID, conns := range sh.connections {
		for connID, conn := range conns {
			conn.Close()
			delete(conns, connID)
		}
		delete(sh.connections, socksID)
	}
}
