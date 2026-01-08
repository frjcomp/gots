package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// TestSocksProxyEndToEnd spins listener+reverse, starts a SOCKS proxy, and fetches from a local HTTP server through it.
func TestSocksProxyEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	httpSrv := newLocalHTTPServer(t, "socks-ok")
	socksPort := freePort(t)
	listenPort := freePort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, "--port", listenPort, "--interface", "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, "--target", "127.0.0.1:"+listenPort, "--retries", "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	send(listener, "ls\n")
	waitForContains(t, listener, "Connected Clients:", 5*time.Second)

	send(listener, "socks 1 "+socksPort+"\n")
	waitForContains(t, listener, "SOCKS5 proxy started", 5*time.Second)

	// Build SOCKS5 HTTP client
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+socksPort, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("create socks5 dialer: %v", err)
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// Retry a few times in case the remote side is still wiring up
	var respBody string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + httpSrv)
		if err == nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			respBody = string(bodyBytes)
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if respBody != "socks-ok" {
		t.Fatalf("unexpected response via SOCKS: %q", respBody)
	}
}

// TestForwardingEndToEnd starts a port forward through the client and calls a local HTTP server through it.
func TestForwardingEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	httpSrv := newLocalHTTPServer(t, "fwd-ok")
	forwardPort := freePort(t)
	listenPort := freePort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	listenerBin := buildBinary(t, "gotsl", "./cmd/gotsl")
	reverseBin := buildBinary(t, "gotsr", "./cmd/gotsr")

	listener := startProcess(ctx, t, listenerBin, "--port", listenPort, "--interface", "127.0.0.1")
	t.Cleanup(listener.stop)
	waitForContains(t, listener, "Listener ready. Waiting for connections", 10*time.Second)

	reverse := startProcess(ctx, t, reverseBin, "--target", "127.0.0.1:"+listenPort, "--retries", "1")
	t.Cleanup(reverse.stop)
	waitForContains(t, reverse, "Connected to listener successfully", 10*time.Second)

	send(listener, "forward 1 "+forwardPort+" "+httpSrv+"\n")
	waitForContains(t, listener, "Port forward started", 5*time.Second)

	client := &http.Client{Timeout: 10 * time.Second}
	url := "http://127.0.0.1:" + forwardPort

	var respBody string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			respBody = string(bodyBytes)
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if respBody != "fwd-ok" {
		t.Fatalf("unexpected response via forward: %q", respBody)
	}
}

// newLocalHTTPServer starts a simple HTTP server that returns the provided body on any request.
func newLocalHTTPServer(t *testing.T, body string) string {
	t.Helper()
	port := freePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	srv := &http.Server{
		Addr:         "127.0.0.1:" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		_ = srv.ListenAndServe()
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})

	return "127.0.0.1:" + port
}
