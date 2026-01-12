package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/frjcomp/gots/pkg/compression"
	"github.com/frjcomp/gots/pkg/protocol"
	"github.com/frjcomp/gots/pkg/server"
)

// headlessServer exposes a minimal HTTP API to drive the listener non-interactively.
// It is intended for CI/e2e automation and local scripting.
type headlessServer struct {
	listener server.ListenerInterface
	srv      *http.Server
	ln       net.Listener
	stopOnce sync.Once
	stopCh   chan struct{}
}

type forwardManager interface {
	StartForward(id, localPort, remoteAddr string, sendFunc func(string)) error
	BindAddr() string
}

type socksManager interface {
	StartSocks(id, localPort string, sendFunc func(string)) error
	BindAddr() string
}

type forwardProvider interface {
	GetForwardManager() forwardManager
}

type socksProvider interface {
	GetSocksManager() socksManager
}

type controlCommandRequest struct {
	Client    string `json:"client"`
	Command   string `json:"command"`
	TimeoutMS int    `json:"timeout_ms"`
}

type controlCommandResponse struct {
	Output string `json:"output"`
}

type clientInfo struct {
	Address    string                `json:"address"`
	Identifier string                `json:"identifier"`
	Metadata   server.ClientMetadata `json:"metadata"`
}

type forwardRequest struct {
	Client     string `json:"client"`
	LocalPort  string `json:"local_port"`
	RemoteAddr string `json:"remote_addr"`
}

type forwardResponse struct {
	ID         string `json:"id"`
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
}

type socksRequest struct {
	Client    string `json:"client"`
	LocalPort string `json:"local_port"`
}

type socksResponse struct {
	ID        string `json:"id"`
	LocalAddr string `json:"local_addr"`
}

func newHeadlessServer(l server.ListenerInterface, addr string) (*headlessServer, error) {
	mux := http.NewServeMux()
	hs := &headlessServer{
		listener: l,
		stopCh:   make(chan struct{}),
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clients := l.GetClientAddressesSorted()
		result := make([]clientInfo, 0, len(clients))
		for _, c := range clients {
			meta, _ := l.GetClientMetadata(c)
			result = append(result, clientInfo{
				Address:    c,
				Identifier: l.GetClientIdentifier(c),
				Metadata:   meta,
			})
		}

		writeJSON(w, result)
	})

	mux.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req controlCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		cmd := strings.TrimSpace(req.Command)
		if cmd == "" {
			http.Error(w, "command is required", http.StatusBadRequest)
			return
		}

		clientAddr := strings.TrimSpace(req.Client)
		if clientAddr == "" {
			clients := l.GetClientAddressesSorted()
			if len(clients) == 0 {
				http.Error(w, "no clients connected", http.StatusServiceUnavailable)
				return
			}
			clientAddr = clients[0]
		}

		timeout := time.Duration(req.TimeoutMS) * time.Millisecond
		if timeout == 0 {
			timeout = protocol.ResponseTimeout * time.Second
		}

		if err := l.SendCommand(clientAddr, cmd); err != nil {
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusBadRequest)
			return
		}

		resp, err := l.GetResponse(clientAddr, timeout)
		if err != nil {
			http.Error(w, fmt.Sprintf("response error: %v", err), http.StatusGatewayTimeout)
			return
		}

		cleaned := strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")
		cleaned = strings.TrimSpace(cleaned)

		writeJSON(w, controlCommandResponse{Output: cleaned})
	})

	mux.HandleFunc("/forward", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req forwardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.LocalPort == "" || req.RemoteAddr == "" {
			http.Error(w, "local_port and remote_addr are required", http.StatusBadRequest)
			return
		}

		var fm forwardManager
		switch v := l.(type) {
		case interface{ GetForwardManager() *server.ForwardManager }:
			fm = v.GetForwardManager()
		case forwardProvider:
			fm = v.GetForwardManager()
		default:
			http.Error(w, "listener does not support forwards", http.StatusInternalServerError)
			return
		}

		clientAddr := strings.TrimSpace(req.Client)
		if clientAddr == "" {
			clients := l.GetClientAddressesSorted()
			if len(clients) == 0 {
				http.Error(w, "no clients connected", http.StatusServiceUnavailable)
				return
			}
			clientAddr = clients[0]
		}

		fwdID := fmt.Sprintf("fwd-%d", time.Now().UnixNano())
		sendFunc := func(msg string) { _ = l.SendCommand(clientAddr, msg) }
		if err := fm.StartForward(fwdID, req.LocalPort, req.RemoteAddr, sendFunc); err != nil {
			http.Error(w, fmt.Sprintf("forward start failed: %v", err), http.StatusBadRequest)
			return
		}

		localAddr := fmt.Sprintf("%s:%s", fm.BindAddr(), req.LocalPort)
		writeJSON(w, forwardResponse{ID: fwdID, LocalAddr: localAddr, RemoteAddr: req.RemoteAddr})
	})

	mux.HandleFunc("/socks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req socksRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.LocalPort == "" {
			http.Error(w, "local_port is required", http.StatusBadRequest)
			return
		}

		var sm socksManager
		switch v := l.(type) {
		case interface{ GetSocksManager() *server.SocksManager }:
			sm = v.GetSocksManager()
		case socksProvider:
			sm = v.GetSocksManager()
		default:
			http.Error(w, "listener does not support socks", http.StatusInternalServerError)
			return
		}

		clientAddr := strings.TrimSpace(req.Client)
		if clientAddr == "" {
			clients := l.GetClientAddressesSorted()
			if len(clients) == 0 {
				http.Error(w, "no clients connected", http.StatusServiceUnavailable)
				return
			}
			clientAddr = clients[0]
		}

		socksID := fmt.Sprintf("socks-%d", time.Now().UnixNano())
		sendFunc := func(msg string) { _ = l.SendCommand(clientAddr, msg) }
		if err := sm.StartSocks(socksID, req.LocalPort, sendFunc); err != nil {
			http.Error(w, fmt.Sprintf("socks start failed: %v", err), http.StatusBadRequest)
			return
		}

		localAddr := fmt.Sprintf("%s:%s", sm.BindAddr(), req.LocalPort)
		writeJSON(w, socksResponse{ID: socksID, LocalAddr: localAddr})
	})

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Client     string `json:"client"`
			LocalPath  string `json:"local_path"`
			RemotePath string `json:"remote_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.LocalPath == "" || req.RemotePath == "" {
			http.Error(w, "local_path and remote_path are required", http.StatusBadRequest)
			return
		}

		clientAddr := strings.TrimSpace(req.Client)
		if clientAddr == "" {
			clients := l.GetClientAddressesSorted()
			if len(clients) == 0 {
				http.Error(w, "no clients connected", http.StatusServiceUnavailable)
				return
			}
			clientAddr = clients[0]
		}

		// Use handleUploadGlobal-like logic
		data, err := ioutil.ReadFile(req.LocalPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("error reading file: %v", err), http.StatusBadRequest)
			return
		}

		compressed, err := compression.CompressToHex(data)
		if err != nil {
			http.Error(w, fmt.Sprintf("error compressing file: %v", err), http.StatusInternalServerError)
			return
		}

		totalSize := len(compressed)
		startCmd := fmt.Sprintf("%s %s %d", protocol.CmdStartUpload, req.RemotePath, totalSize)
		if err := l.SendCommand(clientAddr, startCmd); err != nil {
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusBadRequest)
			return
		}

		resp, err := l.GetResponse(clientAddr, 30*time.Second)
		if err != nil {
			http.Error(w, fmt.Sprintf("response error: %v", err), http.StatusGatewayTimeout)
			return
		}
		if !strings.Contains(resp, "OK") {
			http.Error(w, fmt.Sprintf("upload start failed: %s", strings.TrimSpace(strings.ReplaceAll(resp, protocol.EndOfOutputMarker, ""))), http.StatusBadRequest)
			return
		}

		// Send chunks
		chunkNum := 0
		for i := 0; i < totalSize; i += protocol.ChunkSize {
			end := i + protocol.ChunkSize
			if end > totalSize {
				end = totalSize
			}
			chunk := compressed[i:end]
			chunkNum++
			chunkCmd := fmt.Sprintf("%s %s", protocol.CmdUploadChunk, chunk)
			if err := l.SendCommand(clientAddr, chunkCmd); err != nil {
				http.Error(w, fmt.Sprintf("chunk send failed: %v", err), http.StatusBadRequest)
				return
			}
			resp, err := l.GetResponse(clientAddr, 30*time.Second)
			if err != nil {
				http.Error(w, fmt.Sprintf("chunk response error: %v", err), http.StatusGatewayTimeout)
				return
			}
			if !strings.Contains(resp, "OK") {
				cleanResp := strings.TrimSpace(strings.ReplaceAll(resp, protocol.EndOfOutputMarker, ""))
				http.Error(w, fmt.Sprintf("chunk upload error: %s", cleanResp), http.StatusBadRequest)
				return
			}
		}

		// End upload
		endCmd := fmt.Sprintf("%s %s", protocol.CmdEndUpload, req.RemotePath)
		if err := l.SendCommand(clientAddr, endCmd); err != nil {
			http.Error(w, fmt.Sprintf("end upload failed: %v", err), http.StatusBadRequest)
			return
		}

		resp, err = l.GetResponse(clientAddr, 30*time.Second)
		if err != nil {
			http.Error(w, fmt.Sprintf("response error: %v", err), http.StatusGatewayTimeout)
			return
		}

		writeJSON(w, map[string]interface{}{
			"status":      "success",
			"bytes_sent":  len(data),
			"remote_path": req.RemotePath,
		})
	})

	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Client     string `json:"client"`
			RemotePath string `json:"remote_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.RemotePath == "" {
			http.Error(w, "remote_path is required", http.StatusBadRequest)
			return
		}

		clientAddr := strings.TrimSpace(req.Client)
		if clientAddr == "" {
			clients := l.GetClientAddressesSorted()
			if len(clients) == 0 {
				http.Error(w, "no clients connected", http.StatusServiceUnavailable)
				return
			}
			clientAddr = clients[0]
		}

		cmd := fmt.Sprintf("%s %s", protocol.CmdDownload, req.RemotePath)
		if err := l.SendCommand(clientAddr, cmd); err != nil {
			http.Error(w, fmt.Sprintf("send failed: %v", err), http.StatusBadRequest)
			return
		}

		resp, err := l.GetResponse(clientAddr, time.Duration(protocol.DownloadTimeout))
		if err != nil {
			http.Error(w, fmt.Sprintf("response error: %v", err), http.StatusGatewayTimeout)
			return
		}

		clean := strings.ReplaceAll(resp, protocol.EndOfOutputMarker, "")
		clean = strings.TrimSpace(clean)
		if !strings.HasPrefix(clean, protocol.DataPrefix) {
			http.Error(w, "unexpected download response", http.StatusInternalServerError)
			return
		}

		payload := strings.TrimPrefix(clean, protocol.DataPrefix)
		decoded, err := compression.DecompressHex(payload)
		if err != nil {
			http.Error(w, fmt.Sprintf("decode error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(req.RemotePath)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(decoded)))
		w.Write(decoded)
	})

	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go hs.Stop(context.Background())
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("shutting down"))
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("headless listen failed: %w", err)
	}

	hs.ln = ln
	hs.srv = &http.Server{Handler: mux}
	return hs, nil
}

func (h *headlessServer) Start() {
	go func() {
		if err := h.srv.Serve(h.ln); err != nil && err != http.ErrServerClosed {
			log.Printf("headless server error: %v", err)
		}
		h.stop()
	}()
}

func (h *headlessServer) Stop(ctx context.Context) error {
	h.stop()
	return h.srv.Shutdown(ctx)
}

func (h *headlessServer) stop() {
	h.stopOnce.Do(func() {
		close(h.stopCh)
	})
}

func (h *headlessServer) Wait() <-chan struct{} {
	return h.stopCh
}

func (h *headlessServer) Addr() string {
	if h.ln == nil {
		return ""
	}
	return h.ln.Addr().String()
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
