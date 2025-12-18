package protocol

import "testing"

func TestConstantsExist(t *testing.T) {
	// Verify constants are non-zero
	if BufferSize1MB == 0 {
		t.Error("BufferSize1MB should not be zero")
	}

	if MaxBufferSize == 0 {
		t.Error("MaxBufferSize should not be zero")
	}

	if ChunkSize == 0 {
		t.Error("ChunkSize should not be zero")
	}

	// Verify string constants are non-empty
	if EndOfOutputMarker == "" {
		t.Error("EndOfOutputMarker should not be empty")
	}

	if DataPrefix == "" {
		t.Error("DataPrefix should not be empty")
	}
}

func TestBufferSizes(t *testing.T) {
	// Verify buffer size relationships
	if ChunkSize >= BufferSize1MB {
		t.Errorf("ChunkSize (%d) should be smaller than BufferSize1MB (%d)", ChunkSize, BufferSize1MB)
	}

	if BufferSize1MB >= MaxBufferSize {
		t.Errorf("BufferSize1MB (%d) should be smaller than MaxBufferSize (%d)", BufferSize1MB, MaxBufferSize)
	}
}

func TestTimeouts(t *testing.T) {
	// Verify timeouts are positive
	if ReadTimeout <= 0 {
		t.Error("ReadTimeout should be positive")
	}

	if ResponseTimeout <= 0 {
		t.Error("ResponseTimeout should be positive")
	}

	if PingInterval <= 0 {
		t.Error("PingInterval should be positive")
	}
}
