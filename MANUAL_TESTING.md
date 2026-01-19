# Manual Testing Checklist

This document contains critical manual tests that **MUST** be performed before any release.
These tests cannot be automated due to technical limitations (TTY requirements, platform-specific behavior, etc.).

## Critical: Keyboard Freeze After PTY Exit

**Issue**: Keyboard input freezes after exiting PTY shells, especially Windows shells  
**Impact**: Makes the tool completely unusable  
**Frequency**: Occurs often when switching between Windows and Linux shells  
**Related Code**: `cmd/gotsl/main.go` - `resetReadlineAfterPty()` function

### Test Procedure

1. **Setup**
   ```bash
   # Start gotsl on a real terminal (not CI, not automated test)
   gotsl --port 8443 --interface 0.0.0.0
   ```

2. **Connect Clients**
   - Connect at least one Linux client
   - Connect at least one Windows client
   - Verify both appear with: `ls`

3. **Test Linux Shell** (Baseline)
   ```
   gotsl> shell 1
   # In PTY shell: run commands (ls, pwd, whoami)
   # Exit shell: exit
   # VERIFY: Can type at gotsl> prompt - keyboard is responsive
   ```

4. **Test Windows Shell** (Critical Test)
   ```
   gotsl> shell 2
   # In PTY shell: run commands (dir, cd, echo test)
   # Exit shell: exit
   # VERIFY: Can type at gotsl> prompt - keyboard is NOT frozen
   # THIS IS THE CRITICAL TEST POINT
   ```

5. **Test Multiple Switches**
   - Repeat steps 3-4 at least 5 times
   - Switch: Linux → Windows → Linux → Windows → Linux
   - After EACH exit, verify keyboard is responsive
   - If keyboard freezes at any point, **TEST FAILS**

6. **Test Recovery (Ctrl+L)**
   - If keyboard ever seems stuck, try pressing `Ctrl+L`
   - Should clear screen and restore input
   - If Ctrl+L doesn't work, **TEST FAILS**

### Success Criteria

- ✅ Keyboard remains responsive after every shell exit
- ✅ No freeze when switching between Windows and Linux shells
- ✅ Can switch shells 10+ times without issues
- ✅ Ctrl+L recovers from any temporary stuck state

### Failure Indicators

- ❌ Cannot type at `gotsl>` prompt after exiting Windows shell
- ❌ Need to restart gotsl to regain keyboard input
- ❌ Ctrl+L doesn't recover stuck keyboard
- ❌ Problem gets worse with more shell switches

### If Test Fails

Check these components in `cmd/gotsl/main.go`:

1. **`resetReadlineAfterPty()` function** - Must be called after `enterPtyShell()`
2. **Terminal state restore** - Must call `term.GetState()` and `term.Restore()`
3. **ANSI reset sequences** - Must include DECSTR (`\x1b[!p`) and mode disables
4. **stdin cleanup** - Must drain input and clear deadlines
5. **readline refresh** - Must call `rl.Refresh()` after reset

### Related Files

- Test documentation: `cmd/gotsl/main_test.go` - `TestReadlineRefreshAfterPtyExit`
- Implementation: `cmd/gotsl/main.go` - `resetReadlineAfterPty()`, `enterPtyShell()`
- Platform-specific: `cmd/gotsl/tty_flush_*.go`

---

## Other Manual Tests

### PTY Shell Functionality

- Test Ctrl+C sends interrupt to remote shell (doesn't exit)
- Test Ctrl+D returns to listener prompt
- Test arrow keys work in remote shell
- Test command history works in remote shell
- Test tab completion works in remote shell

### Port Forwarding

- Test forward command establishes tunnel
- Test data flows through tunnel
- Test stop command closes tunnel
- Test reconnection after tunnel closes

### SOCKS5 Proxy

- Test socks command starts proxy
- Test browser can use proxy
- Test authentication if required
- Test stop command closes proxy

---

**Note**: This checklist should be updated whenever new critical manual tests are identified.
