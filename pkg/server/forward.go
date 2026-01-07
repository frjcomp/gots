package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/frjcomp/gots/pkg/protocol"
)

// ForwardInfo holds information about a port forward
type ForwardInfo struct {
	ID          string
	LocalAddr   string
	RemoteAddr  string
	Listener    net.Listener
	Active      bool
	ConnCount   int
	connections map[string]net.Conn // connID -> local connection (from curl)
	mu          sync.Mutex
}

// ForwardManager manages port forwarding sessions
type ForwardManager struct {
	forwards map[string]*ForwardInfo
	mu       sync.RWMutex
}

// NewForwardManager creates a new forward manager
func NewForwardManager() *ForwardManager {
	return &ForwardManager{
		forwards: make(map[string]*ForwardInfo),
	}
}

// StartForward starts a new port forward
func (fm *ForwardManager) StartForward(id, localPort, remoteAddr string, sendFunc func(string)) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if _, exists := fm.forwards[id]; exists {
		return fmt.Errorf("forward %s already exists", id)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:"+localPort)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", localPort, err)
	}

	info := &ForwardInfo{
		ID:          id,
		LocalAddr:   listener.Addr().String(),
		RemoteAddr:  remoteAddr,
		Listener:    listener,
		Active:      true,
		connections: make(map[string]net.Conn),
	}

	fm.forwards[id] = info

	// Start accepting connections
	go fm.acceptConnections(info, sendFunc)

	return nil
}

// acceptConnections accepts incoming connections and forwards them
func (fm *ForwardManager) acceptConnections(info *ForwardInfo, sendFunc func(string)) {
	for {
		conn, err := info.Listener.Accept()
		if err != nil {
			info.mu.Lock()
			active := info.Active
			info.mu.Unlock()
			if !active {
				return
			}
			log.Printf("[-] Forward %s accept error: %v", info.ID, err)
			continue
		}

		info.mu.Lock()
		info.ConnCount++
		connID := fmt.Sprintf("%d", info.ConnCount)
		info.mu.Unlock()

		log.Printf("[+] Forward %s: new connection %s from %s", info.ID, connID, conn.RemoteAddr())

		// Store the local connection so we can write responses to it
		info.mu.Lock()
		info.connections[connID] = conn
		info.mu.Unlock()

		// Send FORWARD_START to client with connID
		sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdForwardStart, info.ID, connID, info.RemoteAddr))

		// Start forwarding data
		go fm.forwardConnection(info, connID, conn, sendFunc)
	}
}

// forwardConnection handles bidirectional forwarding for a single connection
func (fm *ForwardManager) forwardConnection(info *ForwardInfo, connID string, conn net.Conn, sendFunc func(string)) {
	defer func() {
		conn.Close()
		info.mu.Lock()
		delete(info.connections, connID)
		info.mu.Unlock()
	}()

	// Read from local connection and send to remote
	buffer := make([]byte, 32768)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("[-] Forward %s conn %s read error: %v", info.ID, connID, err)
			}
			// Send stop signal to close remote connection
			sendFunc(fmt.Sprintf("%s %s\n", protocol.CmdForwardStop, info.ID))
			return
		}

		if n > 0 {
			// Encode data and send to client
			encoded := base64.StdEncoding.EncodeToString(buffer[:n])
			sendFunc(fmt.Sprintf("%s %s %s %s\n", protocol.CmdForwardData, info.ID, connID, encoded))
		}
	}
}

// HandleForwardData handles incoming data from the remote side
func (fm *ForwardManager) HandleForwardData(fwdID, connID, encodedData string) error {
	fm.mu.RLock()
	info, exists := fm.forwards[fwdID]
	fm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("forward %s not found", fwdID)
	}

	data, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	info.mu.Lock()
	conn, connExists := info.connections[connID]
	info.mu.Unlock()

	if !connExists {
		return fmt.Errorf("connection %s not found", connID)
	}

	_, err = conn.Write(data)
	return err
}

// StopForward stops a port forward
func (fm *ForwardManager) StopForward(id string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	info, exists := fm.forwards[id]
	if !exists {
		return fmt.Errorf("forward %s not found", id)
	}

	info.mu.Lock()
	info.Active = false
	info.mu.Unlock()

	info.Listener.Close()
	delete(fm.forwards, id)

	log.Printf("[+] Stopped forward %s", id)
	return nil
}

// ListForwards returns a list of active forwards
func (fm *ForwardManager) ListForwards() []*ForwardInfo {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make([]*ForwardInfo, 0, len(fm.forwards))
	for _, info := range fm.forwards {
		result = append(result, info)
	}
	return result
}

// StopAll stops all forwards
func (fm *ForwardManager) StopAll() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for id, info := range fm.forwards {
		info.mu.Lock()
		info.Active = false
		info.mu.Unlock()
		info.Listener.Close()
		delete(fm.forwards, id)
	}
}
