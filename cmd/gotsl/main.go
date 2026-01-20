package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/frjcomp/gots/pkg/certs"
	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/config"
	"github.com/frjcomp/gots/pkg/console"
	"github.com/frjcomp/gots/pkg/logging"
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
	var logLevel string
	var quiet bool
	var headless bool
	var controlAddr string

	flag.BoolVar(&useSharedSecret, "s", false, "Enable shared secret authentication")
	flag.BoolVar(&useSharedSecret, "shared-secret", false, "Enable shared secret authentication")
	flag.StringVar(&port, "port", "", "Port to listen on (required, no default)")
	flag.StringVar(&networkInterface, "interface", "", "Network interface to bind to (required, no default)")
	flag.StringVar(&logLevel, "log-level", "", "Log level: error|warn|info|debug (default info)")
	flag.BoolVar(&quiet, "quiet", false, "Reduce logs to errors only (overrides log-level)")
	flag.BoolVar(&headless, "headless", false, "[SECURITY] Run without interactive shell and expose HTTP control API (disabled by default)")
	flag.StringVar(&controlAddr, "control-addr", "127.0.0.1:0", "Headless control listen address (host:port, localhost only by default for security)")
	flag.Parse()

	// Initialize logging from env, then apply flags if provided
	logging.InitFromEnv()
	if logLevel != "" {
		logging.SetLevelFromString(logLevel)
	}
	if quiet {
		logging.SetQuiet(true)
	}

	// Validate required flags
	if port == "" {
		log.Fatal("Error: --port flag is required")
	}
	if networkInterface == "" {
		log.Fatal("Error: --interface flag is required")
	}

	if err := runListener(port, networkInterface, useSharedSecret, headless, controlAddr); err != nil {
		log.Fatal(err)
	}
}

func runListener(port, networkInterface string, useSharedSecret, headless bool, controlAddr string) error {
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
		// Use provided secret if set, otherwise generate random one
		if cfg.SharedSecret != "" {
			secret = cfg.SharedSecret
			log.Printf("✓ Shared secret authentication enabled (using provided secret)")
		} else {
			var err error
			secret, err = certs.GenerateSecret()
			if err != nil {
				return fmt.Errorf("failed to generate shared secret: %w", err)
			}
			log.Printf("✓ Shared secret authentication enabled")
			log.Printf("Secret (hex): %s", secret)
			log.Printf("\nTo connect, use:")
			log.Printf("  gotsr -s %s --cert-fingerprint %s %s:%s <max-retries>\n", secret, fingerprint, cfg.NetworkInterface, cfg.Port)
		}
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

	if headless {
		log.Println("⚠️  WARNING: Headless control API enabled - this exposes full system control over HTTP")
		log.Println("⚠️  Ensure this endpoint is only accessible from trusted networks")

		hs, err := newHeadlessServer(listener, controlAddr)
		if err != nil {
			return err
		}

		// Allow Ctrl+C / SIGTERM to stop the headless server cleanly
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			_ = hs.Stop(context.Background())
		}()

		hs.Start()
		log.Printf("Headless control API listening on %s", hs.Addr())
		<-hs.Wait()
		return nil
	}

	// Redirect subsequent logs to avoid interfering with readline
	logRedirector := newLogRedirector()
	log.SetOutput(logRedirector)

	arbiter, err := console.NewSessionArbiter(log.Printf)
	if err != nil {
		log.Printf("Warning: failed to initialize TTY arbiter: %v", err)
	}

	interactiveShell(listener, logRedirector, arbiter)
	return nil
}

func interactiveShell(l server.ListenerInterface, logRedirector *logRedirector, arbiter *console.SessionArbiter) {
	if arbiter == nil {
		if arb, err := console.NewSessionArbiter(log.Printf); err == nil {
			arbiter = arb
		}
	}
	if arbiter != nil {
		defer arbiter.Close()
		_ = arbiter.EnterShell()
	}

	// Check if stdin is a TTY; if not, use basic shell for compatibility with tests
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		interactiveShellBasic(l, arbiter)
		return
	}

	// Use a pointer to track if we need to recreate readline
	var rl *readline.Instance
	var needsReinitialize bool = true
	var ctrlCCount int = 0
	var ttyFile *os.File // Fresh /dev/tty handle to avoid stale stdin state
	defer func() {
		if ttyFile != nil {
			ttyFile.Close()
		}
	}()

	for {
		// Recreate readline if needed (e.g., after PTY exit corrupts state)
		if needsReinitialize {
			if arbiter != nil {
				_ = arbiter.EnterShell()
			}
			if rl != nil {
				rl.Close()
			}
			if ttyFile != nil {
				ttyFile.Close()
				ttyFile = nil
			}

			// Open a fresh TTY handle to avoid reusing a potentially stale stdin FD
			// after returning from PTY mode. If /dev/tty is unavailable (e.g., Windows
			// host or rare container setup), fall back to the process stdio.
			cfg := &readline.Config{
				Prompt:          "\033[32mgotsl>\033[0m ",
				HistoryFile:     "/tmp/.gotsl_history",
				AutoComplete:    &shellCompleter{listener: l},
				InterruptPrompt: "^C",
				EOFPrompt:       "exit",
			}
			if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
				ttyFile = f
				cfg.Stdin = f
				cfg.Stdout = f
			}

			var err error
			rl, err = readline.NewEx(cfg)
			if err != nil {
				log.Printf("Warning: readline initialization failed, using basic input: %v", err)
				if ttyFile != nil {
					ttyFile.Close()
				}
				interactiveShellBasic(l, arbiter)
				return
			}

			// Set readline instance for log redirector
			logRedirector.setReadline(rl)

			// Set up Ctrl+L handler for clearing screen and refreshing readline state
			rl.Config.FuncFilterInputRune = func(r rune) (rune, bool) {
				// Intercept Ctrl+L (0x0C) to refresh terminal and readline state
				if r == 12 { // Ctrl+L
					// Clear screen using ANSI escape sequence
					fmt.Print("\033[H\033[2J")
					// Refresh readline to resynchronize with terminal state
					rl.Refresh()
					return 0, false // Don't process this character
				}
				return r, true
			}

			printHelp()
			needsReinitialize = false
		}

		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				ctrlCCount++
				if ctrlCCount == 1 {
					fmt.Println("\nPress Ctrl+C again to exit, or type a command to continue")
					continue
				}
				rl.Close()
				return
			}
			if err == io.EOF {
				rl.Close()
				return
			}
			rl.Close()
			return
		}

		// Reset Ctrl+C counter on successful input
		ctrlCCount = 0

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
			enterPtyShell(l, clientAddr, arbiter)
			// After PTY exit, signal that we need to recreate readline
			// This cleanly recovers from any terminal state corruption
			needsReinitialize = true
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
			// Validate remote address format (must be host:port)
			if !strings.Contains(parts[3], ":") {
				fmt.Println("Error: remote address must include port (format: host:port)")
				fmt.Println("Example: forward 1 8080 10.0.0.5:80")
				fmt.Println("         forward 1 8080 127.0.0.1:8080")
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

// interactiveShellBasic is a fallback when readline is not available
func interactiveShellBasic(l server.ListenerInterface, arbiter *console.SessionArbiter) {
	reader := bufio.NewReader(os.Stdin)

	printHelp()

	for {
		fmt.Print("gotsl> ")
		line, err := reader.ReadString('\n')
		if err != nil {
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
			enterPtyShell(l, clientAddr, arbiter)
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
			if !strings.Contains(parts[3], ":") {
				fmt.Println("Error: remote address must include port (format: host:port)")
				fmt.Println("Example: forward 1 8080 10.0.0.5:80")
				fmt.Println("         forward 1 8080 127.0.0.1:8080")
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
			if len(parts) == 1 {
				listSocks(l)
				continue
			}
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
	fmt.Println("Keyboard shortcuts:")
	fmt.Println("  Ctrl-L                      - Clear screen and refresh input (use if keyboard is stuck)")
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
			meta, _ := l.GetClientMetadata(addr)
			suffix := " [no-id]"
			if ident != "" {
				suffix = " [" + ident + "]"
			}
			metaParts := make([]string, 0, 3)
			if meta.OS != "" {
				metaParts = append(metaParts, "os="+meta.OS)
			}
			if meta.Hostname != "" {
				metaParts = append(metaParts, "host="+meta.Hostname)
			}
			if meta.IP != "" {
				metaParts = append(metaParts, "ip="+meta.IP)
			}
			metaSuffix := ""
			if len(metaParts) > 0 {
				metaSuffix = " (" + strings.Join(metaParts, ", ") + ")"
			}
			fmt.Printf("  %d. %s%s%s\n", i+1, addr, suffix, metaSuffix)
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

func enterPtyShell(l server.ListenerInterface, clientAddr string, arbiter *console.SessionArbiter) {
	fmt.Printf("Entering PTY shell with %s...\n", clientAddr)

	if err := l.SendCommand(clientAddr, protocol.CmdPtyMode); err != nil {
		fmt.Printf("Error entering PTY mode: %v\n", err)
		return
	}

	resp, err := l.GetResponse(clientAddr, 10*time.Second)
	if err != nil {
		fmt.Printf("Error getting PTY mode confirmation: %v\n", err)
		return
	}
	if !strings.Contains(resp, "OK") {
		fmt.Printf("Failed to enter PTY mode: %s\n", strings.ReplaceAll(resp, protocol.EndOfOutputMarker, ""))
		return
	}

	ptyDataChan, err := l.EnterPtyMode(clientAddr)
	if err != nil {
		fmt.Printf("Error creating PTY data channel: %v\n", err)
		return
	}

	fmt.Println("PTY shell active. Press Ctrl-D to return to listener prompt.")
	fmt.Println("Press Ctrl-C to send interrupt to remote shell.")

	if arbiter == nil {
		fmt.Println("TTY arbiter unavailable; aborting PTY session")
		_ = l.SendCommand(clientAddr, protocol.CmdPtyExit)
		l.ExitPtyMode(clientAddr)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sendFn := func(data []byte) error {
		// Check for Ctrl-D which signals exit
		for _, b := range data {
			if b == 0x04 {
				cancel()
				return nil
			}
		}
		encoded, err := compression.CompressToHex(data)
		if err != nil {
			return err
		}
		return l.SendCommand(clientAddr, protocol.CmdPtyData+" "+encoded)
	}

	sendExit := func() error {
		return l.SendCommand(clientAddr, protocol.CmdPtyExit)
	}

	cfg := console.PtySessionConfig{
		Incoming:          ptyDataChan,
		Send:              sendFn,
		SendExit:          sendExit,
		ReadDeadline:      120 * time.Millisecond,
		HeartbeatInterval: 12 * time.Second,
		BackpressureLimit: cap(ptyDataChan) + 64,
	}

	err = arbiter.RunPtySession(ctx, cfg)
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		fmt.Printf("PTY session ended: %v\n", err)
	}

	l.ExitPtyMode(clientAddr)
	_ = arbiter.EnterShell()
	fmt.Println("gotsl> ")
}

// deadlineReader is the minimal interface needed to drain pending input with deadlines.
type deadlineReader interface {
	Read([]byte) (int, error)
	SetReadDeadline(time.Time) error
}

// shellCompleter provides tab completion for the interactive shell
type shellCompleter struct {
	listener server.ListenerInterface
}

func (c *shellCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	// Get the current line up to cursor position
	lineStr := string(line[:pos])
	parts := strings.Fields(lineStr)

	// List of all available commands
	commands := []string{
		"ls", "dir", "help", "shell", "upload", "download",
		"forward", "forwards", "socks", "stop", "exit",
	}

	// If we're at the start or only have partial first word, complete commands
	if len(parts) == 0 || (len(parts) == 1 && !strings.HasSuffix(lineStr, " ")) {
		prefix := ""
		if len(parts) == 1 {
			prefix = parts[0]
		}

		var suggestions [][]rune
		for _, cmd := range commands {
			if strings.HasPrefix(cmd, prefix) {
				suggestions = append(suggestions, []rune(cmd[len(prefix):]))
			}
		}
		return suggestions, len(prefix)
	}

	// For commands that need client ID, complete with client numbers
	if len(parts) >= 1 {
		cmd := parts[0]
		needsClientID := cmd == "shell" || cmd == "upload" || cmd == "download" ||
			cmd == "forward" || cmd == "socks"

		if needsClientID && (len(parts) == 1 || (len(parts) == 2 && !strings.HasSuffix(lineStr, " "))) {
			// Complete client IDs
			clients := c.listener.GetClients()
			var suggestions [][]rune
			prefix := ""
			if len(parts) == 2 {
				prefix = parts[1]
			}

			for i := range clients {
				clientID := fmt.Sprintf("%d", i+1)
				if strings.HasPrefix(clientID, prefix) {
					suggestions = append(suggestions, []rune(clientID[len(prefix):]))
				}
			}
			return suggestions, len(prefix)
		}

		// For "stop" command, complete with "forward" or "socks"
		if cmd == "stop" && (len(parts) == 1 || (len(parts) == 2 && !strings.HasSuffix(lineStr, " "))) {
			stopTargets := []string{"forward", "socks"}
			prefix := ""
			if len(parts) == 2 {
				prefix = parts[1]
			}

			var suggestions [][]rune
			for _, target := range stopTargets {
				if strings.HasPrefix(target, prefix) {
					suggestions = append(suggestions, []rune(target[len(prefix):]))
				}
			}
			return suggestions, len(prefix)
		}
	}

	return nil, 0
}

// logRedirector captures log output and writes it above the readline prompt
type logRedirector struct {
	rl  *readline.Instance
	buf []byte
	mu  sync.Mutex
}

func newLogRedirector() *logRedirector {
	return &logRedirector{
		buf: make([]byte, 0, 1024),
	}
}

func (lr *logRedirector) setReadline(rl *readline.Instance) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	lr.rl = rl
}

func (lr *logRedirector) Write(p []byte) (n int, err error) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.rl != nil {
		// Use readline's output mechanism to print above the prompt
		_, err = lr.rl.Stdout().Write(p)
		return len(p), err
	}

	// Fallback to os.Stderr if readline not initialized yet
	return os.Stderr.Write(p)
}

// drainPendingInput consumes any pending stdin bytes without blocking indefinitely.
// If deadlines are unsupported, it returns immediately to avoid stealing user input.
func drainPendingInput(r deadlineReader) {
	if r == nil {
		return
	}

	if err := r.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
		return
	}
	defer r.SetReadDeadline(time.Time{})

	buf := make([]byte, 4096)
	deadline := time.Now().Add(20 * time.Millisecond)

	for {
		n, err := r.Read(buf)
		if n == 0 || err != nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		if err := r.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
			return
		}
	}
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

		bindAddr := listener.GetForwardManager().BindAddr()
		fmt.Printf("✓ Port forward started: %s:%s -> %s (via %s)\n", bindAddr, localPort, remoteAddr, clientAddr)
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

		bindAddr := listener.GetSocksManager().BindAddr()
		fmt.Printf("✓ SOCKS5 proxy started on %s:%s (via %s)\n", bindAddr, localPort, clientAddr)
		fmt.Printf("  SOCKS ID: %s\n", socksID)
		fmt.Printf("  Configure your browser/app to use SOCKS5 proxy at %s:%s\n", bindAddr, localPort)
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
