package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestHeadlessE2E(t *testing.T) {
	endpoint := os.Getenv("HEADLESS_ENDPOINT")
	if endpoint == "" {
		t.Skip("HEADLESS_ENDPOINT not set; skipping headless e2e")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	waitForHealth(t, httpClient, endpoint)
	waitForClient(t, httpClient, endpoint)

	// Create test file for upload test
	testFilePath := "/tmp/test-upload.txt"
	testContent := "test content for file transfer"
	if err := os.WriteFile(testFilePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	t.Logf("Created test file: %s", testFilePath)

	// Load and execute journey playbook
	// Check for JOURNEY_PATH environment variable (for custom journeys)
	journeyPath := os.Getenv("JOURNEY_PATH")
	if journeyPath == "" {
		// Default to default.yaml if not specified
		journeyPath = "integration/journeys/default.yaml"
	}

	// Try multiple paths if file not found
	if _, err := os.Stat(journeyPath); os.IsNotExist(err) {
		// Try from src root
		altPath := "/src/" + journeyPath
		if _, err := os.Stat(altPath); err == nil {
			journeyPath = altPath
		}
	}

	journey, err := LoadJourney(journeyPath)
	if err != nil {
		t.Fatalf("failed to load journey from %s: %v", journeyPath, err)
	}

	ExecuteJourney(t, httpClient, endpoint, journey)
}

func waitForHealth(t *testing.T, client *http.Client, endpoint string) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("headless endpoint %s did not become healthy", endpoint)
}

func waitForClient(t *testing.T, client *http.Client, endpoint string) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(endpoint + "/clients")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("no clients connected to %s within timeout", endpoint)
}
