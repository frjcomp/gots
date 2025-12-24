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
	flag.StringVar(&target, "target", "", "Target server (host:port)")
	flag.StringVar(&maxRetriesStr, "retries", "", "Maximum number of retries (0 = infinite)")
	flag.Parse()

	args := flag.Args()
	// For backward compatibility, accept positional args
	if len(args) > 0 {
		target = args[0]
	}
	if len(args) > 1 {
		maxRetriesStr = args[1]
	}

	maxRetries := 0
	if maxRetriesStr != "" {
		fmt.Sscanf(maxRetriesStr, "%d", &maxRetries)
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

	connectWithRetry(cfg.Target, cfg.MaxRetries, cfg.SharedSecret, cfg.CertFingerprint, func(t, s, f string) reverseClient {
		return client.NewReverseClient(t, s, f)
	}, time.Sleep)
	return nil
}

type reverseClient interface {
	Connect() error
	HandleCommands() error
	Close() error
}

type clientFactory func(target, sharedSecret, certFingerprint string) reverseClient

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
		}
	}
}
