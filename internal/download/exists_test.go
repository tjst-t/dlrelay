package download

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExistingFile(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(dir, "PHOTO.JPG"), []byte("data"), 0o644)

	// Create a subdirectory with a file
	subDir := filepath.Join(dir, "sub")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "nested.mkv"), []byte("data"), 0o644)

	tests := []struct {
		name      string
		filename  string
		dirs      []string
		wantFound bool
		wantBase  string
	}{
		{
			name:      "exact match same extension",
			filename:  "video.mp4",
			dirs:      []string{dir},
			wantFound: true,
			wantBase:  "video.mp4",
		},
		{
			name:      "match with different extension",
			filename:  "video.mkv",
			dirs:      []string{dir},
			wantFound: true,
			wantBase:  "video.mp4",
		},
		{
			name:      "case insensitive match",
			filename:  "photo.png",
			dirs:      []string{dir},
			wantFound: true,
			wantBase:  "PHOTO.JPG",
		},
		{
			name:      "no match",
			filename:  "nonexistent.mp4",
			dirs:      []string{dir},
			wantFound: false,
		},
		{
			name:      "recursive search in subdirectory",
			filename:  "nested.mp4",
			dirs:      []string{dir},
			wantFound: true,
			wantBase:  "nested.mkv",
		},
		{
			name:      "empty dirs",
			filename:  "video.mp4",
			dirs:      nil,
			wantFound: false,
		},
		{
			name:      "empty filename",
			filename:  "",
			dirs:      []string{dir},
			wantFound: false,
		},
		{
			name:      "nonexistent directory",
			filename:  "video.mp4",
			dirs:      []string{"/nonexistent/path/that/does/not/exist"},
			wantFound: false,
		},
		{
			name:      "multiple dirs second matches",
			filename:  "nested.webm",
			dirs:      []string{t.TempDir(), dir},
			wantFound: true,
			wantBase:  "nested.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindExistingFile(tt.filename, tt.dirs)
			if tt.wantFound {
				if result == "" {
					t.Fatal("expected to find file, got empty string")
				}
				if filepath.Base(result) != tt.wantBase {
					t.Errorf("expected base name %q, got %q", tt.wantBase, filepath.Base(result))
				}
			} else {
				if result != "" {
					t.Errorf("expected no match, got %q", result)
				}
			}
		})
	}
}
