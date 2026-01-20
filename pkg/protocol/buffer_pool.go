package protocol

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
)

var bytesBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// GetBuffer returns a reset bytes.Buffer from pool.
func GetBuffer() *bytes.Buffer {
	b := bytesBufferPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

// PutBuffer resets and returns the buffer to the pool.
func PutBuffer(b *bytes.Buffer) {
	if b == nil {
		return
	}
	b.Reset()
	bytesBufferPool.Put(b)
}

var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// GetGzipWriter returns a gzip.Writer reset to write to w.
func GetGzipWriter(w io.Writer) *gzip.Writer {
	gw := gzipWriterPool.Get().(*gzip.Writer)
	gw.Reset(w)
	return gw
}

// PutGzipWriter resets the writer to io.Discard and returns it to the pool.
func PutGzipWriter(gw *gzip.Writer) {
	if gw == nil {
		return
	}
	// Reset to a benign writer to drop references to the previous buffer.
	gw.Reset(io.Discard)
	gzipWriterPool.Put(gw)
}

var scratch64KPool = sync.Pool{
	New: func() any { return make([]byte, 64*1024) },
}

// GetScratch64K returns a 64KB scratch buffer.
func GetScratch64K() []byte {
	return scratch64KPool.Get().([]byte)
}

// PutScratch64K returns a scratch buffer to the pool.
func PutScratch64K(b []byte) {
	if cap(b) >= 64*1024 {
		scratch64KPool.Put(b[:64*1024])
	}
}
