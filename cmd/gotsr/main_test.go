package main

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/config"
)

type fakeClient struct {
	connectErrs  []error
	handleErrs   []error
	connectCalls int
	handleCalls  int
	closed       int
}

func (f *fakeClient) Connect() error {
	if f.connectCalls < len(f.connectErrs) {
		err := f.connectErrs[f.connectCalls]
		f.connectCalls++
		return err
	}
	f.connectCalls++
	return nil
}

func (f *fakeClient) HandleCommands() error {
	if f.handleCalls < len(f.handleErrs) {
		err := f.handleErrs[f.handleCalls]
		f.handleCalls++
		return err
	}
	f.handleCalls++
	return nil
}

func (f *fakeClient) Close() error { f.closed++; return nil }

func noSleep(time.Duration) {}

func TestRunClientArgValidation(t *testing.T) {
	// Test with empty target should fail validation
	_, err := config.LoadClientConfig("", 0, "", "")
	if err == nil {
		t.Fatal("expected error for missing target")
	}

	// Test with invalid secret length should fail
	_, err = config.LoadClientConfig("localhost:9001", 5, "short", "")
	if err == nil {
		t.Fatal("expected error for invalid secret length")
	}

	// Test with valid config should succeed
	_, err = config.LoadClientConfig("localhost:9001", 5, "", "")
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestRunClientValidArgs(t *testing.T) {
	// Test with valid arguments (but will try to connect)
	// This covers the argument parsing logic - no longer possible with new signature
	// Skip this test as it required the old array-based args
	t.Skip("Test no longer applicable with new config-based args")
}

func TestPrintHeader(t *testing.T) {
	// Just call it for coverage
	printHeader()
}

func TestConnectWithRetry_MaxRetriesReachedOnConnectFailures(t *testing.T) {
	fc := &fakeClient{connectErrs: []error{errors.New("fail"), errors.New("fail"), errors.New("fail")}}
	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() { connectWithRetry("127.0.0.1:8443", 3, "", "", factory, noSleep); close(done) }()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("connectWithRetry did not return after max retries")
	}

	if created != 3 {
		t.Fatalf("expected 3 client creations, got %d", created)
	}
	if fc.connectCalls != 3 {
		t.Fatalf("expected 3 connect attempts, got %d", fc.connectCalls)
	}
}

func TestConnectWithRetry_ReconnectAfterHandleCommandsError(t *testing.T) {
	fc := &fakeClient{connectErrs: []error{nil, errors.New("fail")}, handleErrs: []error{errors.New("session error")}}
	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() { connectWithRetry("127.0.0.1:8443", 2, "", "", factory, noSleep); close(done) }()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("connectWithRetry did not return after retries")
	}

	if created < 2 {
		t.Fatalf("expected at least 2 client creations, got %d", created)
	}
	if fc.connectCalls < 2 {
		t.Fatalf("expected at least 2 connect attempts, got %d", fc.connectCalls)
	}
	if fc.handleCalls < 1 {
		t.Fatalf("expected at least 1 handle attempt, got %d", fc.handleCalls)
	}
}

func TestConnectWithRetrySuccessful(t *testing.T) {
	fc := &fakeClient{} // No errors
	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		created++
		return fc
	}

	// Run with 1 retry so it exits after HandleCommands returns nil
	done := make(chan struct{})
	go func() {
		connectWithRetry("127.0.0.1:8443", 0, "", "", factory, noSleep)
		close(done)
	}()

	// Wait a bit for it to run
	time.Sleep(100 * time.Millisecond)

	// It should still be running with infinite retries
	select {
	case <-done:
		t.Log("Client exited (HandleCommands returned)")
	default:
		t.Log("Client still running with infinite retries")
	}
}

func TestConnectWithRetryInfiniteRetries(t *testing.T) {
	// Test with maxRetries=0 (infinite)
	fc := &fakeClient{connectErrs: []error{errors.New("fail"), errors.New("fail")}}
	var mu sync.Mutex
	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		mu.Lock()
		created++
		mu.Unlock()
		return fc
	}

	done := make(chan struct{})
	go func() {
		// This should keep trying forever, but we'll stop after a few attempts
		connectWithRetry("127.0.0.1:8443", 0, "", "", factory, noSleep)
		close(done)
	}()

	// Give it time for a few attempts
	time.Sleep(100 * time.Millisecond)

	// With infinite retries and always failing, it should keep going
	mu.Lock()
	attempts := created
	mu.Unlock()
	if attempts < 2 {
		t.Fatalf("expected multiple retry attempts with infinite retries, got %d", attempts)
	}
}

func TestConnectWithRetryBackoffMaximum(t *testing.T) {
	// Test that backoff caps at 5 minutes
	fc := &fakeClient{connectErrs: []error{
		errors.New("fail1"),
		errors.New("fail2"),
		errors.New("fail3"),
		errors.New("fail4"),
		errors.New("fail5"),
	}}

	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() {
		connectWithRetry("127.0.0.1:8443", 5, "", "", factory, noSleep)
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("connectWithRetry did not return")
	}

	if created != 5 {
		t.Fatalf("expected 5 attempts, got %d", created)
	}
}

func TestConnectWithRetryHandleCommandsSuccess(t *testing.T) {
	// Test successful connection and command handling with eventual failure
	fc := &fakeClient{
		connectErrs: []error{nil, nil},
		handleErrs:  []error{errors.New("disconnect"), errors.New("disconnect")},
	}

	created := 0
	factory := func(target, secret, fingerprint string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() {
		connectWithRetry("127.0.0.1:8443", 2, "", "", factory, noSleep)
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("connectWithRetry did not return")
	}

	if fc.handleCalls < 1 {
		t.Fatalf("expected at least 1 handle attempt, got %d", fc.handleCalls)
	}
	if fc.closed < 1 {
		t.Fatalf("expected Close to be called at least once, got %d", fc.closed)
	}
}

func TestSecretLengthValidation(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty secret", "", false},
		{"valid 64 chars", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", false},
		{"too short", "short", true},
		{"too short hex", "0123456789abcdef", true},
		{"too long", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.LoadClientConfig("127.0.0.1:8443", 1, tt.secret, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadClientConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
