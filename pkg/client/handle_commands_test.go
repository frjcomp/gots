package client

import (
    "bufio"
    "bytes"
    "testing"

    "golang-https-rev/pkg/protocol"
)

// mockClientLoop creates a ReverseClient with configurable reader/writer buffers
func mockClientLoop(input []byte, readerBufSize int) (*ReverseClient, *bytes.Buffer) {
    in := bytes.NewReader(input)
    var reader *bufio.Reader
    if readerBufSize > 0 {
        reader = bufio.NewReaderSize(in, readerBufSize)
    } else {
        reader = bufio.NewReader(in)
    }
    out := new(bytes.Buffer)
    writer := bufio.NewWriter(out)
    rc := &ReverseClient{reader: reader, writer: writer, conn: nil}
    return rc, out
}

// TestHandleCommandsPingExit ensures the loop processes PING then EXIT and returns
func TestHandleCommandsPingExit(t *testing.T) {
    input := []byte(protocol.CmdPing + "\n" + protocol.CmdExit + "\n")
    rc, out := mockClientLoop(input, 0)

    err := rc.HandleCommands()
    if err != nil {
        t.Fatalf("HandleCommands returned error: %v", err)
    }

    // Flush any buffered writes
    rc.writer.Flush()
    got := out.String()
    if !bytes.Contains(out.Bytes(), []byte(protocol.CmdPong)) {
        t.Errorf("expected PONG in output, got: %q", got)
    }
    if !bytes.Contains(out.Bytes(), []byte(protocol.EndOfOutputMarker)) {
        t.Errorf("expected EndOfOutputMarker in output, got: %q", got)
    }
}

// TestHandleCommandsEOF ensures EOF leads to a clean exit with no error
func TestHandleCommandsEOF(t *testing.T) {
    rc, _ := mockClientLoop(nil, 0)
    err := rc.HandleCommands()
    if err != nil {
        t.Fatalf("expected nil error on EOF, got: %v", err)
    }
}

// TestHandleCommandsBufferFull covers the ErrBufferFull branch by using a tiny reader buffer
func TestHandleCommandsBufferFull(t *testing.T) {
    long := bytes.Repeat([]byte("A"), 2048) // long line with no newline initially
    // Provide long line then a newline and EXIT to end the loop
    payload := append(long, '\n')
    payload = append(payload, []byte(protocol.CmdExit+"\n")...)

    // Tiny buffer to force bufio.ErrBufferFull on ReadString
    rc, _ := mockClientLoop(payload, 16)
    err := rc.HandleCommands()
    if err != nil {
        t.Fatalf("HandleCommands returned error under buffer-full scenario: %v", err)
    }
}
