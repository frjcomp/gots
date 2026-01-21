package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/frjcomp/gots/pkg/protocol"
	"github.com/frjcomp/gots/pkg/server"
)

type mockHeadlessListener struct {
	clients   []string
	commands  []string
	responses map[string]string
	fm        *stubForwardManager
	sm        *stubSocksManager
}

func (m *mockHeadlessListener) Start() (netListener net.Listener, err error) { return nil, nil }
func (m *mockHeadlessListener) GetClients() []string                         { return m.clients }
func (m *mockHeadlessListener) GetClientAddressesSorted() []string           { return m.clients }
func (m *mockHeadlessListener) GetClientIdentifier(clientAddr string) string {
	return "id-" + clientAddr
}
func (m *mockHeadlessListener) GetClientMetadata(clientAddr string) (server.ClientMetadata, bool) {
	return server.ClientMetadata{Identifier: "meta-" + clientAddr, OS: "linux"}, true
}
func (m *mockHeadlessListener) SendCommand(clientAddr, cmd string) error {
	m.commands = append(m.commands, clientAddr+"|"+cmd)
	return nil
}
func (m *mockHeadlessListener) GetResponse(clientAddr string, _ time.Duration) (string, error) {
	if m.responses != nil {
		if resp, ok := m.responses[clientAddr]; ok {
			return resp, nil
		}
	}
	return "OK\n" + protocol.EndOfOutputMarker + "\n", nil
}
func (m *mockHeadlessListener) EnterPtyMode(clientAddr string) (chan []byte, error) { return nil, nil }
func (m *mockHeadlessListener) ExitPtyMode(clientAddr string) error                 { return nil }
func (m *mockHeadlessListener) IsInPtyMode(clientAddr string) bool                  { return false }
func (m *mockHeadlessListener) GetPtyDataChan(clientAddr string) (chan []byte, bool) {
	return nil, false
}
func (m *mockHeadlessListener) IsAnyPtyModeActive() bool {
	return false
}
func (m *mockHeadlessListener) DisconnectClient(clientAddr string) error {
	for i, addr := range m.clients {
		if addr == clientAddr {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("client not found")
}
func (m *mockHeadlessListener) GetForwardManager() forwardManager {
	return m.fm
}
func (m *mockHeadlessListener) GetSocksManager() socksManager {
	return m.sm
}

type stubForwardManager struct {
	lastID     string
	lastLocal  string
	lastRemote string
	bindAddr   string
	startErr   error
}

func (s *stubForwardManager) StartForward(id, localPort, remoteAddr string, sendFunc func(string)) error {
	s.lastID = id
	s.lastLocal = localPort
	s.lastRemote = remoteAddr
	return s.startErr
}

func (s *stubForwardManager) BindAddr() string {
	if s.bindAddr != "" {
		return s.bindAddr
	}
	return "127.0.0.1"
}

type stubSocksManager struct {
	lastID    string
	lastLocal string
	bindAddr  string
	startErr  error
}

func (s *stubSocksManager) StartSocks(id, localPort string, sendFunc func(string)) error {
	s.lastID = id
	s.lastLocal = localPort
	return s.startErr
}

func (s *stubSocksManager) BindAddr() string {
	if s.bindAddr != "" {
		return s.bindAddr
	}
	return "127.0.0.1"
}

func TestHeadlessServerCommandFlow(t *testing.T) {
	ml := &mockHeadlessListener{clients: []string{"1.2.3.4:9000"}}
	hs, err := newHeadlessServer(ml, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start headless server: %v", err)
	}
	hs.Start()
	defer hs.Stop(context.Background())

	client := &http.Client{Timeout: 2 * time.Second}

	// Verify clients endpoint
	resp, err := client.Get("http://" + hs.Addr() + "/clients")
	if err != nil {
		t.Fatalf("clients request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var clients []clientInfo
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		t.Fatalf("decode clients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}

	// Issue command and expect OK
	payload := `{"command":"PING"}`
	resp, err = client.Post("http://"+hs.Addr()+"/command", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("command post failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	var cmdResp controlCommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&cmdResp); err != nil {
		t.Fatalf("decode command response: %v", err)
	}
	if cmdResp.Output != "OK" {
		t.Fatalf("unexpected output: %s", cmdResp.Output)
	}

	if len(ml.commands) != 1 || !strings.Contains(ml.commands[0], "PING") {
		t.Fatalf("command not recorded correctly: %+v", ml.commands)
	}
}

func TestHeadlessServerShutdown(t *testing.T) {
	ml := &mockHeadlessListener{clients: []string{"127.0.0.1:1"}}
	hs, err := newHeadlessServer(ml, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start headless server: %v", err)
	}
	hs.Start()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://"+hs.Addr()+"/shutdown", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("shutdown request failed: %v", err)
	}
	resp.Body.Close()

	select {
	case <-hs.Wait():
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("headless server did not stop")
	}
}

func TestHeadlessServerForwardEndpoint(t *testing.T) {
	fm := &stubForwardManager{bindAddr: "0.0.0.0"}
	ml := &mockHeadlessListener{clients: []string{"10.0.0.1:1"}, fm: fm}
	hs, err := newHeadlessServer(ml, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start headless server: %v", err)
	}
	hs.Start()
	defer hs.Stop(context.Background())

	client := &http.Client{Timeout: 2 * time.Second}
	payload := `{"local_port":"18080","remote_addr":"backend:8080"}`
	resp, err := client.Post("http://"+hs.Addr()+"/forward", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("forward request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if fm.lastLocal != "18080" || fm.lastRemote != "backend:8080" {
		t.Fatalf("forward not started correctly: %+v", fm)
	}
}

func TestHeadlessServerSocksEndpoint(t *testing.T) {
	sm := &stubSocksManager{bindAddr: "0.0.0.0"}
	ml := &mockHeadlessListener{clients: []string{"10.0.0.1:1"}, sm: sm}
	hs, err := newHeadlessServer(ml, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start headless server: %v", err)
	}
	hs.Start()
	defer hs.Stop(context.Background())

	client := &http.Client{Timeout: 2 * time.Second}
	payload := `{"local_port":"1080"}`
	resp, err := client.Post("http://"+hs.Addr()+"/socks", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("socks request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	if sm.lastLocal != "1080" {
		t.Fatalf("socks not started correctly: %+v", sm)
	}
}
