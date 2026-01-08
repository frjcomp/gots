package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/frjcomp/gots/pkg/logging"
	"github.com/frjcomp/gots/pkg/protocol"
)

// SocksHandler manages SOCKS5 connections on the client side
type SocksHandler struct {
	connections map[string]map[string]net.Conn      // socksID -> connID -> connection
	stopChans   map[string]map[string]chan struct{} // socksID -> connID -> stop channel
	mu          sync.RWMutex
	sendFunc    func(string)
}

// defaultResolver allows tests to swap DNS resolution.
var defaultResolver resolver = net.DefaultResolver

type resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

// NewSocksHandler creates a new SOCKS handler
func NewSocksHandler(sendFunc func(string)) *SocksHandler {
	return &SocksHandler{
		connections: make(map[string]map[string]net.Conn),
		stopChans:   make(map[string]map[string]chan struct{}),
		sendFunc:    sendFunc,
	}
}

// HandleSocksStart handles a SOCKS_START command
func (sh *SocksHandler) HandleSocksStart(socksID string) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	if _, exists := sh.connections[socksID]; !exists {
		sh.connections[socksID] = make(map[string]net.Conn)
		sh.stopChans[socksID] = make(map[string]chan struct{})
		logging.Debugf("[+] SOCKS proxy %s started", socksID)
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
		sh.stopChans[socksID] = make(map[string]chan struct{})
	}

	conn, dialAddr, err := sh.dialWithIPv4Preference(targetAddr)
	if err != nil {
		logging.Warnf("[-] SOCKS %s conn %s: failed to connect to %s: %v", socksID, connID, targetAddr, err)
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
		return fmt.Errorf("failed to connect to %s: %w", targetAddr, err)
	}

	sh.connections[socksID][connID] = conn
	stopChan := make(chan struct{})
	sh.stopChans[socksID][connID] = stopChan
	logging.Debugf("[+] SOCKS %s conn %s: connected to %s (dial=%s)", socksID, connID, targetAddr, dialAddr)

	// Signal server that connection is ready
	sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksOk, socksID, connID))

	// Start reading from target and sending back
	go sh.readFromTarget(socksID, connID, conn, stopChan)

	return nil
}

// dialWithIPv4Preference tries to reach targetAddr, preferring IPv4 if available.
// It returns the established connection and the concrete dial address used.
func (sh *SocksHandler) dialWithIPv4Preference(targetAddr string) (net.Conn, string, error) {
	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, "", err
	}
	host = strings.Trim(host, "[]")

	for _, addr := range resolveDialAddresses(host, port) {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err == nil {
			return conn, addr, nil
		}
	}

	return nil, "", fmt.Errorf("all dial attempts failed for %s", targetAddr)
}

// resolveDialAddresses returns dial addresses with IPv4 candidates first when available.
func resolveDialAddresses(host, port string) []string {
	ip := net.ParseIP(host)
	if ip != nil {
		// If IPv6 literal, try IPv4 fallback via reverse DNS, then original
		if ip.To4() == nil {
			if v4 := ipv4FallbackFromReverse(host); v4 != "" {
				return []string{net.JoinHostPort(v4, port), net.JoinHostPort(host, port)}
			}
			return []string{net.JoinHostPort(host, port)}
		}
		// IPv4 literal
		return []string{net.JoinHostPort(ip.String(), port)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ips, err := defaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return []string{net.JoinHostPort(host, port)}
	}

	addrs := make([]string, 0, len(ips))
	for _, ipAddr := range ips {
		if v4 := ipAddr.IP.To4(); v4 != nil {
			addrs = append(addrs, net.JoinHostPort(v4.String(), port))
		}
	}
	for _, ipAddr := range ips {
		if ipAddr.IP.To4() == nil {
			addrs = append(addrs, net.JoinHostPort(ipAddr.IP.String(), port))
		}
	}

	if len(addrs) == 0 {
		return []string{net.JoinHostPort(host, port)}
	}
	return addrs
}

// ipv4FallbackFromReverse attempts reverse DNS on an IPv6 literal to find an IPv4 address.
func ipv4FallbackFromReverse(host string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	names, err := defaultResolver.LookupAddr(ctx, host)
	if err != nil || len(names) == 0 {
		return ""
	}

	name := strings.TrimSuffix(names[0], ".")
	ips, err := defaultResolver.LookupIPAddr(ctx, name)
	if err != nil {
		return ""
	}

	for _, ipAddr := range ips {
		if v4 := ipAddr.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

// readFromTarget reads data from the target connection and sends it back
func (sh *SocksHandler) readFromTarget(socksID, connID string, conn net.Conn, stopChan chan struct{}) {
	defer func() {
		sh.mu.Lock()
		if conns, exists := sh.connections[socksID]; exists {
			delete(conns, connID)
		}
		if stops, exists := sh.stopChans[socksID]; exists {
			delete(stops, connID)
		}
		sh.mu.Unlock()
		conn.Close()
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
	}()

	buffer := make([]byte, 32768)
	for {
		// Check if we should stop reading
		select {
		case <-stopChan:
			return
		default:
		}

		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF && !isBenignCloseError(err) {
				logging.Warnf("[-] SOCKS %s conn %s read error: %v", socksID, connID, err)
			} else {
				logging.Debugf("[-] SOCKS %s conn %s read error: %v", socksID, connID, err)
			}
			return
		}

		if n > 0 {
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
		logging.Warnf("[-] SOCKS %s conn %s write error: %v", socksID, connID, err)
		sh.sendFunc(fmt.Sprintf("%s %s %s\n", protocol.CmdSocksClose, socksID, connID))
		sh.mu.Lock()
		sh.closeConnection(socksID, connID)
		sh.mu.Unlock()
		return err
	}

	return nil
}

// benign close detection moved to logutil.go

// HandleSocksClose handles SOCKS_CLOSE command
func (sh *SocksHandler) HandleSocksClose(socksID, connID string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Signal the read goroutine to stop before closing connection
	if stops, exists := sh.stopChans[socksID]; exists {
		if stopChan, exists := stops[connID]; exists {
			select {
			case <-stopChan:
				// Already closed
			default:
				close(stopChan)
			}
			delete(stops, connID)
		}
	}

	sh.closeConnection(socksID, connID)
}

// closeConnection closes a connection (must be called with lock held)
func (sh *SocksHandler) closeConnection(socksID, connID string) {
	if conns, exists := sh.connections[socksID]; exists {
		if conn, exists := conns[connID]; exists {
			conn.Close()
			delete(conns, connID)
			logging.Debugf("[+] Closed SOCKS %s conn %s", socksID, connID)
		}
	}
}

// Close closes all connections
func (sh *SocksHandler) Close() {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	for socksID, conns := range sh.connections {
		for connID := range conns {
			// Signal read goroutines to stop
			if stops, exists := sh.stopChans[socksID]; exists {
				if stopChan, exists := stops[connID]; exists {
					select {
					case <-stopChan:
						// Already closed
					default:
						close(stopChan)
					}
				}
			}
		}
	}

	for socksID, conns := range sh.connections {
		for connID, conn := range conns {
			conn.Close()
			delete(conns, connID)
		}
		delete(sh.connections, socksID)
	}

	for socksID := range sh.stopChans {
		delete(sh.stopChans, socksID)
	}
}
