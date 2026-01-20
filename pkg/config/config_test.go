package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Port != "9001" {
		t.Errorf("expected port 9001, got %s", cfg.Port)
	}
	if cfg.NetworkInterface != "0.0.0.0" {
		t.Errorf("expected network interface 0.0.0.0, got %s", cfg.NetworkInterface)
	}
	if cfg.BufferSize != 1024*1024 {
		t.Errorf("expected buffer size 1MB, got %d", cfg.BufferSize)
	}
	if cfg.MaxBufferSize != 10*1024*1024 {
		t.Errorf("expected max buffer size 10MB, got %d", cfg.MaxBufferSize)
	}
}

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("expected max retries 5, got %d", cfg.MaxRetries)
	}
	if cfg.BufferSize != 1024*1024 {
		t.Errorf("expected buffer size 1MB, got %d", cfg.BufferSize)
	}
}

func TestLoadServerConfig(t *testing.T) {
	cfg, err := LoadServerConfig("8080", "127.0.0.1", false)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Port)
	}
	if cfg.NetworkInterface != "127.0.0.1" {
		t.Errorf("expected network interface 127.0.0.1, got %s", cfg.NetworkInterface)
	}
	if cfg.SharedSecretAuth != false {
		t.Errorf("expected shared secret auth false, got %v", cfg.SharedSecretAuth)
	}
}

func TestLoadServerConfigWithSharedSecret(t *testing.T) {
	cfg, err := LoadServerConfig("9001", "0.0.0.0", true)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.SharedSecretAuth != true {
		t.Errorf("expected shared secret auth true, got %v", cfg.SharedSecretAuth)
	}
}

func TestLoadClientConfig(t *testing.T) {
	cfg, err := LoadClientConfig("localhost:9001", 3, "", "")
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if cfg.Target != "localhost:9001" {
		t.Errorf("expected target localhost:9001, got %s", cfg.Target)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", cfg.MaxRetries)
	}
}

func TestLoadClientConfigWithCredentials(t *testing.T) {
	secret := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	fingerprint := "abc123def456"
	cfg, err := LoadClientConfig("localhost:9001", 3, secret, fingerprint)
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if cfg.SharedSecret != secret {
		t.Errorf("expected secret %s, got %s", secret, cfg.SharedSecret)
	}
	if cfg.CertFingerprint != fingerprint {
		t.Errorf("expected fingerprint %s, got %s", fingerprint, cfg.CertFingerprint)
	}
}

func TestServerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ServerConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     DefaultServerConfig(),
			wantErr: false,
		},
		{
			name: "missing port",
			cfg: &ServerConfig{
				NetworkInterface: "0.0.0.0",
				BufferSize:       1024,
				MaxBufferSize:    10240,
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			cfg: &ServerConfig{
				Port:             "not-a-port",
				NetworkInterface: "0.0.0.0",
				BufferSize:       1024,
				MaxBufferSize:    10240,
			},
			wantErr: true,
		},
		{
			name: "missing network interface",
			cfg: &ServerConfig{
				Port:          "9001",
				BufferSize:    1024,
				MaxBufferSize: 10240,
			},
			wantErr: true,
		},
		{
			name: "invalid buffer size",
			cfg: &ServerConfig{
				Port:             "9001",
				NetworkInterface: "0.0.0.0",
				BufferSize:       -1,
				MaxBufferSize:    10240,
			},
			wantErr: true,
		},
		{
			name: "max buffer smaller than buffer",
			cfg: &ServerConfig{
				Port:             "9001",
				NetworkInterface: "0.0.0.0",
				BufferSize:       10240,
				MaxBufferSize:    1024,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: &ClientConfig{
				Target:          "localhost:9001",
				MaxRetries:      5,
				BufferSize:      1024 * 1024,
				MaxBufferSize:   10 * 1024 * 1024,
				ChunkSize:       65536,
				ReadTimeout:     1 * time.Second,
				ResponseTimeout: 5 * time.Second,
				CommandTimeout:  120 * time.Second,
				DownloadTimeout: 5000000000 * time.Nanosecond,
				PingInterval:    30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "missing target",
			cfg: &ClientConfig{
				MaxRetries:      5,
				BufferSize:      1024,
				MaxBufferSize:   10240,
				ChunkSize:       65536,
				ReadTimeout:     1 * time.Second,
				ResponseTimeout: 5 * time.Second,
				CommandTimeout:  120 * time.Second,
				PingInterval:    30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid shared secret length",
			cfg: &ClientConfig{
				Target:          "localhost:9001",
				MaxRetries:      5,
				BufferSize:      1024,
				MaxBufferSize:   10240,
				ChunkSize:       65536,
				SharedSecret:    "tooshort",
				ReadTimeout:     1 * time.Second,
				ResponseTimeout: 5 * time.Second,
				CommandTimeout:  120 * time.Second,
				PingInterval:    30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "valid shared secret (64 hex chars)",
			cfg: &ClientConfig{
				Target:          "localhost:9001",
				MaxRetries:      5,
				BufferSize:      1024,
				MaxBufferSize:   10240,
				ChunkSize:       65536,
				SharedSecret:    "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
				ReadTimeout:     1 * time.Second,
				ResponseTimeout: 5 * time.Second,
				CommandTimeout:  120 * time.Second,
				PingInterval:    30 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfigEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("GOTS_PORT", "8888")
	os.Setenv("GOTS_NETWORK_INTERFACE", "192.168.1.1")
	os.Setenv("GOTS_BUFFER_SIZE", "2097152")
	defer func() {
		os.Unsetenv("GOTS_PORT")
		os.Unsetenv("GOTS_NETWORK_INTERFACE")
		os.Unsetenv("GOTS_BUFFER_SIZE")
	}()

	cfg, err := LoadServerConfig("", "", false)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.Port != "8888" {
		t.Errorf("expected port 8888, got %s", cfg.Port)
	}
	if cfg.NetworkInterface != "192.168.1.1" {
		t.Errorf("expected network interface 192.168.1.1, got %s", cfg.NetworkInterface)
	}
	if cfg.BufferSize != 2097152 {
		t.Errorf("expected buffer size 2097152, got %d", cfg.BufferSize)
	}
}

func TestClientConfigEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("GOTS_TARGET", "example.com:9001")
	os.Setenv("GOTS_MAX_RETRIES", "10")
	defer func() {
		os.Unsetenv("GOTS_TARGET")
		os.Unsetenv("GOTS_MAX_RETRIES")
	}()

	cfg, err := LoadClientConfig("", -1, "", "")
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if cfg.Target != "example.com:9001" {
		t.Errorf("expected target example.com:9001, got %s", cfg.Target)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("expected max retries 10, got %d", cfg.MaxRetries)
	}
}

func TestConfigEnvInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		value    string
		isServer bool
	}{
		{
			name:     "invalid buffer size",
			envVar:   "GOTS_BUFFER_SIZE",
			value:    "not-a-number",
			isServer: true,
		},
		{
			name:     "invalid max retries",
			envVar:   "GOTS_MAX_RETRIES",
			value:    "not-a-number",
			isServer: false,
		},
		{
			name:     "invalid timeout",
			envVar:   "GOTS_READ_TIMEOUT",
			value:    "invalid-duration",
			isServer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.envVar, tt.value)
			defer os.Unsetenv(tt.envVar)

			var err error
			if tt.isServer {
				_, err = LoadServerConfig("9001", "0.0.0.0", false)
			} else {
				_, err = LoadClientConfig("localhost:9001", 5, "", "")
			}

			if err == nil {
				t.Errorf("expected error with invalid %s, got nil", tt.envVar)
			}
		})
	}
}

func TestConfigArgumentsPriority(t *testing.T) {
	// Arguments should override defaults
	cfg, err := LoadServerConfig("7777", "192.168.0.1", true)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.Port != "7777" {
		t.Errorf("expected port 7777, got %s", cfg.Port)
	}
	if cfg.NetworkInterface != "192.168.0.1" {
		t.Errorf("expected network interface 192.168.0.1, got %s", cfg.NetworkInterface)
	}
	if !cfg.SharedSecretAuth {
		t.Errorf("expected shared secret auth true, got %v", cfg.SharedSecretAuth)
	}
}

// Additional validation tests for better coverage
func TestServerConfigValidateEmptyPort(t *testing.T) {
	cfg := &ServerConfig{NetworkInterface: "0.0.0.0"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty port")
	}
}

func TestServerConfigValidateInvalidPort(t *testing.T) {
	cfg := &ServerConfig{Port: "not-a-port", NetworkInterface: "0.0.0.0"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid port")
	}
}

func TestServerConfigValidateEmptyNetworkInterface(t *testing.T) {
	cfg := &ServerConfig{Port: "9001"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty network interface")
	}
}

func TestServerConfigValidateInvalidBufferSize(t *testing.T) {
	cfg := &ServerConfig{
		Port:             "9001",
		NetworkInterface: "0.0.0.0",
		BufferSize:       0,
		MaxBufferSize:    10 * 1024 * 1024,
		ChunkSize:        65536,
		ReadTimeout:      1 * time.Second,
		ResponseTimeout:  5 * time.Second,
		CommandTimeout:   120 * time.Second,
		PingInterval:     30 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid buffer size")
	}
}

func TestServerConfigValidateMaxBufferSmaller(t *testing.T) {
	cfg := &ServerConfig{
		Port:             "9001",
		NetworkInterface: "0.0.0.0",
		BufferSize:       10 * 1024 * 1024,
		MaxBufferSize:    1 * 1024 * 1024, // smaller than buffer size
		ChunkSize:        65536,
		ReadTimeout:      1 * time.Second,
		ResponseTimeout:  5 * time.Second,
		CommandTimeout:   120 * time.Second,
		PingInterval:     30 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when max_buffer_size < buffer_size")
	}
}

func TestServerConfigValidateInvalidChunkSize(t *testing.T) {
	cfg := &ServerConfig{
		Port:             "9001",
		NetworkInterface: "0.0.0.0",
		BufferSize:       1 * 1024 * 1024,
		MaxBufferSize:    10 * 1024 * 1024,
		ChunkSize:        0, // invalid
		ReadTimeout:      1 * time.Second,
		ResponseTimeout:  5 * time.Second,
		CommandTimeout:   120 * time.Second,
		PingInterval:     30 * time.Second,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid chunk size")
	}
}

func TestServerConfigValidateInvalidTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ServerConfig)
	}{
		{
			name: "invalid read timeout",
			setup: func(cfg *ServerConfig) {
				cfg.ReadTimeout = 0
			},
		},
		{
			name: "invalid response timeout",
			setup: func(cfg *ServerConfig) {
				cfg.ResponseTimeout = 0
			},
		},
		{
			name: "invalid command timeout",
			setup: func(cfg *ServerConfig) {
				cfg.CommandTimeout = 0
			},
		},
		{
			name: "invalid ping interval",
			setup: func(cfg *ServerConfig) {
				cfg.PingInterval = 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ServerConfig{
				Port:             "9001",
				NetworkInterface: "0.0.0.0",
				BufferSize:       1 * 1024 * 1024,
				MaxBufferSize:    10 * 1024 * 1024,
				ChunkSize:        65536,
				ReadTimeout:      1 * time.Second,
				ResponseTimeout:  5 * time.Second,
				CommandTimeout:   120 * time.Second,
				PingInterval:     30 * time.Second,
			}
			tt.setup(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
		})
	}
}

func TestClientConfigValidateEmptyTarget(t *testing.T) {
	cfg := &ClientConfig{}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty target")
	}
}

func TestClientConfigValidateNegativeRetries(t *testing.T) {
	cfg := &ClientConfig{
		Target:     "localhost:9001",
		MaxRetries: -1,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative max retries")
	}
}

func TestClientConfigValidateInvalidSecret(t *testing.T) {
	cfg := &ClientConfig{
		Target:       "localhost:9001",
		SharedSecret: "short", // Too short
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid shared secret length")
	}
}

func TestClientConfigValidateValidSecret(t *testing.T) {
	cfg := &ClientConfig{
		Target:          "localhost:9001",
		MaxRetries:      5,
		SharedSecret:    "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20",
		BufferSize:      1 * 1024 * 1024,
		MaxBufferSize:   10 * 1024 * 1024,
		ChunkSize:       65536,
		ReadTimeout:     1 * time.Second,
		ResponseTimeout: 5 * time.Second,
		CommandTimeout:  120 * time.Second,
		PingInterval:    30 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config with proper secret, got error: %v", err)
	}
}

func TestClientConfigValidateBufferSizeErrors(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		maxBuffer  int
		expectErr  bool
	}{
		{"zero buffer size", 0, 10 * 1024 * 1024, true},
		{"zero max buffer", 1 * 1024 * 1024, 0, true},
		{"max smaller than buffer", 10 * 1024 * 1024, 1 * 1024 * 1024, true},
		{"valid buffers", 1 * 1024 * 1024, 10 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ClientConfig{
				Target:          "localhost:9001",
				MaxRetries:      5,
				BufferSize:      tt.bufferSize,
				MaxBufferSize:   tt.maxBuffer,
				ChunkSize:       65536,
				ReadTimeout:     1 * time.Second,
				ResponseTimeout: 5 * time.Second,
				CommandTimeout:  120 * time.Second,
				PingInterval:    30 * time.Second,
			}
			err := cfg.Validate()
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error=%v, got error=%v", tt.expectErr, err != nil)
			}
		})
	}
}

func TestEnvVarServerConfig(t *testing.T) {
	// Test environment variable overrides for server config
	os.Setenv("GOTS_PORT", "8888")
	os.Setenv("GOTS_NETWORK_INTERFACE", "127.0.0.1")
	defer func() {
		os.Unsetenv("GOTS_PORT")
		os.Unsetenv("GOTS_NETWORK_INTERFACE")
	}()

	cfg, err := LoadServerConfig("9001", "0.0.0.0", false)
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if cfg.Port != "8888" {
		t.Errorf("expected port from env var 8888, got %s", cfg.Port)
	}
	if cfg.NetworkInterface != "127.0.0.1" {
		t.Errorf("expected interface from env var 127.0.0.1, got %s", cfg.NetworkInterface)
	}
}

func TestEnvVarClientConfig(t *testing.T) {
	// Test environment variable overrides for client config
	os.Setenv("GOTS_TARGET", "example.com:9001")
	os.Setenv("GOTS_MAX_RETRIES", "10")
	defer func() {
		os.Unsetenv("GOTS_TARGET")
		os.Unsetenv("GOTS_MAX_RETRIES")
	}()

	cfg, err := LoadClientConfig("localhost:9001", 3, "", "")
	if err != nil {
		t.Fatalf("LoadClientConfig failed: %v", err)
	}

	if cfg.Target != "example.com:9001" {
		t.Errorf("expected target from env var example.com:9001, got %s", cfg.Target)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("expected max retries from env var 10, got %d", cfg.MaxRetries)
	}
}
