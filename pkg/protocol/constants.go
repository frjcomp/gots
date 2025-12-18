// Package protocol defines constants for the reverse shell protocol,
// including buffer sizes, timeouts, command names, and protocol markers.
package protocol

// Protocol constants used across the reverse shell system

const (
	// Buffer sizes
	BufferSize1MB = 1024 * 1024      // 1MB buffer for large file transfers
	MaxBufferSize = 10 * 1024 * 1024 // 10MB maximum accumulated buffer before reset
	ChunkSize     = 65536            // 64KB for file upload chunks

	// Protocol delimiters and markers
	EndOfOutputMarker = "<<<END_OF_OUTPUT>>>"
	DataPrefix        = "DATA "

	// Commands
	CmdPing        = "PING"
	CmdPong        = "PONG"
	CmdExit        = "exit"
	CmdStartUpload = "START_UPLOAD"
	CmdUploadChunk = "UPLOAD_CHUNK"
	CmdEndUpload   = "END_UPLOAD"
	CmdUpload      = "UPLOAD"
	CmdDownload    = "DOWNLOAD"

	// Timeouts
	ReadTimeout         = 1          // second
	ResponseTimeout     = 5          // seconds
	DownloadTimeout     = 5000000000 // nanoseconds (very large for big files)
	PingInterval        = 30         // seconds
	ShutdownGracePeriod = 5          // seconds
)
