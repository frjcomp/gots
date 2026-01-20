package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// Mode represents a terminal ownership mode.
type Mode string

const (
	ModeIdle  Mode = "idle"
	ModeShell Mode = "shell"
	ModePty   Mode = "pty"
)

// ErrAlreadyActive is returned when a mode is requested while another is active.
var ErrAlreadyActive = errors.New("session arbiter already has an active lease")

// PtySessionConfig bundles the callbacks and channels needed to run a PTY session.
type PtySessionConfig struct {
	// Incoming carries bytes from the remote PTY that should be written to the local TTY.
	Incoming <-chan []byte
	// Send pushes local TTY input bytes to the remote endpoint. It must be non-blocking
	// or respect the provided context.
	Send func([]byte) error
	// SendExit is invoked when the local side exits the PTY; optional.
	SendExit func() error

	// ReadDeadline bounds blocking reads on the TTY so we can observe cancellation.
	ReadDeadline time.Duration
	// HeartbeatInterval controls how often we require activity before timing out.
	HeartbeatInterval time.Duration
	// BackpressureLimit drops the session if the incoming queue is too full.
	BackpressureLimit int
	// DisableLocalIO skips TTY pumps (used for tests or headless runs without a real TTY).
	DisableLocalIO bool
}

// SessionArbiter owns the local TTY and arbitrates exclusive access between shell and PTY modes.
type SessionArbiter struct {
	mu     sync.Mutex
	state  Mode
	tty    *os.File
	cooked *term.State
	hasTTY bool

	cancel   context.CancelFunc
	wg       sync.WaitGroup
	lastBeat time.Time

	logf func(string, ...interface{})
}

// NewSessionArbiter opens /dev/tty (falling back to stdin/stdout) and prepares to arbitrate.
func NewSessionArbiter(logf func(string, ...interface{})) (*SessionArbiter, error) {
	// Check if stdin is a terminal first
	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))
	
	var tty *os.File
	var hasTTY bool
	
	// Only try to use /dev/tty if stdin is also a terminal
	// This ensures we read from the same source the process is receiving input from
	if stdinIsTTY {
		var err error
		tty, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			// Fallback: use stdin if /dev/tty unavailable
			tty = os.Stdin
		}
		hasTTY = term.IsTerminal(int(tty.Fd()))
	} else {
		// stdin is not a terminal (pipe/redirect), so use it directly
		tty = os.Stdin
		hasTTY = false
	}
	
	var cooked *term.State
	if hasTTY {
		st, terr := term.GetState(int(tty.Fd()))
		if terr == nil {
			cooked = st
		}
	}

	return &SessionArbiter{
		state:  ModeIdle,
		tty:    tty,
		cooked: cooked,
		hasTTY: hasTTY,
		logf:   logf,
	}, nil
}

// TTY returns the underlying file for readline or logging paths. Callers must not change modes themselves.
func (a *SessionArbiter) TTY() *os.File {
	return a.tty
}

// EnterShell transitions the arbiter to shell mode (cooked). Safe to call repeatedly.
func (a *SessionArbiter) EnterShell() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == ModeShell {
		return nil
	}

	if err := a.detachLocked(); err != nil {
		return err
	}

	if a.hasTTY && a.cooked != nil {
		_ = term.Restore(int(a.tty.Fd()), a.cooked)
	}

	a.state = ModeShell
	return nil
}

// RunPtySession enters PTY mode and runs until context cancellation or error.
func (a *SessionArbiter) RunPtySession(ctx context.Context, cfg PtySessionConfig) error {
	a.mu.Lock()
	if a.state == ModePty {
		a.mu.Unlock()
		return ErrAlreadyActive
	}
	if err := a.detachLocked(); err != nil {
		a.mu.Unlock()
		return err
	}

	if a.hasTTY {
		if st, err := term.MakeRaw(int(a.tty.Fd())); err == nil {
			a.cooked = st
		} else {
			a.mu.Unlock()
			return fmt.Errorf("make raw: %w", err)
		}
	}

	a.state = ModePty
	a.lastBeat = time.Now()

	// Prepare pumps
	readDeadline := cfg.ReadDeadline
	if readDeadline == 0 {
		readDeadline = 150 * time.Millisecond
	}
	heartbeatInterval := cfg.HeartbeatInterval
	if heartbeatInterval == 0 {
		heartbeatInterval = 15 * time.Second
	}
	backpressureLimit := cfg.BackpressureLimit
	if backpressureLimit == 0 {
		backpressureLimit = 256
	}

	pctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	inputErr := make(chan error, 1)
	outputErr := make(chan error, 1)

	if cfg.DisableLocalIO {
		// Headless/test mode: drop incoming data and wait for cancellation.
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			for {
				select {
				case <-pctx.Done():
					return
				case <-cfg.Incoming:
					// drop
				}
			}
		}()
	} else if a.hasTTY {
		// Real TTY: read with deadlines
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			buf := make([]byte, 4096)
			for {
				if readDeadline > 0 {
					_ = a.tty.SetReadDeadline(time.Now().Add(readDeadline))
				}
				n, err := a.tty.Read(buf)
				if n > 0 {
					a.markHeartbeat()
					if serr := cfg.Send(buf[:n]); serr != nil {
						inputErr <- serr
						return
					}
				}
				if err != nil {
					if isTimeout(err) {
						select {
						case <-pctx.Done():
							return
						default:
							continue
						}
					}
					inputErr <- err
					return
				}
				select {
				case <-pctx.Done():
					return
				default:
				}
			}
		}()
	} else {
		// Non-TTY (like pipe/file): read stdin and forward to remote.
		// This is called during PTY mode only.When PTY ends, the context
		// is cancelled and we need to exit immediately WITHOUT reading more stdin.
		// We check context multiple times per loop to prevent consuming input.
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			buf := make([]byte, 4096)
			for {
				// Check 1: Before any blocking operation
				select {
				case <-pctx.Done():
					return
				default:
				}
				
				// Set a short read timeout so we can check context frequently
				_ = a.tty.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
				n, err := a.tty.Read(buf)
				
				// Check 2: Immediately after read returns
				select {
				case <-pctx.Done():
					return
				default:
				}
				
				if n > 0 {
					a.markHeartbeat()
					if serr := cfg.Send(buf[:n]); serr != nil {
						inputErr <- serr
						return
					}
				}
				
				// Check 3: After send operation
				select {
				case <-pctx.Done():
					return
				default:
				}
				
				if err != nil {
					if isTimeout(err) {
						// Timeout expected - loop back to check context
						continue
					}
					// Real error - exit
					inputErr <- err
					return
				}
			}
		}()
	}

	// Write pump: Incoming -> TTY (always needed, regardless of TTY status)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-pctx.Done():
				return
			case data, ok := <-cfg.Incoming:
				if !ok {
					// Channel closed by remote - print message and exit (unless headless)
					// Write to stdout (not stdin/tty) to ensure it's captured
					if !cfg.DisableLocalIO {
						fmt.Fprintf(os.Stdout, "\r\n[Remote shell exited]\r\n")
					}
					outputErr <- io.EOF
					return
				}
				if len(data) == 0 {
					continue
				}
				if len(cfg.Incoming) > backpressureLimit {
					outputErr <- fmt.Errorf("backpressure: incoming queue exceeded limit %d", backpressureLimit)
					return
				}
				a.markHeartbeat()
				if _, err := a.tty.Write(data); err != nil {
					// Write failed (e.g., stdin invalid in CI/headless mode).
					// Only print message if we're not in headless mode.
					if !cfg.DisableLocalIO {
						fmt.Fprintf(os.Stdout, "\r\n[Remote shell exited]\r\n")
					}
					outputErr <- io.EOF
					return
				}
			}
		}
	}()

	if !a.hasTTY || cfg.DisableLocalIO {
		// Skip TTY-specific read pump for headless/non-TTY modes
		// (TTY read pump or non-TTY read pump already started above)
	} else {
		// TTY read pump with deadlines
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			buf := make([]byte, 4096)
			for {
				if readDeadline > 0 {
					_ = a.tty.SetReadDeadline(time.Now().Add(readDeadline))
				}
				n, err := a.tty.Read(buf)
				if n > 0 {
					a.markHeartbeat()
					if serr := cfg.Send(buf[:n]); serr != nil {
						inputErr <- serr
						return
					}
				}
				if err != nil {
					if isTimeout(err) {
						select {
						case <-pctx.Done():
							return
						default:
							continue
						}
					}
					inputErr <- err
					return
				}
				select {
				case <-pctx.Done():
					return
				default:
				}
			}
		}()
	}

	// Watchdog
	watchdogErr := make(chan error, 1)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case <-ticker.C:
				if time.Since(a.lastBeat) > 2*heartbeatInterval {
					watchdogErr <- fmt.Errorf("heartbeat timeout after %s", 2*heartbeatInterval)
					return
				}
			}
		}
	}()

	a.mu.Unlock()

	// Wait for completion or errors
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case e := <-inputErr:
		err = e
	case e := <-outputErr:
		err = e
	case e := <-watchdogErr:
		err = e
	}

	// Cancel context to signal all goroutines to exit
	if a.cancel != nil {
		a.cancel()
	}

	// Wait for all goroutines to finish BEFORE restoring stdin mode
	// The input pump goroutine still needs stdin to be in non-blocking mode
	// while it's running. Once all goroutines are done, stdin is no longer
	// being accessed, so it's safe to change its mode back to blocking.
	a.wg.Wait()

	a.mu.Lock()
	if cfg.SendExit != nil {
		_ = cfg.SendExit()
	}
	// Clear any read deadline set during input pump operations
	_ = a.tty.SetReadDeadline(time.Time{})
	a.detachLocked()
	a.mu.Unlock()

	return err
}

// Close releases resources and restores cooked mode.
func (a *SessionArbiter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.detachLocked()
}

// State returns the current mode; useful for tests and diagnostics.
func (a *SessionArbiter) State() Mode {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// markHeartbeat records activity.
func (a *SessionArbiter) markHeartbeat() {
	a.mu.Lock()
	a.lastBeat = time.Now()
	a.mu.Unlock()
}

// detachLocked cancels the active mode and restores cooked state. Caller must hold a.mu.
func (a *SessionArbiter) detachLocked() error {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.mu.Unlock()
	a.wg.Wait()
	a.mu.Lock()

	if a.hasTTY && a.cooked != nil {
		_ = term.Restore(int(a.tty.Fd()), a.cooked)
	}
	if a.hasTTY {
		// Clear read deadlines
		_ = a.tty.SetReadDeadline(time.Time{})
	}

	a.state = ModeIdle
	return nil
}

func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	if te, ok := err.(timeout); ok {
		return te.Timeout()
	}
	return false
}
