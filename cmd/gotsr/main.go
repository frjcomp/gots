package main

import (
"flag"
"fmt"
"log"
"time"

"github.com/frjcomp/gots/pkg/client"
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
flag.StringVar(&sharedSecret, "s", "", "Shared secret for authentication")
flag.StringVar(&sharedSecret, "shared-secret", "", "Shared secret for authentication")
flag.StringVar(&certFingerprint, "cert-fingerprint", "", "Expected server certificate SHA256 fingerprint")
flag.Parse()

args := flag.Args()
if err := runClient(args, sharedSecret, certFingerprint); err != nil {
log.Fatal(err)
}
}

func runClient(args []string, sharedSecret, certFingerprint string) error {
printHeader()

if len(args) != 2 {
return fmt.Errorf("Usage: gotsr [-s|--shared-secret secret] [--cert-fingerprint fingerprint] <host:port|domain:port> <max-retries>\n  secret: 64 hex characters (32 bytes) or leave empty for no authentication")
}

target := args[0]
maxRetries := 0
fmt.Sscanf(args[1], "%d", &maxRetries)

// Validate shared secret length if provided
if sharedSecret != "" && len(sharedSecret) != 64 {
return fmt.Errorf("invalid secret length: got %d characters, expected 64 (32 bytes hex-encoded)", len(sharedSecret))
}

log.Printf("Starting GOTS - PIPELEEK client...")
log.Printf("Version: %s (commit %s, date %s)", version.Version, version.Commit, version.Date)
log.Printf("Target: %s", target)
log.Printf("Max retries: %d (0 = infinite)", maxRetries)
if sharedSecret != "" {
log.Printf("Shared secret authentication: enabled")
}
if certFingerprint != "" {
log.Printf("Certificate fingerprint validation: enabled")
}

connectWithRetry(target, maxRetries, sharedSecret, certFingerprint, func(t, s, f string) reverseClient { 
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
