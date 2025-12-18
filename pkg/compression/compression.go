// Package compression provides utilities for compressing and decompressing data
// using gzip and hex encoding for protocol transmission.
package compression

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
)

// CompressToHex compresses data using gzip and returns it as a hex-encoded string.
// This is useful for transmitting binary data over text-based protocols.
func CompressToHex(data []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return "", fmt.Errorf("failed to write to gzip: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

// DecompressHex decodes a hex-encoded string and decompresses it using gzip.
// Returns the original uncompressed data.
func DecompressHex(payload string) ([]byte, error) {
	compressed, err := hex.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return data, nil
}
