# Journey Playbook System

## Overview

The journey playbook system provides a declarative, extensible way to define end-to-end test scenarios as YAML files. Instead of hardcoding test logic in Go, you define a series of steps with assertions, allowing tests to be versioned and modified without code changes.

## Files

- **[integration/journeys/default.yaml](../integration/journeys/default.yaml)** - YAML configuration defining the test journey
- **[integration/journey.go](../integration/journey.go)** - Go implementation of playbook loader and executor
- **[integration/headless_e2e_test.go](../integration/headless_e2e_test.go)** - Test that loads and runs the journey

## YAML Schema

### Top-level fields
- `name`: (string) Human-readable name of the journey
- `description`: (string) What this journey tests
- `steps`: (array) List of test steps to execute

### Step types

#### Command Step
Executes a shell command and validates the output.

```yaml
- name: "step name"
  type: "command"
  command: "shell command to run"
  timeout_ms: 15000  # optional, defaults to 15000
  assertions:
    - type: "contains"
      expected: "text to find"
      description: "Human-readable assertion description"
```

#### Forward Step
Starts a port forward from listener to a remote address, then fetches via HTTP.

```yaml
- name: "step name"
  type: "forward"
  local_port: "18080"
  remote_addr: "backend:8080"
  fetch_url: "http://gotsl:18080/"
  assertions:
    - type: "contains"
      expected: "expected response"
      description: "Assertion description"
```

#### SOCKS Step
Starts a SOCKS5 proxy and fetches a target URL through it.

```yaml
- name: "step name"
  type: "socks"
  local_port: "1080"
  target_url: "http://backend:8080/"
  assertions:
    - type: "contains"
      expected: "expected response"
      description: "Assertion description"
```

#### Upload Step (new)
Uploads a file from the listener to the client using the headless `/upload` API.

Fields:
- `local_path` (string): Absolute path on the listener host/container.
- `remote_path` (string): Destination path on the client.
- `client` (string, optional): Explicit client to target. If omitted, the first client is used.

Example:
```yaml
- name: "upload hosts"
  type: "upload"
  local_path: "/etc/hosts"
  remote_path: "/tmp/uploaded-hosts"
  assertions:
    - type: "contains"
      expected: "success"
      description: "upload reported success"
```

#### Download Step (new)
Downloads a file content from the client using the headless `/download` API.

Fields:
- `remote_path` (string): Source path on the client to fetch.
- `client` (string, optional): Explicit client to target. If omitted, the first client is used.

Example:
```yaml
- name: "download hosts"
  type: "download"
  remote_path: "/etc/hosts"
  assertions:
    - type: "contains"
      expected: "localhost"
      description: "downloaded content contains localhost"
```

### Assertion Types

- `contains` - Case-sensitive substring match
- `contains_ci` - Case-insensitive substring match
- `equals` - Exact match (whitespace trimmed)
- `equals_ci` - Exact match, case-insensitive

## How It Works

1. **Load**: `LoadJourney()` reads and parses the YAML file, unmarshaling steps into strongly-typed Go structs (CommandStep, ForwardStep, SocksStep, UploadStep, DownloadStep).

2. **Execute**: `ExecuteJourney()` iterates through steps and calls the appropriate executor based on step type.

3. **Validate**: Each step validates its result against assertions using `validateAssertion()`, which supports multiple assertion types.

4. **Report**: Test logs show journey name, step names, and assertion results with ✓ indicators.

## Example Output

```
Executing journey: Full Feature Journey
  Description: Tests core headless API functionality: commands, port forward, SOCKS proxy
Step 1: echo command (command)
    ✓ echo output should contain command text
Step 2: list root directory (command)
    ✓ ls / output should contain bin directory
Step 3: detect OS (command)
    ✓ uname should report linux (case-insensitive)
Step 4: port forward to backend (forward)
    Forward started: 0.0.0.0:18080
    ✓ forwarded HTTP response should contain backend message
Step 5: SOCKS proxy to backend (socks)
    SOCKS proxy started: 0.0.0.0:1080
    ✓ SOCKS-proxied HTTP response should contain backend message
✓ Journey completed: Full Feature Journey
```

## Extensibility

The system is designed to be extended with new step types:

1. Define a new step struct (e.g., `FileTransferStep`)
2. Add unmarshaling logic in `LoadJourney()` switch statement
3. Add execution handler in `ExecuteJourney()` switch statement
4. Update YAML schema documentation

New assertion types can be added by extending the switch in `validateAssertion()`.

## Usage

Run the test with Docker Compose:

```bash
docker compose -f integration/docker-compose/docker-compose.e2e.yml up --build tester --abort-on-container-exit
```

Or run locally (requires headless endpoint):

```bash
HEADLESS_ENDPOINT=http://localhost:8081 go test ./integration -run HeadlessE2E -v
```

## Benefits

- **Declarative**: Test scenarios are defined as data, not code
- **Reusable**: Multiple tests can reference the same journey
- **Versioned**: Journeys live in version control alongside code
- **Maintainable**: Non-developers can add test scenarios via YAML
- **Debuggable**: Structured logging shows exactly what each step does

## Related Documentation

- **[Headless API Reference](API.md)** - Complete API endpoint documentation with examples
