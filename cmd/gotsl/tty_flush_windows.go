//go:build windows
// +build windows

package main

// flushStdin is a no-op on Windows because the listener runs on Unix.
func flushStdin() error {
	return nil
}
