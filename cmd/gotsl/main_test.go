package main

import (
	"bytes"
	"testing"
)

// TestCompressDecompressRoundTrip verifies that data can be compressed to hex and decompressed back identically
func TestCompressDecompressRoundTrip(t *testing.T) {
	testData := []byte("Hello, this is test data for compression! " + string(bytes.Repeat([]byte("x"), 1000)))

	// Compress
	compressed, err := compressToHex(testData)
	if err != nil {
		t.Fatalf("compressToHex failed: %v", err)
	}

	// Verify compressed is not empty
	if compressed == "" {
		t.Fatal("compressed hex should not be empty")
	}

	// Decompress
	decompressed, err := decompressHex(compressed)
	if err != nil {
		t.Fatalf("decompressHex failed: %v", err)
	}

	// Verify round-trip
	if !bytes.Equal(decompressed, testData) {
		t.Fatalf("decompressed data does not match original: got %d bytes, expected %d bytes", len(decompressed), len(testData))
	}
}

// TestCompressEmptyData handles edge case of compressing empty data
func TestCompressEmptyData(t *testing.T) {
	testData := []byte{}

	compressed, err := compressToHex(testData)
	if err != nil {
		t.Fatalf("compressToHex failed on empty data: %v", err)
	}

	decompressed, err := decompressHex(compressed)
	if err != nil {
		t.Fatalf("decompressHex failed on empty data: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Fatal("decompressed empty data should match original")
	}
}

// TestCompressLargeData ensures compression works with large payloads
func TestCompressLargeData(t *testing.T) {
	// Create 5MB of repetitive data
	testData := bytes.Repeat([]byte("large data payload "), 262144)

	compressed, err := compressToHex(testData)
	if err != nil {
		t.Fatalf("compressToHex failed on large data: %v", err)
	}

	decompressed, err := decompressHex(compressed)
	if err != nil {
		t.Fatalf("decompressHex failed on large data: %v", err)
	}

	if !bytes.Equal(decompressed, testData) {
		t.Fatalf("large data round-trip failed: got %d bytes, expected %d bytes", len(decompressed), len(testData))
	}
}

// TestDecompressInvalidHex verifies that invalid hex input is handled gracefully
func TestDecompressInvalidHex(t *testing.T) {
	_, err := decompressHex("invalid!@#$%hex")
	if err == nil {
		t.Fatal("decompressHex should return error for invalid hex input")
	}
}

// TestDecompressCorruptedGzip verifies that corrupted gzip data is detected
func TestDecompressCorruptedGzip(t *testing.T) {
	// Create valid hex that doesn't contain valid gzip data
	invalidGzip := "deadbeef"

	_, err := decompressHex(invalidGzip)
	if err == nil {
		t.Fatal("decompressHex should return error for corrupted gzip data")
	}
}
