package main

import (
	"errors"
	"testing"
	"time"
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
	if err := runClient([]string{}); err == nil {
		t.Fatal("expected error for missing args")
	}
	if err := runClient([]string{"127.0.0.1:8443"}); err == nil {
		t.Fatal("expected error for too few args")
	}
}

func TestConnectWithRetry_MaxRetriesReachedOnConnectFailures(t *testing.T) {
	fc := &fakeClient{connectErrs: []error{errors.New("fail"), errors.New("fail"), errors.New("fail")}}
	created := 0
	factory := func(target string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() { connectWithRetry("127.0.0.1:8443", 3, factory, noSleep); close(done) }()

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
	factory := func(target string) reverseClient {
		created++
		return fc
	}

	done := make(chan struct{})
	go func() { connectWithRetry("127.0.0.1:8443", 2, factory, noSleep); close(done) }()

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
	if fc.closed < 1 {
		t.Fatalf("expected Close to be called at least once, got %d", fc.closed)
	}
}
