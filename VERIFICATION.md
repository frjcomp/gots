# Keyboard Blocking Issue - Complete Fix & Verification

## Issue Summary
After exiting a Windows shell via PTY session, the Linux listener's keyboard would become unresponsive, blocking on readline input.

## Root Cause Analysis
The `SessionArbiter.RunPtySession()` input pump sets `SetReadDeadline()` on stdin to implement timeouts. When the PTY session ended and control returned to the main loop's readline, these read deadlines were never explicitly cleared, leaving stdin in a state where reads would timeout immediately or block indefinitely.

## Complete Solution

### 1. Architectural Fix (pkg/console/arbiter.go)
- Added `savedReadDeadline` field to `SessionArbiter` struct
- Save original FD state on PTY entry
- Explicitly restore `SetReadDeadline(time.Time{})` after PTY exit and goroutine cleanup
- This ensures stdin is in a known good blocking state for readline

### 2. Test Coverage (pkg/console/arbiter_test.go)
- **TestFDStateRestoredAfterPty** - Validates FD state restoration
- **TestMultiplePtyCycles** - Tests multiple consecutive PTY cycles
- **TestFDStatePreservesReadability** - Verifies TTY accessibility after PTY

### 3. Integration Verification (integration/pty_comprehensive_test.go)
- **TestPtyComprehensive** - 8 comprehensive PTY scenarios
- Tests entry/exit with Ctrl-D
- Tests exit command handling
- Tests multiple cycles
- All tested end-to-end with both Linux and Windows shells

## Verification Checklist

✅ **Code Changes**
- [x] Architectural fix in SessionArbiter (FD state management)
- [x] Removed band-aid fixes from main.go
- [x] Proper separation of concerns

✅ **Test Coverage**
- [x] 3 new unit tests for FD state
- [x] 8 integration tests for PTY scenarios
- [x] All tests pass with race detector
- [x] No race conditions detected

✅ **Cross-Platform**
- [x] Linux listener tested
- [x] Windows shell tested
- [x] Linux shell tested
- [x] All combinations working

✅ **Multiple Cycles**
- [x] Tests verify multiple PTY entry/exit cycles
- [x] No state accumulation issues
- [x] Keyboard responsive after each cycle

## How to Verify the Fix Works

### 1. Run Full Test Suite
```bash
go test ./... -timeout 180s -race
```
All tests must pass with no race detector warnings.

### 2. Run Console Tests Specifically
```bash
go test ./pkg/console -v -race
```
Should see TestFDStateRestoredAfterPty, TestMultiplePtyCycles pass.

### 3. Run Integration Tests
```bash
go test ./integration -run TestPtyComprehensive -v
```
Should see all 8 tests pass (entry/exit, responsiveness, multiple cycles).

### 4. Manual Testing (if access to Windows shell)
```bash
gotsl> shell 1     # Enter PTY with Windows shell
exit               # Exit back to listener
gotsl> ls          # Keyboard should be responsive
```

## What Would Indicate Regression

Any of these would indicate the fix was broken:
1. Tests fail: TestFDStateRestoredAfterPty, TestMultiplePtyCycles
2. Race detector warnings in arbiter.go
3. Integration test times out after PTY exit
4. Manual test shows unresponsive keyboard after 'exit'
5. Multiple PTY cycles fail on 2nd or 3rd cycle

## Commits
- e8e40ea - Initial terminal reset attempt (partial fix)
- 9d24534 - Proper architectural fix with FD state management
- f3272b1 - Comprehensive test coverage
- 1c4d296 - Test documentation

## Conclusion
The keyboard blocking issue is now completely resolved through:
1. **Root cause fix** - Explicit FD state restoration in SessionArbiter
2. **Comprehensive testing** - Unit tests ensure the fix works
3. **Integration verification** - End-to-end tests with real PTY scenarios
4. **Regression prevention** - Tests would immediately catch any break in the fix

The issue will not recur as long as the test suite passes.
