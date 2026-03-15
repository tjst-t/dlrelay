package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tjst-t/dlrelay/internal/model"
)

// safePath returns a safe subdirectory path within baseDir.
// It rejects paths that would escape baseDir via ".." traversal.
func safePath(baseDir, subDir string) (string, error) {
	joined := filepath.Join(baseDir, subDir)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}
	// Ensure the resolved path is under baseDir
	if !strings.HasPrefix(abs+string(filepath.Separator), baseAbs+string(filepath.Separator)) && abs != baseAbs {
		return "", fmt.Errorf("path escapes download directory")
	}
	return abs, nil
}

// HTTPDownload performs a plain HTTP download.
func HTTPDownload(ctx context.Context, task *Task, downloadDir, tempDir string) error {
	task.SetState(model.StateDownloading)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.req.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range task.req.Headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	totalBytes, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	task.SetProgress(0, totalBytes)

	dir := downloadDir
	if task.req.Directory != "" {
		var err error
		dir, err = safePath(downloadDir, task.req.Directory)
		if err != nil {
			return fmt.Errorf("invalid directory: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := filepath.Base(task.req.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = filepath.Base(task.req.URL)
	}
	destPath := filepath.Join(dir, filename)
	destPath = uniquePath(destPath)

	tmpFile, err := os.CreateTemp(tempDir, "dlrelay-dl-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write: %w", writeErr)
			}
			written += int64(n)
			task.SetProgress(written, totalBytes)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read error: %w", readErr)
		}
	}

	tmpFile.Close()
	if err := os.Rename(tmpPath, destPath); err != nil {
		// Cross-device: copy instead
		if copyErr := copyFile(tmpPath, destPath); copyErr != nil {
			return fmt.Errorf("failed to move file: %w", copyErr)
		}
	}

	task.SetFilePath(destPath)
	task.SetState(model.StateCompleted)
	return nil
}

// maxFilenameBytes is the maximum filename length in bytes for most filesystems.
const maxFilenameBytes = 255

// sanitizePathLength truncates the filename component of a path to fit within
// the filesystem's maximum filename length (255 bytes on ext4/btrfs/etc.).
func sanitizePathLength(path string) string {
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	if len(name) <= maxFilenameBytes {
		return path
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	// Truncate base to fit: maxFilenameBytes - len(ext) - safety margin for _N suffix
	maxBase := maxFilenameBytes - len(ext) - 10
	if maxBase < 20 {
		maxBase = 20
	}
	// Truncate at a valid UTF-8 boundary
	truncated := base
	for len(truncated) > maxBase {
		// Remove last rune
		r := []rune(truncated)
		truncated = string(r[:len(r)-1])
	}
	return filepath.Join(dir, truncated+ext)
}

func uniquePath(path string) string {
	path = sanitizePathLength(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 1; i < 10000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	// Fallback: should never happen
	return path
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
