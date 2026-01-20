package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

// Assertion defines what to expect from a step result
type Assertion struct {
	Type        string `yaml:"type"`     // "contains", "contains_ci", "equals", etc.
	Expected    string `yaml:"expected"` // Expected value
	Description string `yaml:"description"`
}

// CommandStep is a shell command execution step
type CommandStep struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"` // "command"
	Command    string      `yaml:"command"`
	TimeoutMS  int         `yaml:"timeout_ms"`
	Assertions []Assertion `yaml:"assertions"`
}

// ForwardStep starts a port forward
type ForwardStep struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"` // "forward"
	LocalPort  string      `yaml:"local_port"`
	RemoteAddr string      `yaml:"remote_addr"`
	FetchURL   string      `yaml:"fetch_url"`
	Assertions []Assertion `yaml:"assertions"`
}

// SocksStep starts a SOCKS proxy
type SocksStep struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"` // "socks"
	LocalPort  string      `yaml:"local_port"`
	TargetURL  string      `yaml:"target_url"`
	Assertions []Assertion `yaml:"assertions"`
}

// UploadStep uploads a file from the listener to the client via headless API
type UploadStep struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"` // "upload"
	LocalPath  string      `yaml:"local_path"`
	RemotePath string      `yaml:"remote_path"`
	Client     string      `yaml:"client"`
	Assertions []Assertion `yaml:"assertions"`
}

// DownloadStep downloads a file from the client via headless API
type DownloadStep struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"` // "download"
	RemotePath string      `yaml:"remote_path"`
	Client     string      `yaml:"client"`
	Assertions []Assertion `yaml:"assertions"`
}

// Journey is the top-level test journey configuration
type Journey struct {
	Name             string        `yaml:"name"`
	Description      string        `yaml:"description"`
	DefaultTimeoutMS int           `yaml:"default_timeout_ms"` // Default timeout for all steps
	Steps            []interface{} `yaml:"steps"`              // Mixed type support: CommandStep, ForwardStep, SocksStep, UploadStep, DownloadStep
}

// LoadJourney loads a journey YAML file
func LoadJourney(filepath string) (*Journey, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read journey file: %w", err)
	}

	// Parse with proper type handling
	var journey struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Steps       []map[string]interface{}
	}
	if err := yaml.Unmarshal(data, &journey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal journey: %w", err)
	}

	result := &Journey{
		Name:        journey.Name,
		Description: journey.Description,
		Steps:       []interface{}{},
	}

	for _, rawStep := range journey.Steps {
		stepType, ok := rawStep["type"].(string)
		if !ok {
			continue
		}

		// Re-marshal each step to proper type
		stepBytes, _ := yaml.Marshal(rawStep)

		switch stepType {
		case "command":
			var step CommandStep
			if err := yaml.Unmarshal(stepBytes, &step); err == nil {
				step.Type = "command"
				result.Steps = append(result.Steps, step)
			}
		case "forward":
			var step ForwardStep
			if err := yaml.Unmarshal(stepBytes, &step); err == nil {
				step.Type = "forward"
				result.Steps = append(result.Steps, step)
			}
		case "socks":
			var step SocksStep
			if err := yaml.Unmarshal(stepBytes, &step); err == nil {
				step.Type = "socks"
				result.Steps = append(result.Steps, step)
			}
		case "upload":
			var step UploadStep
			if err := yaml.Unmarshal(stepBytes, &step); err == nil {
				step.Type = "upload"
				result.Steps = append(result.Steps, step)
			}
		case "download":
			var step DownloadStep
			if err := yaml.Unmarshal(stepBytes, &step); err == nil {
				step.Type = "download"
				result.Steps = append(result.Steps, step)
			}
		}
	}

	return result, nil
}

// ExecuteJourney runs all steps in a journey
func ExecuteJourney(t *testing.T, httpClient *http.Client, endpoint string, journey *Journey) {
	t.Logf("Executing journey: %s", journey.Name)
	if journey.Description != "" {
		t.Logf("  Description: %s", journey.Description)
	}

	// Set default timeout
	defaultTimeout := journey.DefaultTimeoutMS
	if defaultTimeout == 0 {
		defaultTimeout = 15000 // 15 seconds default
	}

	for i, step := range journey.Steps {
		stepNum := i + 1
		switch s := step.(type) {
		case CommandStep:
			t.Logf("Step %d: %s (command)", stepNum, s.Name)
			executeCommandStep(t, httpClient, endpoint, s, defaultTimeout)

		case ForwardStep:
			t.Logf("Step %d: %s (forward)", stepNum, s.Name)
			executeForwardStep(t, httpClient, endpoint, s)

		case SocksStep:
			t.Logf("Step %d: %s (socks)", stepNum, s.Name)
			executeSocksStep(t, httpClient, endpoint, s)

		case UploadStep:
			t.Logf("Step %d: %s (upload)", stepNum, s.Name)
			executeUploadStep(t, httpClient, endpoint, s)

		case DownloadStep:
			t.Logf("Step %d: %s (download)", stepNum, s.Name)
			executeDownloadStep(t, httpClient, endpoint, s)

		default:
			t.Fatalf("Unknown step type at index %d", stepNum)
		}
	}

	t.Logf("✓ Journey completed: %s", journey.Name)
}

func executeCommandStep(t *testing.T, client *http.Client, endpoint string, step CommandStep, defaultTimeout int) {
	timeoutMS := step.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = defaultTimeout
	}

	// Expand environment variables in command
	cmd := os.ExpandEnv(step.Command)

	output := sendCommand(t, client, endpoint, cmd, timeoutMS)

	for _, assertion := range step.Assertions {
		validateAssertion(t, assertion, output)
	}
}

func executeForwardStep(t *testing.T, client *http.Client, endpoint string, step ForwardStep) {
	fwd := startForward(t, client, endpoint, step.LocalPort, step.RemoteAddr)
	t.Logf("    Forward started: %s", fwd.LocalAddr)

	fetchURL := step.FetchURL
	if step.LocalPort == "0" || strings.TrimSpace(fetchURL) == "" {
		// Derive fetch URL from returned local address
		// Expect fwd.LocalAddr like "0.0.0.0:PORT"; use gotsl hostname for reachability
		parts := strings.Split(fwd.LocalAddr, ":")
		port := parts[len(parts)-1]
		fetchURL = fmt.Sprintf("http://gotsl:%s/", strings.TrimSpace(port))
	}

	body := fetchDirect(t, fetchURL)

	for _, assertion := range step.Assertions {
		validateAssertion(t, assertion, body)
	}
}

func executeSocksStep(t *testing.T, httpClient *http.Client, endpoint string, step SocksStep) {
	// Start SOCKS via headless API
	payload := fmt.Sprintf(`{"local_port":"%s"}`, step.LocalPort)
	resp, err := httpClient.Post(endpoint+"/socks", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("socks request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("socks status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var socksInfo struct {
		ID        string `json:"id"`
		LocalAddr string `json:"local_addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&socksInfo); err != nil {
		t.Fatalf("decode socks response: %v", err)
	}
	t.Logf("    SOCKS proxy started: %s", socksInfo.LocalAddr)

	// Use returned local addr's port to avoid conflicts with static ports
	parts := strings.Split(socksInfo.LocalAddr, ":")
	port := parts[len(parts)-1]
	body := fetchViaSocks(t, "gotsl:"+strings.TrimSpace(port), step.TargetURL)

	for _, assertion := range step.Assertions {
		validateAssertion(t, assertion, body)
	}
}

func executeUploadStep(t *testing.T, httpClient *http.Client, endpoint string, step UploadStep) {
	// Build request body
	req := map[string]string{
		"local_path":  step.LocalPath,
		"remote_path": step.RemotePath,
	}
	if strings.TrimSpace(step.Client) != "" {
		req["client"] = step.Client
	}
	body, _ := json.Marshal(req)

	resp, err := httpClient.Post(endpoint+"/upload", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	actual := out.Status

	for _, assertion := range step.Assertions {
		validateAssertion(t, assertion, actual)
	}
}

func executeDownloadStep(t *testing.T, httpClient *http.Client, endpoint string, step DownloadStep) {
	// Build request body
	req := map[string]string{
		"remote_path": step.RemotePath,
	}
	if strings.TrimSpace(step.Client) != "" {
		req["client"] = step.Client
	}
	body, _ := json.Marshal(req)

	resp, err := httpClient.Post(endpoint+"/download", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("download failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	data, _ := io.ReadAll(resp.Body)
	actual := string(data)

	for _, assertion := range step.Assertions {
		validateAssertion(t, assertion, actual)
	}
}

func validateAssertion(t *testing.T, assertion Assertion, actual string) {
	var passed bool

	switch assertion.Type {
	case "contains":
		passed = strings.Contains(actual, assertion.Expected)
	case "contains_ci":
		passed = strings.Contains(strings.ToLower(actual), strings.ToLower(assertion.Expected))
	case "not_contains":
		passed = !strings.Contains(actual, assertion.Expected)
		if !passed {
			t.Fatalf("Assertion failed: %s\n  Type: %s\n  Should NOT contain: %q\n  But output contains it: %q",
				assertion.Description, assertion.Type, assertion.Expected, actual)
		}
	case "equals":
		passed = strings.TrimSpace(actual) == strings.TrimSpace(assertion.Expected)
	case "equals_ci":
		passed = strings.ToLower(strings.TrimSpace(actual)) == strings.ToLower(strings.TrimSpace(assertion.Expected))
	default:
		t.Fatalf("Unknown assertion type: %s", assertion.Type)
	}

	if !passed {
		t.Fatalf("Assertion failed: %s\n  Type: %s\n  Expected: %q\n  Got: %q",
			assertion.Description, assertion.Type, assertion.Expected, actual)
	}

	t.Logf("    ✓ %s", assertion.Description)
}

// Helper functions (adapted from headless_e2e_test.go)

func sendCommand(t *testing.T, client *http.Client, endpoint, command string, timeoutMS int) string {
	// Properly marshal the command into JSON to handle special characters
	req := struct {
		Command   string `json:"command"`
		TimeoutMS int    `json:"timeout_ms"`
	}{
		Command:   command,
		TimeoutMS: timeoutMS,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal command request: %v", err)
	}

	resp, err := client.Post(endpoint+"/command", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("command request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("command %q failed with status %d: %s", command, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct{ Output string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	return out.Output
}

func startForward(t *testing.T, client *http.Client, endpoint, localPort, remoteAddr string) struct {
	ID         string
	LocalAddr  string
	RemoteAddr string
} {
	payload := fmt.Sprintf(`{"local_port":"%s","remote_addr":"%s"}`, localPort, remoteAddr)
	resp, err := client.Post(endpoint+"/forward", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("forward request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("forward status: %d body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		ID         string `json:"id"`
		LocalAddr  string `json:"local_addr"`
		RemoteAddr string `json:"remote_addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode forward response: %v", err)
	}
	if !strings.Contains(result.LocalAddr, localPort) {
		t.Fatalf("forward local addr missing port: %s", result.LocalAddr)
	}
	return struct {
		ID         string
		LocalAddr  string
		RemoteAddr string
	}{
		ID:         result.ID,
		LocalAddr:  result.LocalAddr,
		RemoteAddr: result.RemoteAddr,
	}
}

func fetchDirect(t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("direct fetch failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func fetchViaSocks(t *testing.T, socksAddr, targetURL string) string {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("failed to create socks dialer: %v", err)
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}

	transport := &http.Transport{DialContext: dialContext}
	client := &http.Client{Transport: transport}

	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("socks fetch failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}
