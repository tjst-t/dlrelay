package download

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
