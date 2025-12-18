package main

import "testing"

func TestRunClientArgValidation(t *testing.T) {
	if err := runClient([]string{}); err == nil {
		t.Fatal("expected error for missing args")
	}
	if err := runClient([]string{"127.0.0.1:8443"}); err == nil {
		t.Fatal("expected error for too few args")
	}
}
