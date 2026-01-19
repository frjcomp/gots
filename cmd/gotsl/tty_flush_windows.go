//go:build windows
// +build windows

package main

// flushStdin is a no-op on Windows because Windows doesn't provide
// a direct equivalent to Unix's TCFLSH ioctl for flushing stdin buffer.
// However, this is typically not an issue because:
// 1. gotsl is usually run on Unix/Linux hosts (the listener side)
// 2. Windows 10+ terminal handles are automatically managed by ConPTY
// 3. The drainPendingInput() function in resetReadlineAfterPty() handles
//    cross-platform stdin cleanup through standard Go read operations
func flushStdin() error {
	return nil
}
