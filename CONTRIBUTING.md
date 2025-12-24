# Contributing to GOTS

Thank you for your interest in contributing to GOTS! This guide will help you get started with development.

## Getting Started

### Option 1: GitHub Codespaces (Recommended)

GitHub Codespaces provides a pre-configured development environment in your browser:

1. **Fork the repository** (if you don't have write access)
2. **Open Codespaces**:
   - Click the green `<> Code` button on your fork
   - Select the `Codespaces` tab
   - Click "Create codespace on main"
3. **Wait for the environment to initialize** (usually ~2-3 minutes)
4. You're ready to develop!

### Option 2: Local Development

1. **Clone the repository**:
   ```bash
   git clone https://github.com/frjcomp/gots.git
   cd gots
   ```

2. **Ensure you have Go 1.21+**:
   ```bash
   go version
   ```

3. **Install dependencies**:
   ```bash
   go mod download
   ```

## Quick Start - Testing Both Binaries

### Using Make (Recommended)

The easiest way to test both binaries together:

**Terminal 1 - Start the listener (gotsl)**:
```bash
make l
```
This runs: `./bin/gotsl --port 9001 --interface 0.0.0.0`

**Terminal 2 - Start the client (gotsr)**:
```bash
make r
```
This runs: `./bin/gotsr --target 127.0.0.1:9001 --retries 0`

You should see both binaries connect and establish a session. In the listener terminal, you can run commands like `ls`, `shell 1`, etc.

### Using `go run` (Alternative)

If you prefer to run directly without building:

**Terminal 1 - Start the listener**:
```bash
go run ./cmd/gotsl --port 9001 --interface 0.0.0.0
```

**Terminal 2 - Start the client**:
```bash
go run ./cmd/gotsr --target 127.0.0.1:9001 --retries 0
```

### Testing with Shared Secret Authentication

**Terminal 1 - Start listener with shared secret**:
```bash
go run ./cmd/gotsl -s --port 9001 --interface 0.0.0.0
```

This will print the shared secret. Copy the secret from the output.

**Terminal 2 - Start client with secret**:
```bash
go run ./cmd/gotsr -s <PASTE_SECRET_HERE> --target 127.0.0.1:9001 --retries 0
```

## Common Development Commands

### Building

```bash
# Build both binaries
make build

# Binaries are placed in ./bin/
ls -la bin/
```

### Testing

```bash
# Run all tests
make test

# Run with race condition detector
go test -race ./...

# Run only integration tests
go test -race ./integration

# Run tests with coverage
make cover
# Opens coverage.html in your browser
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make vet

# Run both fmt and vet
make fmt vet
```

### Dependencies

```bash
# Update and clean dependencies
make mod

# Or manually
go mod tidy
```

### Cleanup

```bash
# Remove built binaries and coverage files
make clean
```

### View All Targets

```bash
make help
```

## Project Structure

```
.
├── cmd/
│   ├── gotsl/          # Listener (server) implementation
│   └── gotsr/          # Client implementation
├── pkg/
│   ├── client/         # Client protocol logic
│   ├── certs/          # Certificate generation and management
│   ├── compression/    # Data compression utilities
│   ├── config/         # Configuration management
│   ├── protocol/       # Protocol constants and definitions
│   ├── server/         # Server listener logic
│   └── version/        # Version information
├── integration/        # End-to-end integration tests
├── examples/           # Example scripts (PowerShell, etc.)
├── Makefile           # Build automation
└── README.md          # Project documentation
```

## Understanding the Code

### Main Entry Points
- **gotsl (Listener)**: `cmd/gotsl/main.go`
  - Starts a TLS listener
  - Manages connected clients
  - Provides interactive shell commands

- **gotsr (Client)**: `cmd/gotsr/main.go`
  - Connects to a gotsl listener
  - Maintains connection with retries
  - Executes remote commands

### Key Packages

**pkg/server/** - Server-side connection handling
- `Listener` manages multiple client connections
- Handles commands (shell, file transfer, etc.)

**pkg/client/** - Client-side connection handling
- `ReverseClient` manages the connection to listener
- Implements command handlers for various operations

**pkg/config/** - Configuration system
- `ServerConfig` and `ClientConfig` structs
- Environment variable overrides (GOTS_* prefix)
- Configuration validation

**pkg/protocol/** - Protocol definitions
- Command constants
- Buffer sizes and timeouts
- Protocol markers (END_OF_OUTPUT, etc.)

## CLI Flags Reference

### gotsl (Listener)

```bash
./bin/gotsl --port PORT --interface INTERFACE [--shared-secret]
```

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--port` | string | Yes | Port to listen on (e.g., 9001) |
| `--interface` | string | Yes | Network interface to bind to (e.g., 0.0.0.0) |
| `-s, --shared-secret` | bool | No | Enable shared secret authentication |

### gotsr (Client)

```bash
./bin/gotsr --target HOST:PORT --retries NUM [-s SECRET] [--cert-fingerprint HASH]
```

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--target` | string | Yes | Target server (host:port) |
| `--retries` | int | Yes | Max retries (0 = infinite) |
| `-s, --shared-secret` | string | No | Shared secret for authentication |
| `--cert-fingerprint` | string | No | Server certificate SHA256 fingerprint |

## Environment Variables

All configuration can be overridden via environment variables (GOTS_* prefix):

```bash
# Server config
export GOTS_PORT=8080
export GOTS_NETWORK_INTERFACE=192.168.1.100
export GOTS_BUFFER_SIZE=2097152
export GOTS_MAX_BUFFER_SIZE=20971520

# Client config
export GOTS_TARGET=listener.example.com:9001
export GOTS_MAX_RETRIES=10
export GOTS_SHARED_SECRET=<hex_secret>
export GOTS_CERT_FINGERPRINT=<sha256_hash>

# Timeouts (duration format: "5s", "30ms", etc.)
export GOTS_READ_TIMEOUT=2s
export GOTS_RESPONSE_TIMEOUT=10s
export GOTS_COMMAND_TIMEOUT=180s
export GOTS_PING_INTERVAL=30s
```

## Making Changes

### Before Committing

1. **Run tests**:
   ```bash
   go test -race ./...
   ```

2. **Format code**:
   ```bash
   make fmt
   ```

3. **Check for issues**:
   ```bash
   make vet
   ```

4. **Verify coverage** (for significant changes):
   ```bash
   make cover
   ```

### Commit Guidelines

- Keep commits focused on a single concern
- Use clear, descriptive commit messages
- Include test cases for new features
- Ensure all tests pass before pushing

### Pull Request Process

1. Create a feature branch: `git checkout -b feature/your-feature`
2. Make your changes and commit
3. Push to your fork: `git push origin feature/your-feature`
4. Open a Pull Request on GitHub
5. Ensure CI checks pass
6. Request review from maintainers