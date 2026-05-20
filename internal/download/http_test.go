package download

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tjst-t/dlrelay/internal/model"
)

func TestSanitizeOutputFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "video.mp4", "video.mp4"},
		{"leading empty comma fields", ", , 本文.mp4", "本文.mp4"},
		{"leading commas no spaces", ",,title.mp4", "title.mp4"},
		{"comma between words", "foo, bar.mp4", "foo， bar.mp4"},
		{"only commas and spaces", ", , .mp4", "video.mp4"},
		{"japanese fullwidth space prefix", "　タイトル.mp4", "タイトル.mp4"},
		{"no extension", ",,name", "name"},
		{"title with commas only", "a,b,c.mp4", "a，b，c.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOutputFilename(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeOutputFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeOutputFilenameTruncates(t *testing.T) {
	// Long Japanese title that previously triggered:
	//   - [Errno 36] ENAMETOOLONG  on ext4 (255-byte limit) before truncation
	//   - [Errno 95] ENOTSUP       on a NAS share at 244 bytes
	// We now cap at outputFilenameMaxBytes (180) to stay safely under both.
	longTitle := "大人のパフォーマーによる熱くて情熱的なシーンをご覧ください。" +
		"この魅力的なビデオでは、親密で官能的な瞬間を体験できます。" +
		"大人向けコンテンツ愛好家に最適です。 エロ動画 - SpankBang.mp4"
	got := sanitizeOutputFilename(longTitle)

	// Must fit within the conservative output limit even after yt-dlp's
	// ".ytdl" suffix and uniquePath's potential "_N" collision suffix.
	worstCase := got + "_9999.ytdl"
	if len(worstCase) > outputFilenameMaxBytes {
		t.Errorf("sanitized name + worst-case suffix is %d bytes (max %d): %q",
			len(worstCase), outputFilenameMaxBytes, worstCase)
	}

	// Extension must be preserved.
	if filepath.Ext(got) != ".mp4" {
		t.Errorf("extension changed: got %q, want %q", filepath.Ext(got), ".mp4")
	}

	// Result must still be valid UTF-8 (no mid-rune cut).
	if !utf8.ValidString(got) {
		t.Errorf("truncated filename is not valid UTF-8: %q", got)
	}

	t.Logf("input %d bytes → output %d bytes (worst-case %d)",
		len(longTitle), len(got), len(worstCase))
}

func TestSanitizePathLength(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantFit  bool // result filename should fit in 255 bytes
	}{
		{"short", "video.mp4", true},
		{"exactly 255", strings.Repeat("a", 251) + ".mp4", true},
		{"over 255 ascii", strings.Repeat("a", 300) + ".mp4", true},
		{"long japanese", "長いファイル名テスト_【テスト用の非常に長い日本語ファイル名】動画ダウンロードテスト用データ＿サンプルビデオファイル名が二百五十五バイトを超える場合の処理確認テスト＿追加テキストで更に長くする＿まだまだ続く長いファイル名＿これで十分な長さになるはず_720p.m3u8", true},
		{"long ext", strings.Repeat("a", 300) + ".m3u8", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join("/tmp", tt.filename)
			result := sanitizePathLength(path)
			base := filepath.Base(result)
			if len(base) > maxFilenameBytes {
				t.Errorf("filename too long: %d bytes (max %d)", len(base), maxFilenameBytes)
			}
			// Extension should be preserved
			if filepath.Ext(result) != filepath.Ext(path) {
				t.Errorf("extension changed: got %q, want %q", filepath.Ext(result), filepath.Ext(path))
			}
			t.Logf("  %d → %d bytes: %s", len(tt.filename), len(base), base[:min(80, len(base))])
		})
	}
}

func TestUniquePathLongFilename(t *testing.T) {
	dir := t.TempDir()
	longName := strings.Repeat("あ", 100) + ".mp4" // 300 bytes + 4 = 304 bytes
	path := filepath.Join(dir, longName)
	result := uniquePath(path)
	base := filepath.Base(result)
	if len(base) > maxFilenameBytes {
		t.Fatalf("uniquePath returned filename > %d bytes: %d", maxFilenameBytes, len(base))
	}
	// Should be able to create the file
	f, err := os.Create(result)
	if err != nil {
		t.Fatalf("cannot create file: %v", err)
	}
	f.Close()
	os.Remove(result)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSpecialCharFilename(t *testing.T) {
	content := "test content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	mgr := NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	tests := []struct {
		name     string
		filename string
	}{
		{"spaces", "my video file.mp4"},
		{"japanese", "テスト動画.mp4"},
		{"special chars", "video (1) [720p].mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := mgr.Submit(model.DownloadRequest{
				URL:      srv.URL + "/test.bin",
				Filename: tt.filename,
			})
			if err != nil {
				t.Fatalf("Submit failed: %v", err)
			}

			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				status, _ := mgr.Get(id)
				if status.State == model.StateCompleted || status.State == model.StateFailed {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

			status, _ := mgr.Get(id)
			if status.State != model.StateCompleted {
				t.Fatalf("expected completed for filename %q, got %s (err: %v)", tt.filename, status.State, status.Error)
			}
		})
	}
}

func TestEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		// Write nothing
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	mgr := NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      srv.URL + "/empty.bin",
		Filename: "empty.bin",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	// Empty responses should complete (0-byte file is valid)
	if status.State != model.StateCompleted {
		t.Fatalf("expected completed for empty response, got %s", status.State)
	}
}

func TestHTTPResumeOnRetry(t *testing.T) {
	// Server that sends partial content then drops connection
	callCount := 0
	fullContent := "abcdefghijklmnopqrstuvwxyz0123456789"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		rangeHeader := r.Header.Get("Range")

		if callCount == 1 {
			// First request: send partial data then close
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fullContent[:10])) // Only send 10 bytes
			// Connection drops here (client will get an error)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				if conn != nil {
					conn.Close()
				}
			}
			return
		}

		// Second request: should have Range header for resume
		if rangeHeader != "" {
			// Parse range
			var start int64
			fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if start > 0 && int(start) < len(fullContent) {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)-int(start)))
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(fullContent)-1, len(fullContent)))
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fullContent[start:]))
				return
			}
		}
		// Fallback: send full content
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)))
		w.Write([]byte(fullContent))
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	mgr := NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      srv.URL + "/test.bin",
		Filename: "resume_test.bin",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for first attempt to fail
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateFailed || status.State == model.StateCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	if status.State == model.StateCompleted {
		t.Log("Download completed on first try (hijack not supported), skipping resume test")
		return
	}
	if status.State != model.StateFailed {
		t.Fatalf("expected failed, got %s", status.State)
	}

	// Retry — should attempt to resume
	if err := mgr.Retry(id); err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ = mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ = mgr.Get(id)
	if status.State != model.StateCompleted {
		t.Fatalf("expected completed after retry, got %s (error: %v)", status.State, status.Error)
	}
	t.Logf("Resume test: %d HTTP requests made", callCount)
}

func TestNetworkTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond - just hang
		<-r.Context().Done()
	}))
	defer srv.Close()

	downloadDir := t.TempDir()
	mgr := NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      srv.URL + "/hang.bin",
		Filename: "hang.bin",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Cancel after 1 second to simulate timeout handling
	time.Sleep(time.Second)
	mgr.Cancel(id)

	status, _ := mgr.Get(id)
	if status.State != model.StateCancelled {
		t.Fatalf("expected cancelled, got %s", status.State)
	}
}
