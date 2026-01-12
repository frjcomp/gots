# Headless API Reference

The headless API provides HTTP endpoints to programmatically control gotsl and interact with connected clients. This API is designed for automation, testing, and CI/CD integration.

## Overview

- **Base URL**: `http://{control-addr}` (e.g., `http://127.0.0.1:8080`)
- **Content-Type**: `application/json`
- **Default Timeout**: 15 seconds for most operations

## Endpoints

### GET /health

Health check endpoint for service readiness.

**Request:**
```bash
curl -s http://localhost:8081/health
```

**Response (200 OK):**
```
ok
```

**Use Cases:**
- Docker health checks
- Load balancer probes
- Startup verification

---

### GET /clients

List all currently connected clients.

**Request:**
```bash
curl -s http://localhost:8081/clients
```

**Response (200 OK):**
```json
[
  {
    "address": "192.168.1.100:54321",
    "identifier": "client-1",
    "metadata": {
      "hostname": "workstation",
      "os": "Linux",
      "arch": "amd64"
    }
  },
  {
    "address": "192.168.1.101:54322",
    "identifier": "client-2",
    "metadata": {
      "hostname": "server",
      "os": "Linux",
      "arch": "arm64"
    }
  }
]
```

**Response Fields:**
- `address`: Client's network address (IP:port)
- `identifier`: User-defined identifier for the client (from `--client-id` flag)
- `metadata.hostname`: Client's hostname
- `metadata.os`: Operating system
- `metadata.arch`: CPU architecture

**Use Cases:**
- Monitor connected clients
- Verify client connectivity before executing commands
- Collect system information about connected systems

---

### POST /upload

Upload a file from the listener to a connected client.

**Request:**
```bash
curl -s -X POST http://localhost:8081/upload \
  -H "Content-Type: application/json" \
  -d '{
    "local_path": "/etc/passwd",
    "remote_path": "/tmp/passwd-copy",
    "client": ""
  }'
```

**Request Body:**
```json
{
  "local_path": "/path/to/local/file",
  "remote_path": "/path/on/client",
  "client": "192.168.1.100:54321"
}
```

**Request Fields:**
- `local_path` (required): Path to file on listener system to upload
- `remote_path` (required): Destination path on the client
- `client` (optional): Target client address. If omitted, uses first connected client.

**Response (200 OK):**
```json
{
  "status": "success",
  "bytes_sent": 1234,
  "remote_path": "/tmp/passwd-copy"
}
```

**Response Fields:**
- `status`: Operation status (success/failure)
- `bytes_sent`: Original file size in bytes
- `remote_path`: File path on client

**Error Responses:**
- `400 Bad Request`: Missing required fields or file not found
- `503 Service Unavailable`: No clients connected
- `504 Gateway Timeout`: File transfer timeout

**Notes:**
- Files are compressed during transfer for bandwidth efficiency
- Supports binary and text files of any size
- Uses chunked transfer protocol for reliability

---

### POST /download

Download a file from a connected client to the listener.

**Request:**
```bash
curl -s -X POST http://localhost:8081/download \
  -H "Content-Type: application/json" \
  -d '{
    "remote_path": "/etc/hostname",
    "client": ""
  }' \
  --output /tmp/hostname-copy
```

**Request Body:**
```json
{
  "remote_path": "/path/on/client",
  "client": "192.168.1.100:54321"
}
```

**Request Fields:**
- `remote_path` (required): Path to file on client to download
- `client` (optional): Target client address. If omitted, uses first connected client.

**Response (200 OK):**
```
[raw file contents as binary data]
```

**Response Headers:**
- `Content-Type`: application/octet-stream
- `Content-Disposition`: attachment; filename=...
- `Content-Length`: File size in bytes

**Error Responses:**
- `400 Bad Request`: Missing `remote_path` field
- `503 Service Unavailable`: No clients connected
- `504 Gateway Timeout`: File transfer timeout
- `500 Internal Server Error`: Decompression error

**Notes:**
- Response is binary file content (not JSON)
- Client filename is automatically extracted from remote_path
- Files are decompressed automatically by the API

---

### POST /command

Execute a shell command on a connected client and return output.

**Request:**
```bash
curl -s -X POST http://localhost:8081/command \
  -H "Content-Type: application/json" \
  -d '{
    "command": "ls -la /tmp",
    "timeout_ms": 15000,
    "client": ""
  }'
```

**Request Body:**
```json
{
  "command": "shell command to execute",
  "timeout_ms": 15000,
  "client": "192.168.1.100:54321"
}
```

**Request Fields:**
- `command` (required): Shell command to execute
- `timeout_ms` (optional): Timeout in milliseconds (default: 15000)
- `client` (optional): Target client address. If omitted, uses first connected client.

**Response (200 OK):**
```json
{
  "output": "total 48\ndrwxrwxrwt 10 root root ...\n"
}
```

**Error Responses:**
- `400 Bad Request`: Missing `command` field or invalid JSON
- `503 Service Unavailable`: No clients connected
- `504 Gateway Timeout`: Command execution timed out

**Examples:**

Execute command on first available client:
```bash
curl -X POST http://localhost:8081/command \
  -d '{"command": "whoami"}'
```

Execute on specific client with 5-second timeout:
```bash
curl -X POST http://localhost:8081/command \
  -d '{
    "command": "uname -a",
    "timeout_ms": 5000,
    "client": "192.168.1.100:54321"
  }'
```

---

### POST /forward

Start a port forward from the listener to a remote address accessible from a connected client.

**Request:**
```bash
curl -s -X POST http://localhost:8081/forward \
  -H "Content-Type: application/json" \
  -d '{
    "local_port": "18080",
    "remote_addr": "internal-api.local:8080",
    "client": ""
  }'
```

**Request Body:**
```json
{
  "local_port": "18080",
  "remote_addr": "internal-api.local:8080",
  "client": "192.168.1.100:54321"
}
```

**Request Fields:**
- `local_port` (required): Port to bind on the listener (as string)
- `remote_addr` (required): Target address reachable from client (host:port format)
- `client` (optional): Target client address. If omitted, uses first connected client.

**Response (200 OK):**
```json
{
  "id": "fwd-1705081234567890",
  "local_addr": "0.0.0.0:18080",
  "remote_addr": "internal-api.local:8080"
}
```

**Response Fields:**
- `id`: Unique forward identifier
- `local_addr`: Address where listener is listening
- `remote_addr`: Target address being forwarded to

**Error Responses:**
- `400 Bad Request`: Missing required fields or invalid parameters
- `503 Service Unavailable`: No clients connected
- `500 Internal Server Error`: Listener doesn't support forwarding

**Usage Pattern:**
```bash
# 1. Start forward
RESPONSE=$(curl -s -X POST http://localhost:8081/forward \
  -d '{"local_port":"18080","remote_addr":"database:5432"}')
echo $RESPONSE

# 2. Connect to forwarded port
psql -h 127.0.0.1 -p 18080

# 3. Access is tunneled through the client's network
```

---

### POST /socks

Start a SOCKS5 proxy on the listener that routes traffic through a connected client.

**Request:**
```bash
curl -s -X POST http://localhost:8081/socks \
  -H "Content-Type: application/json" \
  -d '{
    "local_port": "1080",
    "client": ""
  }'
```

**Request Body:**
```json
{
  "local_port": "1080",
  "client": "192.168.1.100:54321"
}
```

**Request Fields:**
- `local_port` (required): Port to bind SOCKS5 proxy on (as string)
- `client` (optional): Target client address. If omitted, uses first connected client.

**Response (200 OK):**
```json
{
  "id": "socks-1705081234567890",
  "local_addr": "0.0.0.0:1080"
}
```

**Response Fields:**
- `id`: Unique SOCKS proxy identifier
- `local_addr`: Address where SOCKS5 proxy is listening

**Error Responses:**
- `400 Bad Request`: Missing `local_port` field
- `503 Service Unavailable`: No clients connected
- `500 Internal Server Error`: Listener doesn't support SOCKS

**Usage Pattern:**
```bash
# 1. Start SOCKS proxy
RESPONSE=$(curl -s -X POST http://localhost:8081/socks \
  -d '{"local_port":"1080"}')
echo $RESPONSE

# 2. Configure application to use SOCKS5 at localhost:1080
# For curl:
curl -x socks5://127.0.0.1:1080 https://internal-site.local

# For Firefox: Settings > Network > SOCKS Host: 127.0.0.1, Port: 1080

# 3. All traffic is routed through the client
```

---

### POST /shutdown

Gracefully shutdown the headless API server.

**Request:**
```bash
curl -s -X POST http://localhost:8081/shutdown
```

**Response (200 OK):**
```
shutting down
```

**Behavior:**
- Initiates graceful shutdown of the headless API
- Existing connections are allowed to complete
- Listener continues running (unless explicitly stopped)

---

## Error Handling

All endpoints return standard HTTP status codes:

| Status | Meaning |
|--------|---------|
| 200 | Success |
| 400 | Bad request (invalid JSON, missing fields) |
| 405 | Method not allowed (e.g., GET on POST-only endpoint) |
| 503 | Service unavailable (no clients connected) |
| 504 | Gateway timeout (command execution timeout) |

Error responses include a message in the body:
```bash
curl -s http://localhost:8081/command
# {"output":""}
# Status: 400
# Body: invalid json
```

---

## Common Patterns

### Wait for Client Connection

```bash
#!/bin/bash
ENDPOINT="http://localhost:8081"

# Poll until client connects
until curl -s "$ENDPOINT/clients" | grep -q "address"; do
  echo "Waiting for client..."
  sleep 1
done

echo "Client connected!"
```

### Execute with Retry

```bash
#!/bin/bash
ENDPOINT="http://localhost:8081"
MAX_RETRIES=3
RETRY_DELAY=2

execute_with_retry() {
  local cmd=$1
  for ((i=1; i<=MAX_RETRIES; i++)); do
    result=$(curl -s -X POST "$ENDPOINT/command" \
      -d "{\"command\":\"$cmd\"}")
    if [[ $? -eq 0 ]]; then
      echo "$result"
      return 0
    fi
    echo "Attempt $i failed, retrying in ${RETRY_DELAY}s..."
    sleep $RETRY_DELAY
  done
  return 1
}

execute_with_retry "uname -a"
```

### Upload and Download Files

```bash
#!/bin/bash
ENDPOINT="http://localhost:8081"

# Upload a file to the client
echo "Uploading file..."
curl -s -X POST "$ENDPOINT/upload" \
  -d '{
    "local_path": "/etc/hostname",
    "remote_path": "/tmp/hostname-copy"
  }'

# Download the file back
echo "Downloading file..."
curl -s -X POST "$ENDPOINT/download" \
  -H "Content-Type: application/json" \
  -d '{"remote_path": "/tmp/hostname-copy"}' \
  --output /tmp/downloaded-hostname

# Verify
cat /tmp/downloaded-hostname
```

### Setup Forward and Test Connectivity

```bash
#!/bin/bash
ENDPOINT="http://localhost:8081"

# Start forward to internal database
FORWARD=$(curl -s -X POST "$ENDPOINT/forward" \
  -d '{"local_port":"5432","remote_addr":"db.internal:5432"}')

LOCAL_ADDR=$(echo "$FORWARD" | jq -r '.local_addr')
echo "Database forwarded to: $LOCAL_ADDR"

# Test connectivity
if psql -h 127.0.0.1 -p 5432 -c "SELECT 1"; then
  echo "Forward is working!"
else
  echo "Forward failed!"
fi
```

### Route Traffic Through SOCKS

```bash
#!/bin/bash
ENDPOINT="http://localhost:8081"

# Start SOCKS proxy
SOCKS=$(curl -s -X POST "$ENDPOINT/socks" \
  -d '{"local_port":"1080"}')

echo "SOCKS proxy at: $(echo "$SOCKS" | jq -r '.local_addr')"

# Use with curl
curl -x socks5://127.0.0.1:1080 https://internal-service.local/api

# Or configure system-wide proxy
export https_proxy=socks5://127.0.0.1:1080
export http_proxy=socks5://127.0.0.1:1080
```

---

## Security Considerations

- **Network Binding**: By default, the control API binds to `127.0.0.1` (localhost only)
- **Unrestricted Access**: If bound to `0.0.0.0`, the API provides full system control
- **Authentication**: Currently no built-in authentication; rely on network isolation
- **HTTPS**: Use a reverse proxy (nginx, etc.) to add TLS/authentication in production

**Recommended Setup:**
```bash
# Local testing only - safe
./gotsl --headless --control-addr 127.0.0.1:8080

# Production - restrict with firewall + reverse proxy
./gotsl --headless --control-addr 0.0.0.0:8080
# Then use: nginx reverse proxy with authentication
```
