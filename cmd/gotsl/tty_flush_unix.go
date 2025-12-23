//go:build !windows
// +build !windows

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// flushStdin discards any pending input bytes on stdin (Unix platforms).
func flushStdin() error {
	fd := int(os.Stdin.Fd())
	return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
}
