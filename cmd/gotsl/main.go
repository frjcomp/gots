package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/frjcomp/gots/pkg/certs"
	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/config"
	"github.com/frjcomp/gots/pkg/protocol"
	"github.com/frjcomp/gots/pkg/server"
	"github.com/frjcomp/gots/pkg/version"
	"golang.org/x/term"
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
}

func main() {
	var useSharedSecret bool
	var port string
	var networkInterface string

	flag.BoolVar(&useSharedSecret, "s", false, "Enable shared secret authentication")
	flag.BoolVar(&useSharedSecret, "shared-secret", false, "Enable shared secret authentication")
	flag.StringVar(&port, "port", "", "Port to listen on (required, no default)")
	flag.StringVar(&networkInterface, "interface", "", "Network interface to bind to (required, no default)")
	flag.Parse()

	// Validate required flags
	if port == "" {
		log.Fatal("Error: --port flag is required")
	}
	if networkInterface == "" {
		log.Fatal("Error: --interface flag is required")
	}

	if err := runListener(port, networkInterface, useSharedSecret); err != nil {
		log.Fatal(err)
	}
}

func runListener(port, networkInterface string, useSharedSecret bool) error {
	printHeader()

	// Load configuration with defaults and environment overrides
	cfg, err := config.LoadServerConfig(port, networkInterface, useSharedSecret)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	log.Println("Generating self-signed certificate...")
	cert, fingerprint, err := certs.GenerateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("failed to generate certificate: %w", err)
	}

	log.Printf("Certificate generated successfully (SHA256: %s)", fingerprint)

	var secret string
	if cfg.SharedSecretAuth {
		secret, err = certs.GenerateSecret()
		if err != nil {
			return fmt.Errorf("failed to generate shared secret: %w", err)
		}
		log.Printf("✓ Shared secret authentication enabled")
		log.Printf("Secret (hex): %s", secret)
		log.Printf("\nTo connect, use:")
		log.Printf("  gotsr -s %s --cert-fingerprint %s %s:%s <max-retries>\n", secret, fingerprint, cfg.NetworkInterface, cfg.Port)
	}

	log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)
	log.Printf("Configuration: port=%s, interface=%s", cfg.Port, cfg.NetworkInterface)

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create listener with configuration
	listener := server.NewListener(cfg.Port, cfg.NetworkInterface, tlsConfig, secret)
	netListener, err := listener.Start()
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	defer netListener.Close()

	log.Println("Listener ready. Waiting for connections...")
	interactiveShell(listener)
	return nil
}

func interactiveShell(l server.ListenerInterface) {
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
		case "help":
			printHelp()
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
		case "forward":
			if len(parts) < 2 {
				fmt.Println("Usage: forward <client_id> <local_port> <remote_addr>")
				fmt.Println("Example: forward 1 8080 10.0.0.5:80")
				continue
			}
			if len(parts) != 4 {
				fmt.Println("Usage: forward <client_id> <local_port> <remote_addr>")
				continue
			}
			clientAddr := getClientByID(l, parts[1])
			if clientAddr == "" {
				continue
			}
			handleForward(l, clientAddr, parts[2], parts[3])
		case "forwards":
			listForwards(l)
		case "socks":
				// If no args: list active SOCKS proxies
				if len(parts) == 1 {
					listSocks(l)
					continue
				}
				// Expect: socks <client_id> <local_port>
				if len(parts) != 3 {
					fmt.Println("Usage: socks <client_id> <local_port>")
					fmt.Println("Example: socks 1 1080")
					continue
				}
				clientAddr := getClientByID(l, parts[1])
				if clientAddr == "" {
					continue
				}
				handleSocks(l, clientAddr, parts[2])
		case "stop":
			if len(parts) < 2 {
				fmt.Println("Usage: stop forward <id> | stop socks <id>")
				continue
			}
			if len(parts) != 3 {
				fmt.Println("Usage: stop forward <id> | stop socks <id>")
				continue
			}
			handleStop(l, parts[1], parts[2])
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
	fmt.Println("  forward <id> <local_port> <remote_addr> - Forward local port to remote address through client")
	fmt.Println("  forwards                    - List active port forwards")
	fmt.Println("  socks                       - List active SOCKS5 proxies")
	fmt.Println("  socks <id> <local_port>     - Start SOCKS5 proxy on local port through client")
	fmt.Println("  stop forward <id>           - Stop a port forward by ID")
	fmt.Println("  stop socks <id>             - Stop a SOCKS5 proxy by ID")
	fmt.Println("  exit                        - Exit the listener")
	fmt.Println()
	fmt.Println("In PTY shell mode:")
	fmt.Println("  Ctrl-D                      - Return to listener prompt")
	fmt.Println("  Ctrl-C                      - Send interrupt signal to remote shell")
	fmt.Println()
}

func listClients(l server.ListenerInterface) {
	clients := l.GetClients()
	if len(clients) == 0 {
		fmt.Println("No clients connected")
	} else {
		fmt.Println("\nConnected Clients:")
		for i, addr := range clients {
			ident := l.GetClientIdentifier(addr)
			suffix := " [no-id]"
			if ident != "" {
				suffix = " [" + ident + "]"
			}
			fmt.Printf("  %d. %s%s\n", i+1, addr, suffix)
		}
		fmt.Println()
	}
}

func getClientByID(l server.ListenerInterface, idStr string) string {
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

func handleUploadGlobal(l server.ListenerInterface, currentClient, localPath, remotePath string) bool {
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

func handleDownloadGlobal(l server.ListenerInterface, currentClient, remotePath, localPath string) bool {
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

func enterPtyShell(l server.ListenerInterface, clientAddr string) {
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
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Printf("Warning: Could not set raw mode: %v\n", err)
		// Continue anyway
	}
	defer func() {
		// Clear any read deadlines on stdin
		os.Stdin.SetReadDeadline(time.Time{})

		// Restore terminal state BEFORE disabling features
		// This ensures the terminal is in cooked mode when we send the disable sequences
		if oldState != nil {
			term.Restore(fd, oldState)
		}
		
		// Now disable terminal features that may have been enabled by the remote PTY
		// Send these in cooked mode so the terminal processes them correctly
		// - Disable mouse tracking (all modes)
		// - Disable focus events  
		// - Reset bracketed paste mode
		os.Stdout.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l\x1b[?2004l\x1b[?1004l")
		os.Stdout.Sync()
		
		// Flush stdin only if it's a real terminal (not a pipe/test)
		// This consumes any pending input like terminal response escape sequences
		if term.IsTerminal(fd) {
			// Use a goroutine with timeout to avoid indefinite blocking
			done := make(chan struct{})
			go func() {
				defer close(done)
				discardBuf := make([]byte, 4096)
				os.Stdin.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
				for {
					n, err := os.Stdin.Read(discardBuf)
					if n == 0 || err != nil {
						break
					}
				}
			}()
			select {
			case <-done:
			case <-time.After(20 * time.Millisecond):
			}
			os.Stdin.SetReadDeadline(time.Time{})
			
			// Also flush using platform-specific method if available
			if err := flushStdin(); err != nil {
				log.Printf("Warning: failed to flush stdin after PTY exit: %v", err)
			}
		}

		// Force a newline to reset the terminal display
		fmt.Println()
	}()

	// Channel to signal we should exit (closed channel broadcasts to all goroutines)
	exitPty := make(chan struct{})

	// Track which goroutine triggered the exit to avoid double-closing
	var exitOnce sync.Once

	// WaitGroup to ensure both goroutines finish before exiting
	var wg sync.WaitGroup
	wg.Add(2) // For output and stdin goroutines

	// Forward PTY output to stdout
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in PTY output goroutine: %v", r)
			}
		}()

		for {
			data, ok := <-ptyDataChan
			if !ok {
				// Channel closed - remote PTY exited
				fmt.Printf("\r\n[Remote shell exited]\r\n")
				exitOnce.Do(func() {
					close(exitPty) // Broadcast exit to all goroutines
				})
				return
			}
			os.Stdout.Write(data)
		}
	}()

	// Read from stdin and forward to PTY
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in PTY stdin goroutine: %v", r)
			}
			// Ensure deadline is cleared when goroutine exits
			os.Stdin.SetReadDeadline(time.Time{})
		}()

		stdinBuf := make([]byte, 1024)

		for {
			// Check if we should exit
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
					// Timeout, check if we should exit in next iteration
					continue
				}
				// EOF or error - exit gracefully
				return
			}

			if n > 0 {
				data := stdinBuf[:n]

				// Check for Ctrl-D (EOF)
				if strings.Contains(string(data), "\x04") {
					exitOnce.Do(func() {
						close(exitPty)
					})
					return
				}

				// **CRITICAL**: Double-check before sending in case remote just exited
				select {
				case <-exitPty:
					return
				default:
				}

				// Send data immediately to PTY
				encoded, err := compression.CompressToHex(data)
				if err != nil {
					fmt.Printf("\nError encoding input: %v\n", err)
					return
				}

				// Send command without blocking on response
				if err := l.SendCommand(clientAddr, protocol.CmdPtyData+" "+encoded); err != nil {
					log.Printf("Failed to send PTY data (client disconnected): %v", err)
					return
				}
			}
		}
	}()

	// Wait for exit signal
	<-exitPty

	// Force any blocking stdin read to unblock immediately
	_ = os.Stdin.SetReadDeadline(time.Now())

	// Exit PTY mode (sending PTY_EXIT but not waiting for response - client might have already exited)
	fmt.Println("\nExiting PTY shell... (Press Enter to return to prompt)")
	_ = l.SendCommand(clientAddr, protocol.CmdPtyExit)
	l.ExitPtyMode(clientAddr)

	// Wait for both goroutines to fully finish before returning
	wg.Wait()
}

func handleForward(l server.ListenerInterface, clientAddr, localPort, remoteAddr string) {
	// Generate unique forward ID
	fwdID := fmt.Sprintf("fwd-%d", time.Now().UnixNano())
	
	// Get access to the forward manager (via type assertion)
	if listener, ok := l.(*server.Listener); ok {
		// Create send function for this client
		sendFunc := func(msg string) {
			_ = l.SendCommand(clientAddr, msg)
		}
		
		err := listener.GetForwardManager().StartForward(fwdID, localPort, remoteAddr, sendFunc)
		if err != nil {
			fmt.Printf("Failed to start forward: %v\n", err)
			return
		}
		
		fmt.Printf("✓ Port forward started: 127.0.0.1:%s -> %s (via %s)\n", localPort, remoteAddr, clientAddr)
		fmt.Printf("  Forward ID: %s\n", fwdID)
	} else {
		fmt.Println("Error: could not access forward manager")
	}
}

func listForwards(l server.ListenerInterface) {
	if listener, ok := l.(*server.Listener); ok {
		forwards := listener.GetForwardManager().ListForwards()
		if len(forwards) == 0 {
			fmt.Println("No active port forwards")
		} else {
			fmt.Println("\nActive Port Forwards:")
			for i, fwd := range forwards {
				fmt.Printf("  %d. %s -> %s (ID: %s)\n", i+1, fwd.LocalAddr, fwd.RemoteAddr, fwd.ID)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("Error: could not access forward manager")
	}
}

func listSocks(l server.ListenerInterface) {
	if listener, ok := l.(*server.Listener); ok {
		proxies := listener.GetSocksManager().ListSocks()
		if len(proxies) == 0 {
			fmt.Println("No active SOCKS proxies")
		} else {
			fmt.Println("\nActive SOCKS Proxies:")
			for i, p := range proxies {
				fmt.Printf("  %d. %s (ID: %s)\n", i+1, p.LocalAddr, p.ID)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("Error: could not access SOCKS manager")
	}
}

func handleSocks(l server.ListenerInterface, clientAddr, localPort string) {
	// Generate unique SOCKS ID
	socksID := fmt.Sprintf("socks-%d", time.Now().UnixNano())
	
	// Get access to the SOCKS manager (via type assertion)
	if listener, ok := l.(*server.Listener); ok {
		// Create send function for this client
		sendFunc := func(msg string) {
			_ = l.SendCommand(clientAddr, msg)
		}
		
		err := listener.GetSocksManager().StartSocks(socksID, localPort, sendFunc)
		if err != nil {
			fmt.Printf("Failed to start SOCKS proxy: %v\n", err)
			return
		}
		
		fmt.Printf("✓ SOCKS5 proxy started on 127.0.0.1:%s (via %s)\n", localPort, clientAddr)
		fmt.Printf("  SOCKS ID: %s\n", socksID)
		fmt.Printf("  Configure your browser/app to use SOCKS5 proxy at 127.0.0.1:%s\n", localPort)
	} else {
		fmt.Println("Error: could not access SOCKS manager")
	}
}

func handleStop(l server.ListenerInterface, stopType, id string) {
	if listener, ok := l.(*server.Listener); ok {
		switch stopType {
		case "forward":
			err := listener.GetForwardManager().StopForward(id)
			if err != nil {
				fmt.Printf("Failed to stop forward: %v\n", err)
			} else {
				fmt.Printf("✓ Stopped port forward %s\n", id)
			}
		case "socks":
			err := listener.GetSocksManager().StopSocks(id)
			if err != nil {
				fmt.Printf("Failed to stop SOCKS proxy: %v\n", err)
			} else {
				fmt.Printf("✓ Stopped SOCKS proxy %s\n", id)
			}
		default:
			fmt.Printf("Unknown stop type: %s (use 'forward' or 'socks')\n", stopType)
		}
	} else {
		fmt.Println("Error: could not access managers")
	}
}
