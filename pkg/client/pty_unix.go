//go:build !windows
// +build !windows

package client

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// startPty starts a command in a PTY (Unix implementation)
func startPty(cmd *exec.Cmd) (*os.File, error) {
	return pty.Start(cmd)
}

// setPtySize sets the PTY window size (Unix implementation)
func setPtySize(ptmx *os.File, rows, cols int) error {
	ws := &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}
	return pty.Setsize(ptmx, ws)
}

// ptyReader wraps PTY for reading (Unix - direct use)
type ptyReader struct {
	*os.File
}

func newPtyReader(ptmx *os.File) io.Reader {
	return &ptyReader{ptmx}
}

// wrapPtyFile is a no-op on Unix (file is already directly usable)
func wrapPtyFile(f *os.File) io.ReadWriteCloser {
	return f
}
