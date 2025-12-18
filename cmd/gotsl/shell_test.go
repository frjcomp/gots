package main

import (
	"errors"
	"os"
	"testing"

	"golang-https-rev/pkg/protocol"
)

func TestHandleUploadSuccess(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "upload-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte("test content"))
	tmpFile.Close()

	ml := &mockListener{
		responses: []string{
			"OK" + protocol.EndOfOutputMarker,
			"OK" + protocol.EndOfOutputMarker,
			"upload complete" + protocol.EndOfOutputMarker,
		},
	}

	result := handleUpload(ml, "client1", []string{"upload", tmpFile.Name(), "/remote/path.txt"})
	if !result {
		t.Fatal("expected success")
	}
}

func TestHandleUploadMissingArgs(t *testing.T) {
	ml := &mockListener{}
	result := handleUpload(ml, "client1", []string{"upload", "file.txt"})
	if !result {
		t.Fatal("expected true when args are wrong but no disconnect")
	}
}

func TestHandleUploadFileNotFound(t *testing.T) {
	ml := &mockListener{}
	result := handleUpload(ml, "client1", []string{"upload", "/nonexistent/file.txt", "/remote/path.txt"})
	if !result {
		t.Fatal("expected true when file not found (error handled)")
	}
}

func TestHandleUploadSendCommandError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "upload-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte("test"))
	tmpFile.Close()

	ml := &mockListener{sendErr: errors.New("send failed")}
	result := handleUpload(ml, "client1", []string{"upload", tmpFile.Name(), "/remote/path.txt"})
	if result {
		t.Fatal("expected false on send error")
	}
}

func TestHandleDownloadSuccess(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "download-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	ml := &mockListener{
		responses: []string{protocol.DataPrefix + "1f8b08000000000000ff" + protocol.EndOfOutputMarker},
	}

	result := handleDownload(ml, "client1", []string{"download", "/remote/file.txt", tmpPath})
	if !result {
		t.Fatal("expected success")
	}
}

func TestHandleDownloadMissingArgs(t *testing.T) {
	ml := &mockListener{}
	result := handleDownload(ml, "client1", []string{"download", "file.txt"})
	if !result {
		t.Fatal("expected true on arg error")
	}
}

func TestHandleDownloadSendError(t *testing.T) {
	ml := &mockListener{sendErr: errors.New("send failed")}
	result := handleDownload(ml, "client1", []string{"download", "/remote/file.txt", "/local/path.txt"})
	if result {
		t.Fatal("expected false on send error")
	}
}

func TestHandleDownloadUnexpectedResponse(t *testing.T) {
	ml := &mockListener{responses: []string{"NOT_DATA_PREFIX" + protocol.EndOfOutputMarker}}
	result := handleDownload(ml, "client1", []string{"download", "/remote/file.txt", "/tmp/out.txt"})
	if !result {
		t.Fatal("expected true on unexpected response (error handled)")
	}
}

