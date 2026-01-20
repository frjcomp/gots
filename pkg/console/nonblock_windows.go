//go:build windows
// +build windows

package console

import "syscall"

// setNonblock sets the file descriptor to non-blocking mode.
// On Windows, file descriptors need to be converted to Handle.
func setNonblock(fd uintptr, nonblocking bool) error {
	return syscall.SetNonblock(syscall.Handle(fd), nonblocking)
}
