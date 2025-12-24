package server

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/frjcomp/gots/pkg/protocol"
)

// SOCKS5 protocol constants
const (
	socks5Version = 0x05
	socks5NoAuth  = 0x00
	socks5Connect = 0x01
	socks5IPv4    = 0x01
	socks5Domain  = 0x03
	socks5IPv6    = 0x04
	
	socks5Success          = 0x00
	socks5GeneralFailure   = 0x01
	socks5ConnectionDenied = 0x05
	socks5HostUnreachable  = 0x04
)

// SocksConnection represents a single SOCKS5 connection
type SocksConnection struct {
	ID       string
	TargetAddr string
	Active   bool
}

// SocksProxy manages SOCKS5 proxy connections
type SocksProxy struct {
	ID          string
	LocalAddr   string
	Listener    net.Listener
	Active      bool
	connections map[string]net.Conn // connID -> connection
	connReady   map[string]chan bool // connID -> ready signal
	connCount   int
	mu          sync.Mutex
	sendFunc    func(string)
}

// SocksManager manages SOCKS5 proxies
type SocksManager struct {
	proxies map[string]*SocksProxy
	mu      sync.RWMutex
}

// NewSocksManager creates a new SOCKS manager
func NewSocksManager() *SocksManager {
	return &SocksManager{
		proxies: make(map[string]*SocksProxy),
	}
}

// StartSocks starts a new SOCKS5 proxy
func (sm *SocksManager) StartSocks(id, localPort string, sendFunc func(string)) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.proxies[id]; exists {
		return fmt.Errorf("SOCKS proxy %s already exists", id)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:"+localPort)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", localPort, err)
	}

	proxy := &SocksProxy{
		ID:          id,
		LocalAddr:   listener.Addr().String(),
		Listener:    listener,
		Active:      true,
		connections: make(map[string]net.Conn),
		connReady:   make(map[string]chan bool),
		sendFunc:    sendFunc,
	}

	sm.proxies[id] = proxy

	// Send SOCKS_START to client
	sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdSocksStart, id))

	// Start accepting connections
	go sm.acceptConnections(proxy)

	return nil
}

// acceptConnections accepts incoming SOCKS5 connections
func (sm *SocksManager) acceptConnections(proxy *SocksProxy) {
	for {
		conn, err := proxy.Listener.Accept()
		if err != nil {
			proxy.mu.Lock()
			active := proxy.Active
			proxy.mu.Unlock()
			if !active {
				return
			}
			log.Printf("[-] SOCKS %s accept error: %v", proxy.ID, err)
			continue
		}

		proxy.mu.Lock()
		proxy.connCount++
		connID := fmt.Sprintf("%d", proxy.connCount)
		proxy.connections[connID] = conn
		proxy.mu.Unlock()

		log.Printf("[+] SOCKS %s: new connection %s from %s", proxy.ID, connID, conn.RemoteAddr())

		// Handle SOCKS5 handshake and proxy
		go sm.handleSocksConnection(proxy, connID, conn)
	}
}

// handleSocksConnection handles a single SOCKS5 connection
func (sm *SocksManager) handleSocksConnection(proxy *SocksProxy, connID string, conn net.Conn) {
	defer func() {
		conn.Close()
		proxy.mu.Lock()
		delete(proxy.connections, connID)
		proxy.mu.Unlock()
		proxy.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, proxy.ID, connID))
	}()

	// SOCKS5 handshake: client -> [version, nauth, auth_methods]
	buf := make([]byte, 257)
	n, err := conn.Read(buf)
	if err != nil || n < 2 {
		log.Printf("[-] SOCKS %s conn %s: handshake read error", proxy.ID, connID)
		return
	}

	version := buf[0]
	if version != socks5Version {
		log.Printf("[-] SOCKS %s conn %s: unsupported version %d", proxy.ID, connID, version)
		return
	}

	// Send: [version, selected_auth_method]
	_, err = conn.Write([]byte{socks5Version, socks5NoAuth})
	if err != nil {
		log.Printf("[-] SOCKS %s conn %s: handshake write error", proxy.ID, connID)
		return
	}

	// Read request: [version, cmd, reserved, addr_type, addr, port]
	n, err = conn.Read(buf)
	if err != nil || n < 4 {
		log.Printf("[-] SOCKS %s conn %s: request read error", proxy.ID, connID)
		return
	}

	if buf[0] != socks5Version {
		log.Printf("[-] SOCKS %s conn %s: bad request version", proxy.ID, connID)
		return
	}

	cmd := buf[1]
	if cmd != socks5Connect {
		log.Printf("[-] SOCKS %s conn %s: unsupported command %d", proxy.ID, connID, cmd)
		// Send failure response
		conn.Write([]byte{socks5Version, socks5GeneralFailure, 0x00, socks5IPv4, 0, 0, 0, 0, 0, 0})
		return
	}

	addrType := buf[3]
	var targetAddr string
	var addrEnd int

	switch addrType {
	case socks5IPv4:
		if n < 10 {
			log.Printf("[-] SOCKS %s conn %s: incomplete IPv4 address", proxy.ID, connID)
			return
		}
		targetAddr = fmt.Sprintf("%d.%d.%d.%d", buf[4], buf[5], buf[6], buf[7])
		addrEnd = 8
	case socks5Domain:
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			log.Printf("[-] SOCKS %s conn %s: incomplete domain address", proxy.ID, connID)
			return
		}
		targetAddr = string(buf[5 : 5+domainLen])
		addrEnd = 5 + domainLen
	case socks5IPv6:
		if n < 22 {
			log.Printf("[-] SOCKS %s conn %s: incomplete IPv6 address", proxy.ID, connID)
			return
		}
		// Format IPv6 address
		targetAddr = fmt.Sprintf("[%x:%x:%x:%x:%x:%x:%x:%x]",
			binary.BigEndian.Uint16(buf[4:6]),
			binary.BigEndian.Uint16(buf[6:8]),
			binary.BigEndian.Uint16(buf[8:10]),
			binary.BigEndian.Uint16(buf[10:12]),
			binary.BigEndian.Uint16(buf[12:14]),
			binary.BigEndian.Uint16(buf[14:16]),
			binary.BigEndian.Uint16(buf[16:18]),
			binary.BigEndian.Uint16(buf[18:20]))
		addrEnd = 20
	default:
		log.Printf("[-] SOCKS %s conn %s: unsupported address type %d", proxy.ID, connID, addrType)
		conn.Write([]byte{socks5Version, socks5GeneralFailure, 0x00, socks5IPv4, 0, 0, 0, 0, 0, 0})
		return
	}

	port := binary.BigEndian.Uint16(buf[addrEnd : addrEnd+2])
	targetAddr = fmt.Sprintf("%s:%d", targetAddr, port)

	log.Printf("[+] SOCKS %s conn %s: connecting to %s", proxy.ID, connID, targetAddr)

	// Create a ready signal for this connection
	readyChan := make(chan bool, 1)
	proxy.mu.Lock()
	proxy.connReady[connID] = readyChan
	proxy.mu.Unlock()

	// Send connection request to client
	proxy.sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdSocksConn, proxy.ID, connID, targetAddr))

	// Send success response (we'll handle actual connection on client side)
	// Response: [version, status, reserved, addr_type, addr, port]
	response := []byte{socks5Version, socks5Success, 0x00, socks5IPv4, 0, 0, 0, 0, 0, 0}
	_, err = conn.Write(response)
	if err != nil {
		log.Printf("[-] SOCKS %s conn %s: failed to send success response", proxy.ID, connID)
		proxy.mu.Lock()
		delete(proxy.connReady, connID)
		proxy.mu.Unlock()
		return
	}

	// Wait for client to establish remote connection (with timeout)
	select {
	case <-readyChan:
		log.Printf("[+] SOCKS %s conn %s: remote connection established", proxy.ID, connID)
	case <-time.After(5 * time.Second):
		log.Printf("[-] SOCKS %s conn %s: timeout waiting for remote connection", proxy.ID, connID)
		proxy.mu.Lock()
		delete(proxy.connReady, connID)
		proxy.mu.Unlock()
		return
	}

	// Now relay data bidirectionally
	sm.relayData(proxy, connID, conn)
}

// relayData relays data between local connection and remote
func (sm *SocksManager) relayData(proxy *SocksProxy, connID string, conn net.Conn) {
	buffer := make([]byte, 32768)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("[-] SOCKS %s conn %s read error: %v", proxy.ID, connID, err)
			}
			return
		}

		if n > 0 {
			// Encode and send to client
			encoded := base64.StdEncoding.EncodeToString(buffer[:n])
			proxy.sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdSocksData, proxy.ID, connID, encoded))
		}
	}
}

// SignalSocksReady signals that a remote connection is established
func (sm *SocksManager) SignalSocksReady(socksID, connID string) {
	sm.mu.RLock()
	proxy, exists := sm.proxies[socksID]
	sm.mu.RUnlock()

	if !exists {
		return
	}

	proxy.mu.Lock()
	if readyChan, exists := proxy.connReady[connID]; exists {
		select {
		case readyChan <- true:
		default:
		}
		delete(proxy.connReady, connID)
	}
	proxy.mu.Unlock()
}

// HandleSocksData handles incoming data from the remote side
func (sm *SocksManager) HandleSocksData(socksID, connID, encodedData string) error {
	sm.mu.RLock()
	proxy, exists := sm.proxies[socksID]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("SOCKS proxy %s not found", socksID)
	}

	proxy.mu.Lock()
	conn, exists := proxy.connections[connID]
	proxy.mu.Unlock()

	if !exists {
		return fmt.Errorf("SOCKS connection %s not found", connID)
	}

	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	_, err = conn.Write(data)
	return err
}

// HandleSocksClose handles connection close from remote side
func (sm *SocksManager) HandleSocksClose(socksID, connID string) {
	sm.mu.RLock()
	proxy, exists := sm.proxies[socksID]
	sm.mu.RUnlock()

	if !exists {
		return
	}

	proxy.mu.Lock()
	if conn, exists := proxy.connections[connID]; exists {
		conn.Close()
		delete(proxy.connections, connID)
	}
	proxy.mu.Unlock()
}

// StopSocks stops a SOCKS proxy
func (sm *SocksManager) StopSocks(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	proxy, exists := sm.proxies[id]
	if !exists {
		return fmt.Errorf("SOCKS proxy %s not found", id)
	}

	proxy.mu.Lock()
	proxy.Active = false
	// Close all connections
	for _, conn := range proxy.connections {
		conn.Close()
	}
	proxy.connections = make(map[string]net.Conn)
	proxy.mu.Unlock()

	proxy.Listener.Close()
	delete(sm.proxies, id)

	log.Printf("[+] Stopped SOCKS proxy %s", id)
	return nil
}

// ListSocks returns a list of active SOCKS proxies
func (sm *SocksManager) ListSocks() []*SocksProxy {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]*SocksProxy, 0, len(sm.proxies))
	for _, proxy := range sm.proxies {
		result = append(result, proxy)
	}
	return result
}

// StopAll stops all SOCKS proxies
func (sm *SocksManager) StopAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for id, proxy := range sm.proxies {
		proxy.mu.Lock()
		proxy.Active = false
		for _, conn := range proxy.connections {
			conn.Close()
		}
		proxy.mu.Unlock()
		proxy.Listener.Close()
		delete(sm.proxies, id)
	}
}
