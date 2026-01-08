package server

import (
	"net"
	"time"
)

// ListenerInterface defines the interface for a TLS listener that manages reverse shell client connections.
// It handles accepting client connections, managing the client pool, and executing commands on clients.
type ListenerInterface interface {
	// Start begins listening for incoming client connections.
	// Returns the underlying net.Listener and any error that occurred.
	Start() (net.Listener, error)

	// GetClients returns a list of currently connected client addresses.
	GetClients() []string

	// SendCommand sends a command to a specific client by its address.
	// Returns an error if the client is not connected or the send fails.
	SendCommand(clientAddr, cmd string) error

	// GetResponse waits for and retrieves the response from a specific client.
	// Blocks until a response is available or timeout is exceeded.
	GetResponse(clientAddr string, timeout time.Duration) (string, error)

	// GetClientAddressesSorted returns a sorted list of connected client addresses.
	GetClientAddressesSorted() []string

	// GetClientIdentifier returns the short identifier for a client if present.
	// Returns empty string when no identifier was announced.
	GetClientIdentifier(clientAddr string) string

	// GetClientMetadata returns metadata sent during IDENT, if available.
	GetClientMetadata(clientAddr string) (ClientMetadata, bool)

	// EnterPtyMode enters interactive PTY mode with a specific client.
	// Returns a channel that receives PTY data, or an error if mode entry fails.
	EnterPtyMode(clientAddr string) (chan []byte, error)

	// ExitPtyMode exits interactive PTY mode with a specific client.
	ExitPtyMode(clientAddr string) error

	// IsInPtyMode checks if a specific client is in PTY mode.
	IsInPtyMode(clientAddr string) bool

	// GetPtyDataChan retrieves the data channel for a client in PTY mode.
	GetPtyDataChan(clientAddr string) (chan []byte, bool)
}

// CommandHandler defines the interface for handling command execution on clients.
type CommandHandler interface {
	// ExecuteCommand sends a command and gets the response.
	ExecuteCommand(cmd string) (string, error)

	// IsConnected checks if the command handler has an active connection.
	IsConnected() bool
}

// ListenerStats defines the interface for retrieving listener statistics.
type ListenerStats interface {
	// GetClientCount returns the number of currently connected clients.
	GetClientCount() int

	// GetTotalConnections returns the total number of connections made since startup.
	GetTotalConnections() int

	// GetLastError returns the last error that occurred.
	GetLastError() error
}
