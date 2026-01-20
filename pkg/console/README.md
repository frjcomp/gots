# Console Package - Terminal Session Arbiter

## Overview

The `console` package implements a **Terminal Session Arbiter** that provides exclusive, safe access to the local TTY across multiple operational modes (shell and PTY). This architectural pattern makes "terminal becomes unresponsive after mode switches" impossible by design.

## Problem Solved

**Previous Issue**: When switching between the gotsl interactive shell and remote PTY sessions multiple times, the terminal would become unresponsive and break. This occurred because:

- Multiple goroutines directly accessed stdin/stdout with conflicting deadlines
- Terminal raw/cooked mode transitions were scattered across the codebase
- No central authority managed terminal state during handoffs
- Stale stdin data and escape sequences accumulated after PTY exits
- Readline state became corrupted after returning from raw mode

## Architecture

### Key Components

1. **SessionArbiter**: Single owner of `/dev/tty` and terminal state
2. **State Machine**: `Idle → Shell` or `Idle → Pty`, with atomic transitions
3. **Mode Isolation**: Only arbiter touches TTY; modes receive channels/callbacks
4. **Liveness Guarantees**: Heartbeat monitoring and backpressure detection
5. **Clean Handoffs**: Explicit detach/flush sequence on every mode exit

### State Machine

```
Idle ──EnterShell()──> Shell
  ↑                       ↓
  │                  (can RunPtySession)
  │                       ↓
  └──────────────────── Pty ──> Idle (on exit/error)
```

- **Idle**: Cooked mode, no active session
- **Shell**: Cooked mode, readline owns input/output
- **Pty**: Raw mode, bidirectional bridge to remote PTY

### Invariants

1. Only one mode active at a time (enforced via `ErrAlreadyActive`)
2. Terminal always returns to cooked `Idle` after any session
3. Deadlines and raw mode cleared on detach
4. Watchdog ensures sessions cannot hang indefinitely

## Usage

### Basic Shell Mode

```go
arbiter, _ := console.NewSessionArbiter(log.Printf)
defer arbiter.Close()

_ = arbiter.EnterShell()
// Use readline or other cooked-mode interactions
```

### PTY Session

```go
cfg := console.PtySessionConfig{
    Incoming:          ptyDataChan,  // Remote PTY output
    Send:              sendFunc,      // Forward local input to remote
    SendExit:          exitFunc,      // Cleanup on exit
    HeartbeatInterval: 12 * time.Second,
    ReadDeadline:      120 * time.Millisecond,
    BackpressureLimit: 256,
}

ctx := context.Background()
err := arbiter.RunPtySession(ctx, cfg)
// Arbiter auto-detaches to Idle on context cancel or error
```

### Shell ↔ PTY Switching

```go
// In shell mode
_ = arbiter.EnterShell()

// User requests PTY
_ = arbiter.RunPtySession(ctx, cfg)  // Blocks until exit

// After PTY exits, automatically back in Idle
_ = arbiter.EnterShell()  // Can re-enter shell cleanly
```

## Safety Features

### 1. Heartbeat Watchdog

- Monitors incoming and outgoing activity
- Auto-detaches if no heartbeat for `2 * HeartbeatInterval`
- Prevents hung sessions from wedging the terminal

### 2. Backpressure Detection

- Monitors incoming channel depth
- Terminates session if queue exceeds `BackpressureLimit`
- Prevents memory exhaustion from slow consumers

### 3. Deadline Management

- All TTY reads bounded by `ReadDeadline`
- Allows periodic cancellation checks
- Cleared on detach to avoid interfering with next mode

### 4. Atomic Transitions

- `detachLocked()` waits for all goroutines via `sync.WaitGroup`
- Restores cooked mode before releasing lock
- No race conditions between mode switch and I/O pumps

### 5. Panic Recovery

- Each goroutine has defer/recover to log panics
- Arbiter detaches cleanly even if a pump crashes

## Testing

### Unit Tests

- `TestStateTransitionsShellToPty`: Verify state machine transitions
- `TestHeartbeatTimeoutTriggersDetach`: Watchdog timeout behavior

### Integration Tests

- `TestRapidShellPtySwitch`: 10 rapid shell→pty→shell cycles (reproduces original bug)
- `TestBackpressureTriggersDetach`: Excessive incoming data handling
- `TestConcurrentLeaseRequestsRejected`: Mutual exclusion enforcement

Run tests:
```bash
go test ./pkg/console -v -timeout 30s
```

## Design Decisions

### Why a Single Arbiter?

- **Simplicity**: One source of truth for terminal state
- **Safety**: Eliminates race conditions on raw/cooked transitions
- **Debuggability**: Centralized logging and state inspection

### Why Not Use Channels for Everything?

- Direct TTY reads are more efficient than polling channels
- `term.MakeRaw()` must be on the TTY FD, not a pipe
- Readline library requires direct file access

### Why Heartbeats?

- Detects wedged network connections without user intervention
- Prevents terminal lockup if remote PTY hangs
- Low overhead: only updates `lastBeat` timestamp

### Why Backpressure Limit?

- Protects against DoS via flood of PTY data
- Ensures bounded memory usage
- Fails fast instead of accumulating unbounded queues

## Observability

The arbiter accepts a `logf` function for structured logging:

```go
arbiter, _ := console.NewSessionArbiter(log.Printf)
```

Logged events:
- Mode transitions (Shell, Pty, Idle)
- Watchdog timeouts
- Backpressure trips
- Detach reasons (error, cancel, EOF)

For production, wire this to your structured logger.

## Limitations

- **Single TTY**: Designed for one local terminal; not suitable for multiplexing
- **No Mode Nesting**: Cannot run PTY inside PTY
- **Headless Support**: TTY pumps disabled if `/dev/tty` unavailable (tests/CI)

## Future Enhancements

- **Telemetry**: Emit metrics for attach latency, heartbeat jitter, session duration
- **Configurable Policies**: Per-mode timeouts, max session duration
- **Multi-TTY**: Support multiple terminals with arbiter pool

## References

- Issue: Recurring keyboard freeze after multiple shell↔PTY switches
- Pattern: Single Owner of Shared Resource
- Inspiration: Terminal multiplexers (tmux, screen) use similar arbitration
