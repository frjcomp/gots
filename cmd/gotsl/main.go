package main

import (
	"crypto/tls"
	"fmt"
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang-https-rev/pkg/certs"
	"golang-https-rev/pkg/compression"
	"golang-https-rev/pkg/protocol"
	"golang-https-rev/pkg/server"
	"golang-https-rev/pkg/version"
)

func printHeader() {
	fmt.Println()
	fmt.Println(` ██████╗  ██████╗ ████████╗ ██████╗  ██╗      `)
	fmt.Println(`██╔════╝ ██╔═══██╗╚══██╔══╝██╔════╝ ██║      `)
	fmt.Println(`██║  ███╗██║   ██║   ██║   ██████╗  ██║      `)
	fmt.Println(`██║   ██║██║   ██║   ██║   ██╔══██╗ ██║      `)
	fmt.Println(`╚██████╔╝╚██████╔╝   ██║   ╚██████╔╝███████╗ `)
	fmt.Println(` ╚═════╝  ╚═════╝    ╚═╝    ╚═════╝ ╚══════╝ `)
	fmt.Println()
	fmt.Println("  GOTSL - Go TLS Listener")
	fmt.Println()
}

func main() {
	if err := runListener(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func runListener(args []string) error {
	printHeader()

	if len(args) != 2 {
		return fmt.Errorf("Usage: gotsl <port> <network-interface>")
	}

	port := args[0]
	networkInterface := args[1]

	log.Println("Generating self-signed certificate...")
	cert, err := certs.GenerateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}
	log.Println("Certificate generated successfully")

	log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create listener
	listener := server.NewListener(port, networkInterface, tlsConfig)
	netListener, err := listener.Start()
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	defer netListener.Close()

	log.Println("Listener ready. Waiting for connections...")
	interactiveShell(listener)
	return nil
}

type listenerInterface interface {
	GetClients() []string
	SendCommand(client, cmd string) error
	GetResponse(client string, timeout time.Duration) (string, error)
}

func interactiveShell(l *server.Listener) {
	reader := bufio.NewReader(os.Stdin)

	printHelp()

	for {
		fmt.Print("listener> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			// Treat EOF (Ctrl-D) as exit; other errors just return
			return
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := parts[0]

		switch command {
		case "ls", "dir":
			listClients(l)
		case "shell":
			if len(parts) < 2 {
				fmt.Println("Usage: shell <client_id>")
				continue
			}
			clientAddr := getClientByID(l, parts[1])
			if clientAddr == "" {
				continue
			}
			enterPtyShell(l, clientAddr)
		case "upload":
			if len(parts) != 4 {
				fmt.Println("Usage: upload <client_id> <local_path> <remote_path>")
				continue
			}
			clientAddr := getClientByID(l, parts[1])
			if clientAddr == "" {
				continue
			}
			handleUploadGlobal(l, clientAddr, parts[2], parts[3])
		case "download":
			if len(parts) != 4 {
				fmt.Println("Usage: download <client_id> <remote_path> <local_path>")
				continue
			}
			clientAddr := getClientByID(l, parts[1])
			if clientAddr == "" {
				continue
			}
			handleDownloadGlobal(l, clientAddr, parts[2], parts[3])
		case "exit":
			return
		default:
			fmt.Printf("Unknown command: %s (type 'help' or see available commands above)\n", command)
		}
	}
}

func printHelp() {
	fmt.Println("\nCommands:")
	fmt.Println("  ls                          - List connected clients")
	fmt.Println("  shell <client_id>           - Open interactive PTY shell with client")
	fmt.Println("  upload <id> <local> <remote> - Upload local file to remote path on client")
	fmt.Println("  download <id> <remote> <local> - Download remote file from client")
	fmt.Println("  exit                        - Exit the listener")
	fmt.Println()
	fmt.Println("In PTY shell mode:")
	fmt.Println("  Ctrl-D                      - Return to listener prompt")
	fmt.Println("  Ctrl-C                      - Send interrupt signal to remote shell")
	fmt.Println()
}

func listClients(l listenerInterface) {
	clients := l.GetClients()
	if len(clients) == 0 {
		fmt.Println("No clients connected")
	} else {
		fmt.Println("\nConnected Clients:")
		for i, addr := range clients {
			fmt.Printf("  %d. %s\n", i+1, addr)
		}
		fmt.Println()
	}
}

func getClientByID(l listenerInterface, idStr string) string {
	var numIdx int
	if _, err := fmt.Sscanf(idStr, "%d", &numIdx); err != nil {
		fmt.Printf("Invalid client ID: %s\n", idStr)
		return ""
	}

	clients := l.GetClients()
	if numIdx > 0 && numIdx <= len(clients) {
		return clients[numIdx-1]
	}

	fmt.Println("Client not found")
	return ""
}

func handleUploadGlobal(l listenerInterface, currentClient, localPath, remotePath string) bool {
	data, err := os.ReadFile(localPath)
	if err != nil {
		fmt.Printf("Error reading local file: %v\n", err)
		return true
	}

	compressed, err := compression.CompressToHex(data)
	if err != nil {
		fmt.Printf("Error compressing file: %v\n", err)
		return true
	}

	totalSize := len(compressed)
	startCmd := fmt.Sprintf("%s %s %d", protocol.CmdStartUpload, remotePath, totalSize)
	if err := l.SendCommand(currentClient, startCmd); err != nil {
		fmt.Printf("Error starting upload: %v\n", err)
		return false
	}

	resp, err := l.GetResponse(currentClient, 30*time.Second)
	if err != nil {
		fmt.Printf("Error getting start upload response: %v\n", err)
		return false
	}
	if !strings.Contains(resp, "OK") {
		fmt.Printf("Error starting upload: unexpected response: %s\n", strings.TrimSpace(strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")))
		return false
	}

	chunkNum := 0
	for i := 0; i < totalSize; i += protocol.ChunkSize {
		end := i + protocol.ChunkSize
		if end > totalSize {
			end = totalSize
		}
		chunk := compressed[i:end]
		chunkNum++
		chunkCmd := fmt.Sprintf("%s %s", protocol.CmdUploadChunk, chunk)
		if err := l.SendCommand(currentClient, chunkCmd); err != nil {
			fmt.Printf("Error sending upload chunk: %v\n", err)
			return false
		}
		resp, err := l.GetResponse(currentClient, 30*time.Second)
		if err != nil {
			fmt.Printf("Error getting chunk response: %v\n", err)
			return false
		}
		if !strings.Contains(resp, "OK") {
			cleanResp := strings.TrimSpace(strings.ReplaceAll(resp, protocol.EndOfOutputMarker, ""))
			fmt.Printf("Chunk upload error: %s\n", cleanResp)
			return false
		}
		fmt.Printf("Uploaded chunk %d: %d bytes\n", chunkNum, len(chunk))
	}

	endCmd := fmt.Sprintf("%s %s", protocol.CmdEndUpload, remotePath)
	if err := l.SendCommand(currentClient, endCmd); err != nil {
		fmt.Printf("Error ending upload: %v\n", err)
		return false
	}

	resp, err = l.GetResponse(currentClient, 30*time.Second)
	if err != nil {
		fmt.Printf("Error getting upload response: %v\n", err)
		return false
	}

	clean := strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")
	fmt.Print(clean)
	if !strings.HasSuffix(clean, "\n") {
		fmt.Println()
	}
	fmt.Printf("Total uploaded: %d bytes (original), %d bytes (compressed)\n", len(data), totalSize)
	return true
}

func handleDownloadGlobal(l listenerInterface, currentClient, remotePath, localPath string) bool {
	cmd := fmt.Sprintf("%s %s", protocol.CmdDownload, remotePath)
	if err := l.SendCommand(currentClient, cmd); err != nil {
		fmt.Printf("Error sending download: %v\n", err)
		return false
	}

	resp, err := l.GetResponse(currentClient, time.Duration(protocol.DownloadTimeout))
	if err != nil {
		fmt.Printf("Error getting download response: %v\n", err)
		return false
	}

	clean := strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")
	clean = strings.TrimSpace(clean)
	if !strings.HasPrefix(clean, protocol.DataPrefix) {
		fmt.Printf("Unexpected download response (length %d bytes)\n", len(clean))
		return true
	}

	payload := strings.TrimPrefix(clean, protocol.DataPrefix)
	decoded, err := compression.DecompressHex(payload)
	if err != nil {
		fmt.Printf("Error decoding payload: %v\n", err)
		return true
	}

	if err := os.WriteFile(localPath, decoded, 0644); err != nil {
		fmt.Printf("Error writing local file: %v\n", err)
		return true
	}

	fmt.Printf("Downloaded %d bytes to %s\n", len(decoded), localPath)
	return true
}

func enterPtyShell(l *server.Listener, clientAddr string) {
	fmt.Printf("Entering PTY shell with %s...\n", clientAddr)
	
	// Send PTY_MODE command
	if err := l.SendCommand(clientAddr, protocol.CmdPtyMode); err != nil {
		fmt.Printf("Error entering PTY mode: %v\n", err)
		return
	}

	// Wait for confirmation
	resp, err := l.GetResponse(clientAddr, 10*time.Second)
	if err != nil {
		fmt.Printf("Error getting PTY mode confirmation: %v\n", err)
		return
	}

	if !strings.Contains(resp, "OK") {
		fmt.Printf("Failed to enter PTY mode: %s\n", strings.ReplaceAll(resp, protocol.EndOfOutputMarker, ""))
		return
	}

	// Enter PTY mode on listener side (creates PTY data channel)
	ptyDataChan, err := l.EnterPtyMode(clientAddr)
	if err != nil {
		fmt.Printf("Error creating PTY data channel: %v\n", err)
		return
	}

	fmt.Println("PTY shell active. Press Ctrl-D to return to listener prompt.")
	fmt.Println("Press Ctrl-C to send interrupt to remote shell.")

	// Setup raw terminal mode for local terminal
	oldState, err := setRawMode()
	if err != nil {
		fmt.Printf("Warning: Could not set raw mode: %v\n", err)
		// Continue anyway
	}
	defer func() {
		// Clear any read deadlines on stdin
		os.Stdin.SetReadDeadline(time.Time{})
		
    	// Restore terminal state
    	// Restore terminal state to what it was before PTY mode
    	// The REPL handles input normally outside PTY mode; restore the saved state
		if oldState != nil {
			restoreTerminal(oldState)
		}
		
		// Force a newline to reset the terminal display
		fmt.Println()
	}()

	// Channel to signal we should exit
	exitPty := make(chan bool, 1)
	remoteClosed := false

	// Forward PTY output to stdout
	go func() {
		for {
			data, ok := <-ptyDataChan
			if !ok {
				// Channel closed - remote PTY exited
				fmt.Printf("\r\n[Remote shell exited]\r\n")
				remoteClosed = true
				exitPty <- true
				return
			}
			os.Stdout.Write(data)
		}
	}()

	// Read from stdin and forward to PTY
	go func() {
		stdinBuf := make([]byte, 1024)
		
		defer func() {
			// Ensure deadline is cleared when goroutine exits
			os.Stdin.SetReadDeadline(time.Time{})
		}()
		
		for {
			select {
			case <-exitPty:
				// Remote closed, stop reading stdin
				return
			default:
				// Continue reading
			}
			
			// Set a read timeout so we can check exitPty periodically
			os.Stdin.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, err := os.Stdin.Read(stdinBuf)
			
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Timeout, check if we should exit
					continue
				}
				if err != io.EOF {
					// Real error
					return
				}
				return
			}

			if n > 0 {
				data := stdinBuf[:n]
				
				// Check for Ctrl-D (EOF)
				if strings.Contains(string(data), "\x04") {
					exitPty <- true
					return
				}

				// Send data immediately to PTY
				encoded, err := compression.CompressToHex(data)
				if err != nil {
					fmt.Printf("\nError encoding input: %v\n", err)
					return
				}
				l.SendCommand(clientAddr, protocol.CmdPtyData+" "+encoded)
			}
		}
	}()

	// Wait for exit signal
	<-exitPty

	// Exit PTY mode (unless remote already closed)
	if !remoteClosed {
		fmt.Println("\nExiting PTY shell...")
		l.SendCommand(clientAddr, protocol.CmdPtyExit)
		time.Sleep(100 * time.Millisecond) // Give it time to process
	}
	
	l.ExitPtyMode(clientAddr)
	
	// Give goroutines a moment to finish
	time.Sleep(100 * time.Millisecond)
}

func setRawMode() (*syscall.Termios, error) {
	// Get current terminal state
	var oldState syscall.Termios
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&oldState))); err != 0 {
		return nil, fmt.Errorf("failed to get terminal state: %v", err)
	}

	// Set raw mode
	newState := oldState
	newState.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	newState.Iflag &^= syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	newState.Oflag &^= syscall.OPOST
	newState.Cflag |= syscall.CS8
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0

	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&newState))); err != 0 {
		return nil, fmt.Errorf("failed to set raw mode: %v", err)
	}

	return &oldState, nil
}

func restoreTerminal(oldState *syscall.Termios) {
	// Restore terminal to original state
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(oldState))); err != 0 {
		log.Printf("Warning: failed to restore terminal state: %v", err)
	}
}
