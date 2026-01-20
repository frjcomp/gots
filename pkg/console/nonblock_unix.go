//go:build !windows
// +build !windows

package console

import "syscall"

// setNonblock sets the file descriptor to non-blocking mode.
// On Unix-like systems, file descriptors are ints.
func setNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(int(fd), nonblocking)
}
