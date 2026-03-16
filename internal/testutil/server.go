package testutil

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tjst-t/dlrelay/internal/convert"
	"github.com/tjst-t/dlrelay/internal/download"
	"github.com/tjst-t/dlrelay/internal/server"
)

// TestServer creates a test HTTP server with real download and convert managers.
func TestServer(t *testing.T) *httptest.Server {
	t.Helper()
	download.AllowPrivateIPs = true

	downloadDir, err := os.MkdirTemp("", "dlrelay-test-dl-*")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()

	dlMgr := download.NewManager(downloadDir, tempDir, 3, nil, nil)
	convMgr := convert.NewManager()

	srv := server.New(dlMgr, convMgr)
	ts := httptest.NewServer(srv)
	t.Cleanup(func() {
		ts.Close()
		os.RemoveAll(downloadDir)
	})

	return ts
}

// TestServerWithDir creates a test server and returns the download directory.
func TestServerWithDir(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	download.AllowPrivateIPs = true

	downloadDir, err := os.MkdirTemp("", "dlrelay-test-dl-*")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()

	dlMgr := download.NewManager(downloadDir, tempDir, 3, nil, nil)
	convMgr := convert.NewManager()

	srv := server.New(dlMgr, convMgr)
	ts := httptest.NewServer(srv)
	t.Cleanup(func() {
		ts.Close()
		os.RemoveAll(downloadDir)
	})

	return ts, downloadDir
}

// FileExists checks if a file exists and has content.
func FileExists(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() > 0
}
