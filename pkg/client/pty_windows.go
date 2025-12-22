//go:build windows
// +build windows

package client

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/UserExistsError/conpty"
)

// windowsPty wraps ConPTY to provide *os.File-like interface
type windowsPty struct {
	cpty  *conpty.ConPty
	readPipe  *os.File
	writePipe *os.File
}

func (w *windowsPty) Read(p []byte) (int, error) {
	return w.readPipe.Read(p)
}

func (w *windowsPty) Write(p []byte) (int, error) {
	if w.cpty == nil {
		return 0, io.EOF
	}
	return w.cpty.Write(p)
}

func (w *windowsPty) Close() error {
	if w.readPipe != nil {
		w.readPipe.Close()
	}
	if w.writePipe != nil {
		w.writePipe.Close()
	}
	if w.cpty != nil {
		return w.cpty.Close()
	}
	return nil
}

func (w *windowsPty) Fd() uintptr {
	return w.readPipe.Fd()
}

// startPty starts a command in a PTY (Windows ConPTY implementation)
func startPty(cmd *exec.Cmd) (*os.File, error) {
	// Build command line
	cmdLine := cmd.Path
	if len(cmd.Args) > 1 {
		cmdLine = strings.Join(cmd.Args, " ")
	}

	// Start ConPTY with the command
	cpty, err := conpty.Start(cmdLine)
	if err != nil {
		return nil, err
	}

	// Create a pipe to wrap ConPTY as *os.File
	r, w, err := os.Pipe()
	if err != nil {
		cpty.Close()
		return nil, err
	}

	// Create wrapper
	wrapper := &windowsPty{
		cpty:      cpty,
		readPipe:  r,
		writePipe: w,
	}

	// Forward ConPTY output to the pipe
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := cpty.Read(buf)
			if err != nil {
				w.Close()
				return
			}
			if n > 0 {
				w.Write(buf[:n])
			}
		}
	}()

	// We need to return something that satisfies *os.File interface
	// but also can intercept Write() calls. This is tricky because
	// Go doesn't allow us to return a custom type as *os.File.
	// 
	// Solution: Store the wrapper in a package-level map keyed by Fd()
	storePtyWrapper(r.Fd(), wrapper)

	return r, nil
}

var (
	ptyWrappers = make(map[uintptr]*windowsPty)
)

func storePtyWrapper(fd uintptr, wrapper *windowsPty) {
	ptyWrappers[fd] = wrapper
}

func getPtyWrapper(fd uintptr) *windowsPty {
	return ptyWrappers[fd]
}

// setPtySize sets the PTY window size (Windows ConPTY implementation)
func setPtySize(ptmx *os.File, rows, cols int) error {
	wrapper := getPtyWrapper(ptmx.Fd())
	if wrapper == nil || wrapper.cpty == nil {
		return nil
	}
	return wrapper.cpty.Resize(cols, rows)
}

// ptyReader wraps ConPTY for reading (Windows)
type ptyReader struct {
	*os.File
}

func newPtyReader(ptmx *os.File) io.Reader {
	return &ptyReader{ptmx}
}

// wrapPtyFile wraps the file handle to intercept writes on Windows
func wrapPtyFile(f *os.File) io.ReadWriteCloser {
	wrapper := getPtyWrapper(f.Fd())
	if wrapper != nil {
		return wrapper
	}
	return f
}
