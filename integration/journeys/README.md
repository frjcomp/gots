# Journey Playbooks

This directory contains YAML-based test journeys that verify gots functionality through the headless HTTP API.

## Available Journeys

### 1. default.yaml - Comprehensive Feature Test ✅
**Purpose**: Main e2e test verifying core functionality

**What it tests**:
- **Client Verification**: Confirms commands execute on gotsr client (not listener)
  - Step 1: Verifies client is connected via `/clients` endpoint
  - Step 2: Confirms gotsr binary present on client
  - Step 3: Confirms gotsr process running with --target flag
  
- **Shell Command Execution**: Commands execute on connected client
  - Proves shell commands pass through headless API to client and back
  
- **File Transfer**: Upload and download files to/from client
  - Upload: Sends file from tester to client
  - Download: Retrieves file from client back to tester
  - Integrity: Verifies content matches after round-trip
  
- **Port Forwarding**: Access unreachable services through client's network
  - Setup: Create forward from listener to backend service
  - Verification: Use curl to test forward works
  
- **SOCKS Proxy**: Access services via SOCKS5 tunneling
  - Setup: Create SOCKS proxy on listener
  - Verification: Use curl to test proxy connectivity

**Running**:
```bash
docker compose -f integration/docker-compose/docker-compose.e2e.yml up tester
# Or with custom endpoint:
export HEADLESS_ENDPOINT="http://localhost:8081"
go test -v ./integration -run TestHeadlessE2E
```

**11 Total Steps**: ✓ All pass

---

### 2. cert-verification.yaml - Certificate Validation Test 📋
**Purpose**: Verify certificate fingerprint pinning and TLS validation

**What it tests**:
- **Certificate Validation**: Client validates server TLS certificate
- **Fingerprint Pinning**: Confirms client rejects certificate mismatches
- **MITM Prevention**: Demonstrates protection against man-in-the-middle attacks
- **Authenticated Connections**: Commands only work with valid certificates

**How it works**:
1. Client (gotsr) connects with TLS to listener (gotsl)
2. During TLS handshake, server certificate is exchanged
3. If --cert-fingerprint flag provided, client validates certificate SHA256
4. Only after validation succeeds can commands execute

**Security properties**:
- Without pinning: Self-signed cert warning shown, connection allowed
- With correct pinning: Connection succeeds (fingerprint matches)
- With wrong pinning: Connection fails immediately (fingerprint mismatch)
- MITM scenario: Attacker certificate rejected even if valid TLS

**Running**:
```bash
# Listener starts with self-signed cert
./gotsl --headless --port 9001 --interface 0.0.0.0

# Extract fingerprint from listener output:
# "Certificate generated successfully (SHA256: abc123...)"

# Client connects with fingerprint pinning
./gotsr --target localhost:9001 --cert-fingerprint abc123... --retries 5

# Run test
export HEADLESS_ENDPOINT="http://localhost:8081"
# Test would verify connection and commands work
```

**Notes**:
- Certificate validation is automatic in TLS handshake
- Fingerprint pinning is optional but recommended for security
- See `pkg/certs` for certificate generation and validation logic

---

### 3. shared-secret.yaml - Authentication Test 🔐
**Purpose**: Verify shared secret handshake and command authentication

**What it tests**:
- **Secret Handshake**: Listener and client share hex-encoded secret
- **Authentication Flow**: Command execution requires valid secret
- **Unauthorized Rejection**: Wrong or missing secrets rejected
- **Secure Communication**: Commands only work after auth succeeds

**How it works**:
1. Start listener with `-s` flag (generates random secret)
2. Listener prints hex-encoded secret and full gotsr command
3. Start client with `-s <hex-secret>` flag
4. During connection, client sends hash of secret
5. Listener validates hash - only then commands work

**Authentication flow**:
```
Client connects (TLS)
     ↓
TLS handshake completes
     ↓
Client sends: HMAC-SHA256(secret)
     ↓
Listener validates hash
     ↓
Connection authenticated - commands enabled
```

**Security properties**:
- Commands fail with "authentication failed" if secret wrong
- Listener logs warning: "Client X failed authentication: invalid secret"
- Network-accessible listener can't be exploited without secret
- Even with network access, commands only work with correct secret

**Running**:
```bash
# Start listener with shared secret
./gotsl -s --headless --port 9001 --interface 0.0.0.0
# Output: Secret (hex): a1b2c3d4e5f6g7h8...

# Copy the full command or extract secret
SECRET="a1b2c3d4e5f6g7h8..."

# Start client with secret
./gotsr -s $SECRET --target localhost:9001 --retries 5

# Commands execute only with authenticated connection
```

**Notes**:
- Listener logs the secret when started with `-s`
- Client command printed by listener includes correct secret
- Invalid secret causes immediate connection failure in logs
- See `pkg/protocol` for authentication protocol details

---

## YAML Schema Reference

### Common Fields
- `name`: Step display name
- `type`: Step type (command, upload, download, forward, socks)
- `assertions`: List of validations

### Assertion Types
- `contains`: Substring match (case-sensitive)
- `contains_ci`: Substring match (case-insensitive)
- `equals`: Exact match (case-sensitive)
- `equals_ci`: Exact match (case-insensitive)

### Step Types

**Command**
```yaml
- name: "execute command"
  type: "command"
  command: "shell command here"
  assertions:
    - type: "contains"
      expected: "expected output"
```

**Upload**
```yaml
- name: "upload file"
  type: "upload"
  local_file: "/path/to/local/file"
  remote_path: "/path/on/client"
  assertions:
    - type: "contains"
      expected: "confirmation text"
```

**Download**
```yaml
- name: "download file"
  type: "download"
  remote_path: "/path/on/client"
  local_file: "/path/to/save"
  assertions:
    - type: "contains"
      expected: "file content"
```

**Forward**
```yaml
- name: "port forward"
  type: "forward"
  local_port: "8080"
  remote_addr: "target:9000"
  fetch_url: "http://localhost:8080/"
  assertions:
    - type: "contains"
      expected: "response content"
```

**SOCKS**
```yaml
- name: "SOCKS proxy"
  type: "socks"
  local_port: "1080"
  target_url: "http://target:8080/"
  assertions:
    - type: "contains"
      expected: "response content"
```

## Global Configuration

Each journey supports:
- `name`: Display name
- `description`: What the journey tests
- `default_timeout_ms`: Timeout for all steps (default 15000)

---

## Testing Workflow

1. **Start listener** (gotsl):
   ```bash
   ./gotsl --headless --control-addr 127.0.0.1:8081 --port 9001 --interface 0.0.0.0
   ```

2. **Start client** (gotsr):
   ```bash
   ./gotsr --target localhost:9001 --retries 5
   ```

3. **Run journey** (tester):
   ```bash
   export HEADLESS_ENDPOINT="http://localhost:8081"
   go test -v ./integration -run TestHeadlessE2E
   ```

## Test Execution Order

The headless_e2e_test.go:
1. Discovers journey YAML (default.yaml or specified path)
2. Waits for listener to be healthy (/health endpoint)
3. Waits for client to connect (/clients endpoint)
4. Executes journey steps sequentially
5. Validates assertions after each step
6. Reports overall pass/fail

## Security Considerations

- ✅ Headless API is disabled by default (requires --headless flag)
- ✅ Control API binds to localhost by default
- ✅ Security warnings logged when headless is enabled
- ✅ Certificate validation prevents MITM attacks
- ✅ Shared secret authentication prevents unauthorized access
- ✅ Commands execute on remote client (not listener)

See [../README.md](../README.md#headless-mode-with-http-control-api) for headless mode security details.
