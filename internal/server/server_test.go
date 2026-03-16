package server_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/dlrelay/internal/convert"
	"github.com/tjst-t/dlrelay/internal/download"
	"github.com/tjst-t/dlrelay/internal/model"
	"github.com/tjst-t/dlrelay/internal/server"
	"github.com/tjst-t/dlrelay/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", result["status"])
	}
}

func TestDownloadCreate(t *testing.T) {
	ts := testutil.TestServer(t)

	// Start a file server to serve test content
	fileServer := startTestFileServer(t, "hello world test content")

	body, _ := json.Marshal(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "test.txt",
	})

	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var status model.DownloadStatus
	json.NewDecoder(resp.Body).Decode(&status)
	if status.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if status.Filename != "test.txt" {
		t.Fatalf("expected filename test.txt, got %s", status.Filename)
	}
}

func TestDownloadCreateInvalidBody(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader([]byte(`{invalid`)))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDownloadCreateMissingURL(t *testing.T) {
	ts := testutil.TestServer(t)

	body, _ := json.Marshal(model.DownloadRequest{Filename: "test.txt"})
	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDownloadList(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/api/downloads")
	if err != nil {
		t.Fatalf("GET /api/downloads failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var list []model.DownloadStatus
	json.NewDecoder(resp.Body).Decode(&list)
	if list == nil {
		t.Fatal("expected non-nil list")
	}
}

func TestDownloadGetNotFound(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/api/downloads/nonexistent")
	if err != nil {
		t.Fatalf("GET /api/downloads/nonexistent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadDeleteNotFound(t *testing.T) {
	ts := testutil.TestServer(t)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/downloads/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /api/downloads/nonexistent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadProgressAndComplete(t *testing.T) {
	ts, downloadDir := testutil.TestServerWithDir(t)

	// Start a file server
	content := "test file content for download"
	fileServer := startTestFileServer(t, content)

	body, _ := json.Marshal(model.DownloadRequest{
		URL:      fileServer.URL + "/testfile.bin",
		Filename: "testfile.bin",
	})

	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	var status model.DownloadStatus
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()

	id := status.ID

	// Poll until completed or timeout
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get(ts.URL + "/api/downloads/" + id)
		if err != nil {
			t.Fatalf("GET /api/downloads/%s failed: %v", id, err)
		}
		json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()

		if status.State == model.StateCompleted {
			break
		}
		if status.State == model.StateFailed {
			t.Fatalf("download failed: %v", status.Error)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if status.State != model.StateCompleted {
		t.Fatalf("expected completed, got %s", status.State)
	}

	// Verify file exists
	if !testutil.FileExists(t, downloadDir+"/testfile.bin") {
		t.Fatal("downloaded file not found")
	}
}

func TestDownloadCancel(t *testing.T) {
	ts := testutil.TestServer(t)

	// Use a slow server that takes forever
	slowServer := startSlowFileServer(t)

	body, _ := json.Marshal(model.DownloadRequest{
		URL:      slowServer.URL + "/slow.bin",
		Filename: "slow.bin",
	})

	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	var status model.DownloadStatus
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()

	// Delete (cancel) the download
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/downloads/"+status.ID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestConvertGetNotFound(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/api/convert/nonexistent")
	if err != nil {
		t.Fatalf("GET /api/convert/nonexistent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestExtensionZipEndpoint(t *testing.T) {
	// Create a fake extension directory with background.js and popup/popup.js
	extDir := t.TempDir()
	os.MkdirAll(filepath.Join(extDir, "popup"), 0o755)

	os.WriteFile(filepath.Join(extDir, "manifest.json"), []byte(`{"name":"test"}`), 0o644)
	os.WriteFile(filepath.Join(extDir, "background.js"), []byte(`const config = { serverUrl: "", debug: false };`), 0o644)
	os.WriteFile(filepath.Join(extDir, "popup", "popup.js"), []byte(`const defaults = { serverUrl: "", timeout: 30 };`), 0o644)
	os.WriteFile(filepath.Join(extDir, "icon.svg"), []byte(`<svg></svg>`), 0o644)       // should be excluded
	os.WriteFile(filepath.Join(extDir, ".hidden"), []byte(`secret`), 0o644)              // should be excluded

	dlMgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	convMgr := convert.NewManager()
	srv := server.New(dlMgr, convMgr, server.WithExtensionDir(extDir))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/api/extension.zip")
	if err != nil {
		t.Fatalf("GET /api/extension.zip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("expected application/zip, got %s", ct)
	}

	// Read and parse zip
	body, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("failed to parse zip: %v", err)
	}

	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(data)
	}

	// Verify SVG and hidden files are excluded
	if _, ok := files["icon.svg"]; ok {
		t.Error("SVG files should be excluded from zip")
	}
	if _, ok := files[".hidden"]; ok {
		t.Error("hidden files should be excluded from zip")
	}

	// Verify manifest.json is included unchanged
	if files["manifest.json"] != `{"name":"test"}` {
		t.Errorf("manifest.json unexpected content: %s", files["manifest.json"])
	}

	// Verify server URL injection in background.js
	bg := files["background.js"]
	if bg == "" {
		t.Fatal("background.js not found in zip")
	}
	if !bytes.Contains([]byte(bg), []byte(`serverUrl: "`+ts.URL+`"`)) {
		t.Errorf("background.js should have server URL injected, got: %s", bg)
	}

	// Verify server URL injection in popup/popup.js
	popup := files["popup/popup.js"]
	if popup == "" {
		t.Fatal("popup/popup.js not found in zip")
	}
	if !bytes.Contains([]byte(popup), []byte(`serverUrl: "`+ts.URL+`"`)) {
		t.Errorf("popup/popup.js should have server URL injected, got: %s", popup)
	}
}

func TestExtensionZipNotConfigured(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/api/extension.zip")
	if err != nil {
		t.Fatalf("GET /api/extension.zip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when extension dir not configured, got %d", resp.StatusCode)
	}
}

func TestAPIKeyAuthBlocksMutations(t *testing.T) {
	// Create server with API key
	dlMgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	convMgr := convert.NewManager()
	srv := server.New(dlMgr, convMgr, server.WithAPIKey("test-secret"))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// POST without API key should be rejected
	body, _ := json.Marshal(model.DownloadRequest{
		URL:      "http://example.com/video.mp4",
		Filename: "video.mp4",
	})
	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/downloads failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d", resp.StatusCode)
	}

	// POST with wrong API key should be rejected
	req, _ := http.NewRequest("POST", ts.URL+"/api/downloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "wrong-key")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", resp.StatusCode)
	}

	// POST with correct API key should be accepted
	req, _ = http.NewRequest("POST", ts.URL+"/api/downloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 with correct key, got %d", resp.StatusCode)
	}
}

func TestAPIKeyAuthAllowsReads(t *testing.T) {
	// Create server with API key
	dlMgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	convMgr := convert.NewManager()
	srv := server.New(dlMgr, convMgr, server.WithAPIKey("test-secret"))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// GET endpoints should work without API key
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for health without key, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/downloads")
	if err != nil {
		t.Fatalf("GET /api/downloads failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for downloads list without key, got %d", resp.StatusCode)
	}
}

func TestAPIKeyViaQueryParamRejected(t *testing.T) {
	dlMgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	convMgr := convert.NewManager()
	srv := server.New(dlMgr, convMgr, server.WithAPIKey("test-secret"))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// POST with key in query param should be rejected (API key only via header)
	body, _ := json.Marshal(model.DownloadRequest{
		URL:      "http://example.com/video.mp4",
		Filename: "video.mp4",
	})
	resp, err := http.Post(ts.URL+"/api/downloads?key=test-secret", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with key in query param (no longer supported), got %d", resp.StatusCode)
	}
}

func TestNoAPIKeyMeansNoAuth(t *testing.T) {
	// Without API key configured, everything should work
	ts := testutil.TestServer(t)

	body, _ := json.Marshal(model.DownloadRequest{
		URL:      "http://example.com/video.mp4",
		Filename: "video.mp4",
	})
	resp, err := http.Post(ts.URL+"/api/downloads", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 without API key config, got %d", resp.StatusCode)
	}
}

func TestCORSAllowsAnyOrigin(t *testing.T) {
	ts := testutil.TestServer(t)

	// Any origin should get CORS header (API key is the security boundary)
	origins := []string{
		"http://example.com",
		"https://www.youtube.com",
		"chrome-extension://abcdef123456",
		"moz-extension://abcdef123456",
	}
	for _, o := range origins {
		req, _ := http.NewRequest("OPTIONS", ts.URL+"/api/downloads", nil)
		req.Header.Set("Origin", o)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("OPTIONS failed for origin %s: %v", o, err)
		}
		resp.Body.Close()
		got := resp.Header.Get("Access-Control-Allow-Origin")
		if got != o {
			t.Fatalf("expected CORS origin %s, got: %s", o, got)
		}
	}

	// No Origin header should not set Access-Control-Allow-Origin
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/api/downloads", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS failed: %v", err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS header without Origin, got: %s", got)
	}
}

func TestBookmarkletPage(t *testing.T) {
	ts := testutil.TestServer(t)

	resp, err := http.Get(ts.URL + "/bookmarklet")
	if err != nil {
		t.Fatalf("GET /bookmarklet failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %s", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, ts.URL) {
		t.Error("bookmarklet page should contain server URL")
	}
	if !strings.Contains(html, "javascript:") {
		t.Error("bookmarklet page should contain bookmarklet code")
	}
}

func TestBookmarkletPageWithAPIKey(t *testing.T) {
	dlMgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	convMgr := convert.NewManager()
	srv := server.New(dlMgr, convMgr, server.WithAPIKey("my-secret-key"))
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/bookmarklet")
	if err != nil {
		t.Fatalf("GET /bookmarklet failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "my-secret-key") {
		t.Error("bookmarklet page should contain API key")
	}
}

func TestConvertDeleteNotFound(t *testing.T) {
	ts := testutil.TestServer(t)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/convert/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
