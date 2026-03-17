package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

func hasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func TestDownloadToFile(t *testing.T) {
	content := "test file content for downloadToFile"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(content))
	}))
	defer srv.Close()

	tmpFile := filepath.Join(t.TempDir(), "output.bin")
	err := downloadToFile(context.Background(), srv.URL+"/test.bin", nil, tmpFile, 0)
	if err != nil {
		t.Fatalf("downloadToFile failed: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch: got %q, want %q", string(data), content)
	}
}

func TestDownloadToFileWithHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Write([]byte("protected data"))
	}))
	defer srv.Close()

	tmpFile := filepath.Join(t.TempDir(), "output.bin")
	headers := map[string]string{
		"Authorization": "Bearer test-token",
	}
	err := downloadToFile(context.Background(), srv.URL+"/test.bin", headers, tmpFile, 0)
	if err != nil {
		t.Fatalf("downloadToFile failed: %v", err)
	}

	if receivedAuth != "Bearer test-token" {
		t.Fatalf("expected auth header 'Bearer test-token', got %q", receivedAuth)
	}
}

func TestMuxStreams(t *testing.T) {
	if !hasFFmpeg() {
		t.Skip("ffmpeg not available, skipping mux test")
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "video.mp4")
	audioPath := filepath.Join(dir, "audio.m4a")
	outputPath := filepath.Join(dir, "output.mp4")

	// Generate 1-second test video
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", videoPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to generate test video: %v: %s", err, string(out))
	}

	// Generate 1-second test audio
	cmd = exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "aac", audioPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to generate test audio: %v: %s", err, string(out))
	}

	err := muxStreams(context.Background(), videoPath, audioPath, outputPath)
	if err != nil {
		t.Fatalf("muxStreams failed: %v", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
	t.Logf("muxed output: %d bytes", info.Size())
}

func TestDASHDownloadE2E(t *testing.T) {
	if !hasFFmpeg() {
		t.Skip("ffmpeg not available")
	}

	// Create a real test: serve proper video and audio streams
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "src_video.mp4")
	audioPath := filepath.Join(dir, "src_audio.m4a")

	// Generate test media files
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", videoPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to generate test video: %v: %s", err, string(out))
	}

	cmd = exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "aac", audioPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("failed to generate test audio: %v: %s", err, string(out))
	}

	videoData, _ := os.ReadFile(videoPath)
	audioData, _ := os.ReadFile(audioPath)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/video.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(videoData)
		case "/audio.m4a":
			w.Header().Set("Content-Type", "audio/mp4")
			w.Write(audioData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	tempDir := t.TempDir()
	mgr := NewManager(downloadDir, tempDir, 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      srv.URL + "/video.mp4",
		AudioURL: srv.URL + "/audio.m4a",
		Filename: "dash_output.mp4",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted {
			break
		}
		if status.State == model.StateFailed {
			t.Fatalf("DASH download failed: %v", status.Error)
		}
		time.Sleep(100 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	if status.State != model.StateCompleted {
		t.Fatalf("expected completed, got %s", status.State)
	}

	outFile := filepath.Join(downloadDir, "dash_output.mp4")
	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("output file is empty")
	}
	t.Logf("DASH E2E: output=%s (%d bytes)", outFile, info.Size())
}

func TestManagerRoutesDASH(t *testing.T) {
	// Test that the manager correctly routes requests with AudioURL
	videoContent := strings.Repeat("video-data-", 100)
	audioContent := strings.Repeat("audio-data-", 50)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v.mp4":
			w.Write([]byte(videoContent))
		case "/a.m4a":
			w.Write([]byte(audioContent))
		}
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	tempDir := t.TempDir()
	mgr := NewManager(downloadDir, tempDir, 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      srv.URL + "/v.mp4",
		AudioURL: srv.URL + "/a.m4a",
		Filename: "dash.mp4",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for completion
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	// With fake data, ffmpeg will fail, but the download of streams should have been attempted
	t.Logf("Manager routing test: state=%s", status.State)
	if status.State == model.StateCompleted && !hasFFmpeg() {
		t.Fatal("unexpected completion without ffmpeg")
	}
}
