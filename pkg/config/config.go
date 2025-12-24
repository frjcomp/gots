// Package config provides configuration management for GOTS.
// It supports loading configuration from files and environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ServerConfig holds configuration for the gotsl listener.
type ServerConfig struct {
	Port               string        `yaml:"port" json:"port"`
	NetworkInterface   string        `yaml:"network_interface" json:"network_interface"`
	BufferSize         int           `yaml:"buffer_size" json:"buffer_size"`
	MaxBufferSize      int           `yaml:"max_buffer_size" json:"max_buffer_size"`
	ChunkSize          int           `yaml:"chunk_size" json:"chunk_size"`
	ReadTimeout        time.Duration `yaml:"read_timeout" json:"read_timeout"`
	ResponseTimeout    time.Duration `yaml:"response_timeout" json:"response_timeout"`
	CommandTimeout     time.Duration `yaml:"command_timeout" json:"command_timeout"`
	DownloadTimeout    time.Duration `yaml:"download_timeout" json:"download_timeout"`
	PingInterval       time.Duration `yaml:"ping_interval" json:"ping_interval"`
	SharedSecretAuth   bool          `yaml:"shared_secret_auth" json:"shared_secret_auth"`
}

// ClientConfig holds configuration for the gotsr client.
type ClientConfig struct {
	Target             string        `yaml:"target" json:"target"`
	MaxRetries         int           `yaml:"max_retries" json:"max_retries"`
	BufferSize         int           `yaml:"buffer_size" json:"buffer_size"`
	MaxBufferSize      int           `yaml:"max_buffer_size" json:"max_buffer_size"`
	ChunkSize          int           `yaml:"chunk_size" json:"chunk_size"`
	ReadTimeout        time.Duration `yaml:"read_timeout" json:"read_timeout"`
	ResponseTimeout    time.Duration `yaml:"response_timeout" json:"response_timeout"`
	CommandTimeout     time.Duration `yaml:"command_timeout" json:"command_timeout"`
	DownloadTimeout    time.Duration `yaml:"download_timeout" json:"download_timeout"`
	PingInterval       time.Duration `yaml:"ping_interval" json:"ping_interval"`
	SharedSecret       string        `yaml:"shared_secret" json:"shared_secret"`
	CertFingerprint    string        `yaml:"cert_fingerprint" json:"cert_fingerprint"`
}

// DefaultServerConfig returns server configuration with sensible defaults.
// Based on values from protocol/constants.go
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:             "9001",
		NetworkInterface: "0.0.0.0",
		BufferSize:       1024 * 1024,                  // 1MB
		MaxBufferSize:    10 * 1024 * 1024,             // 10MB
		ChunkSize:        65536,                        // 64KB
		ReadTimeout:      1 * time.Second,
		ResponseTimeout:  5 * time.Second,
		CommandTimeout:   120 * time.Second,
		DownloadTimeout:  5000000000 * time.Nanosecond, // ~5 seconds for large files
		PingInterval:     30 * time.Second,
		SharedSecretAuth: false,
	}
}

// DefaultClientConfig returns client configuration with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxRetries:      5,
		BufferSize:      1024 * 1024,                  // 1MB
		MaxBufferSize:   10 * 1024 * 1024,             // 10MB
		ChunkSize:       65536,                        // 64KB
		ReadTimeout:     1 * time.Second,
		ResponseTimeout: 5 * time.Second,
		CommandTimeout:  120 * time.Second,
		DownloadTimeout: 5000000000 * time.Nanosecond, // ~5 seconds for large files
		PingInterval:    30 * time.Second,
	}
}

// LoadServerConfig loads server configuration with environment variable overrides.
// Priority: env vars > passed values > defaults
func LoadServerConfig(port, networkInterface string, useSharedSecret bool) (*ServerConfig, error) {
	cfg := DefaultServerConfig()

	// Override with provided arguments
	if port != "" {
		cfg.Port = port
	}
	if networkInterface != "" {
		cfg.NetworkInterface = networkInterface
	}
	cfg.SharedSecretAuth = useSharedSecret

	// Apply environment variable overrides
	if err := applyServerConfigEnv(cfg); err != nil {
		return nil, err
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadClientConfig loads client configuration with environment variable overrides.
// Priority: env vars > passed values > defaults
func LoadClientConfig(target string, maxRetries int, sharedSecret, certFingerprint string) (*ClientConfig, error) {
	cfg := DefaultClientConfig()

	// Override with provided arguments
	if target != "" {
		cfg.Target = target
	}
	if maxRetries >= 0 {
		cfg.MaxRetries = maxRetries
	}
	if sharedSecret != "" {
		cfg.SharedSecret = sharedSecret
	}
	if certFingerprint != "" {
		cfg.CertFingerprint = certFingerprint
	}

	// Apply environment variable overrides
	if err := applyClientConfigEnv(cfg); err != nil {
		return nil, err
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyServerConfigEnv applies environment variable overrides to server config.
func applyServerConfigEnv(cfg *ServerConfig) error {
	envMap := map[string]func(string) error{
		"GOTS_PORT": func(v string) error {
			if v != "" {
				cfg.Port = v
			}
			return nil
		},
		"GOTS_NETWORK_INTERFACE": func(v string) error {
			if v != "" {
				cfg.NetworkInterface = v
			}
			return nil
		},
		"GOTS_BUFFER_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_BUFFER_SIZE: %w", err)
				}
				cfg.BufferSize = size
			}
			return nil
		},
		"GOTS_MAX_BUFFER_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_MAX_BUFFER_SIZE: %w", err)
				}
				cfg.MaxBufferSize = size
			}
			return nil
		},
		"GOTS_CHUNK_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_CHUNK_SIZE: %w", err)
				}
				cfg.ChunkSize = size
			}
			return nil
		},
		"GOTS_READ_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_READ_TIMEOUT: %w", err)
				}
				cfg.ReadTimeout = d
			}
			return nil
		},
		"GOTS_RESPONSE_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_RESPONSE_TIMEOUT: %w", err)
				}
				cfg.ResponseTimeout = d
			}
			return nil
		},
		"GOTS_COMMAND_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_COMMAND_TIMEOUT: %w", err)
				}
				cfg.CommandTimeout = d
			}
			return nil
		},
		"GOTS_PING_INTERVAL": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_PING_INTERVAL: %w", err)
				}
				cfg.PingInterval = d
			}
			return nil
		},
	}

	for envVar, apply := range envMap {
		if err := apply(os.Getenv(envVar)); err != nil {
			return err
		}
	}

	return nil
}

// applyClientConfigEnv applies environment variable overrides to client config.
func applyClientConfigEnv(cfg *ClientConfig) error {
	envMap := map[string]func(string) error{
		"GOTS_TARGET": func(v string) error {
			if v != "" {
				cfg.Target = v
			}
			return nil
		},
		"GOTS_MAX_RETRIES": func(v string) error {
			if v != "" {
				retries, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_MAX_RETRIES: %w", err)
				}
				cfg.MaxRetries = retries
			}
			return nil
		},
		"GOTS_BUFFER_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_BUFFER_SIZE: %w", err)
				}
				cfg.BufferSize = size
			}
			return nil
		},
		"GOTS_MAX_BUFFER_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_MAX_BUFFER_SIZE: %w", err)
				}
				cfg.MaxBufferSize = size
			}
			return nil
		},
		"GOTS_CHUNK_SIZE": func(v string) error {
			if v != "" {
				size, err := strconv.Atoi(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_CHUNK_SIZE: %w", err)
				}
				cfg.ChunkSize = size
			}
			return nil
		},
		"GOTS_READ_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_READ_TIMEOUT: %w", err)
				}
				cfg.ReadTimeout = d
			}
			return nil
		},
		"GOTS_RESPONSE_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_RESPONSE_TIMEOUT: %w", err)
				}
				cfg.ResponseTimeout = d
			}
			return nil
		},
		"GOTS_COMMAND_TIMEOUT": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_COMMAND_TIMEOUT: %w", err)
				}
				cfg.CommandTimeout = d
			}
			return nil
		},
		"GOTS_PING_INTERVAL": func(v string) error {
			if v != "" {
				d, err := time.ParseDuration(v)
				if err != nil {
					return fmt.Errorf("invalid GOTS_PING_INTERVAL: %w", err)
				}
				cfg.PingInterval = d
			}
			return nil
		},
		"GOTS_SHARED_SECRET": func(v string) error {
			if v != "" {
				cfg.SharedSecret = v
			}
			return nil
		},
		"GOTS_CERT_FINGERPRINT": func(v string) error {
			if v != "" {
				cfg.CertFingerprint = v
			}
			return nil
		},
	}

	for envVar, apply := range envMap {
		if err := apply(os.Getenv(envVar)); err != nil {
			return err
		}
	}

	return nil
}

// Validate validates the server configuration.
func (c *ServerConfig) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("port is required")
	}

	// Verify port is numeric
	if _, err := strconv.Atoi(c.Port); err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	if c.NetworkInterface == "" {
		return fmt.Errorf("network interface is required")
	}

	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive")
	}

	if c.MaxBufferSize <= 0 {
		return fmt.Errorf("max_buffer_size must be positive")
	}

	if c.MaxBufferSize < c.BufferSize {
		return fmt.Errorf("max_buffer_size must be >= buffer_size")
	}

	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk_size must be positive")
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}

	if c.ResponseTimeout <= 0 {
		return fmt.Errorf("response_timeout must be positive")
	}

	if c.CommandTimeout <= 0 {
		return fmt.Errorf("command_timeout must be positive")
	}

	if c.PingInterval <= 0 {
		return fmt.Errorf("ping_interval must be positive")
	}

	return nil
}

// Validate validates the client configuration.
func (c *ClientConfig) Validate() error {
	if c.Target == "" {
		return fmt.Errorf("target is required")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be non-negative")
	}

	if c.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive")
	}

	if c.MaxBufferSize <= 0 {
		return fmt.Errorf("max_buffer_size must be positive")
	}

	if c.MaxBufferSize < c.BufferSize {
		return fmt.Errorf("max_buffer_size must be >= buffer_size")
	}

	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk_size must be positive")
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("read_timeout must be positive")
	}

	if c.ResponseTimeout <= 0 {
		return fmt.Errorf("response_timeout must be positive")
	}

	if c.CommandTimeout <= 0 {
		return fmt.Errorf("command_timeout must be positive")
	}

	if c.PingInterval <= 0 {
		return fmt.Errorf("ping_interval must be positive")
	}

	// Validate shared secret if provided
	if c.SharedSecret != "" && len(c.SharedSecret) != 64 {
		return fmt.Errorf("invalid shared_secret length: got %d characters, expected 64 (32 bytes hex-encoded)", len(c.SharedSecret))
	}

	return nil
}
