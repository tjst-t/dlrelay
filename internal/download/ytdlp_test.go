package download

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

func hasYtdlp() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

func TestYtdlpDownload(t *testing.T) {
	if !hasYtdlp() {
		t.Skip("yt-dlp not installed")
	}

	dir := t.TempDir()

	req := model.DownloadRequest{
		URL:      "https://www.youtube.com/watch?v=aqz-KE-bpKQ", // Big Buck Bunny
		Filename: "big_buck_bunny.mp4",
		Method:   "ytdlp",
		Quality:  "worst", // Use worst quality for speed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	task := NewTask("test-ytdlp-1", req, cancel)
	err := YtdlpDownload(ctx, task, dir, 0)
	if err != nil {
		t.Fatalf("YtdlpDownload failed: %v", err)
	}

	status := task.Status()
	if status.State != model.StateCompleted {
		t.Errorf("expected state completed, got %s", status.State)
	}

	// Check that a file was created
	files, _ := filepath.Glob(filepath.Join(dir, "big_buck_bunny*"))
	if len(files) == 0 {
		t.Fatal("no output file found")
	}

	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	if info.Size() < 1024 {
		t.Errorf("output file too small: %d bytes", info.Size())
	}
	t.Logf("Downloaded: %s (%d bytes)", filepath.Base(files[0]), info.Size())
}

func TestYtdlpDownloadCancel(t *testing.T) {
	if !hasYtdlp() {
		t.Skip("yt-dlp not installed")
	}

	dir := t.TempDir()

	req := model.DownloadRequest{
		URL:      "https://www.youtube.com/watch?v=aqz-KE-bpKQ",
		Filename: "cancel_test.mp4",
		Method:   "ytdlp",
		Quality:  "worst",
	}

	ctx, cancel := context.WithCancel(context.Background())

	task := NewTask("test-ytdlp-cancel", req, cancel)

	done := make(chan error, 1)
	go func() {
		done <- YtdlpDownload(ctx, task, dir, 0)
	}()

	// Cancel after a short delay
	time.Sleep(2 * time.Second)
	cancel()

	err := <-done
	if err == nil {
		t.Log("Download completed before cancel (fast network)")
		return
	}
	t.Logf("Cancelled as expected: %v", err)
}

func TestYtdlpInvalidURL(t *testing.T) {
	if !hasYtdlp() {
		t.Skip("yt-dlp not installed")
	}

	dir := t.TempDir()

	req := model.DownloadRequest{
		URL:      "https://www.youtube.com/watch?v=INVALID_VIDEO_ID_XXXXX",
		Filename: "invalid.mp4",
		Method:   "ytdlp",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	task := NewTask("test-ytdlp-invalid", req, cancel)
	err := YtdlpDownload(ctx, task, dir, 0)
	if err == nil {
		t.Fatal("expected error for invalid video URL")
	}
	t.Logf("Got expected error: %v", err)
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.youtube.com/watch?v=xyz", "www.youtube.com"},
		{"http://example.com:8080/path", "example.com"},
		{"https://vimeo.com/video/test", "vimeo.com"},
	}
	for _, tt := range tests {
		got := extractHost(tt.url)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestWriteCookieFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "cookies-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	writeCookieFile(f, "music.youtube.com", "session=abc123; cf_clearance=xyz789; lang=ja")
	f.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should use parent domain for subdomains
	if !contains(content, ".youtube.com") {
		t.Errorf("expected .youtube.com domain, got:\n%s", content)
	}
	if !contains(content, "session\tabc123") {
		t.Errorf("expected session cookie, got:\n%s", content)
	}
	if !contains(content, "cf_clearance\txyz789") {
		t.Errorf("expected cf_clearance cookie, got:\n%s", content)
	}
	if !contains(content, "lang\tja") {
		t.Errorf("expected lang cookie, got:\n%s", content)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) && searchString(s, substr))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestToBytes(t *testing.T) {
	tests := []struct {
		size float64
		unit string
		want int64
	}{
		{100, "mib", 104857600},
		{1.5, "gib", 1610612736},
		{500, "kib", 512000},
		{1000, "b", 1000},
		{50, "mb", 50000000},
	}

	for _, tt := range tests {
		got := toBytes(tt.size, tt.unit)
		if got != tt.want {
			t.Errorf("toBytes(%v, %q) = %d, want %d", tt.size, tt.unit, got, tt.want)
		}
	}
}
