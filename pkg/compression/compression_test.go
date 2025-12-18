package compression

import (
	"bytes"
	"testing"
)

func TestCompressDecompressRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "simple text",
			data: []byte("Hello, World!"),
		},
		{
			name: "empty data",
			data: []byte{},
		},
		{
			name: "large repetitive data",
			data: bytes.Repeat([]byte("test"), 10000),
		},
		{
			name: "binary data",
			data: []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compress
			compressed, err := CompressToHex(tt.data)
			if err != nil {
				t.Fatalf("CompressToHex failed: %v", err)
			}

			if len(tt.data) > 0 && compressed == "" {
				t.Fatal("compressed hex should not be empty for non-empty input")
			}

			// Decompress
			decompressed, err := DecompressHex(compressed)
			if err != nil {
				t.Fatalf("DecompressHex failed: %v", err)
			}

			// Verify round-trip
			if !bytes.Equal(decompressed, tt.data) {
				t.Fatalf("decompressed data does not match original: got %d bytes, expected %d bytes", len(decompressed), len(tt.data))
			}
		})
	}
}

func TestDecompressHexInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "invalid hex characters",
			payload: "invalid!@#$%hex",
		},
		{
			name:    "corrupted gzip data",
			payload: "deadbeef",
		},
		{
			name:    "odd length hex",
			payload: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecompressHex(tt.payload)
			if err == nil {
				t.Fatal("DecompressHex should return error for invalid input")
			}
		})
	}
}

func TestCompressToHexError(t *testing.T) {
	// Test with very large data to ensure proper error handling
	largeData := bytes.Repeat([]byte("x"), 100*1024*1024) // 100MB

	// Should not panic and should handle gracefully
	_, err := CompressToHex(largeData)
	if err != nil {
		t.Logf("Large data compression failed as expected: %v", err)
	} else {
		t.Log("Large data compression succeeded")
	}
}

// Benchmark tests
func BenchmarkCompressToHex(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark data "), 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CompressToHex(data)
	}
}

func BenchmarkDecompressHex(b *testing.B) {
	data := bytes.Repeat([]byte("benchmark data "), 1000)
	compressed, _ := CompressToHex(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecompressHex(compressed)
	}
}
