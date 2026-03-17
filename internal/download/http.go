package download

import (
	"context"
	"errors"
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

// HTTPDownload performs a plain HTTP download with optional resume support.
func HTTPDownload(ctx context.Context, task *Task, downloadDir, tempDir string, resumeFrom int64, bandwidthLimit int64) (downloadErr error) {
	task.SetState(model.StateDownloading)

	dir := downloadDir
	if task.req.Directory != "" {
		var err error
		dir, err = safePath(downloadDir, task.req.Directory)
		if err != nil {
			return NewDownloadError(ErrValidation, "invalid directory", err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return NewDownloadError(ErrFileSystem, "failed to create directory", err)
	}

	filename := filepath.Base(task.req.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = filepath.Base(task.req.URL)
	}
	destPath := filepath.Join(dir, filename)
	destPath = uniquePath(destPath)

	// Handle resume: reuse existing temp file or create new one
	var tmpFile *os.File
	var tmpPath string
	var written int64

	task.mu.RLock()
	existingTempPath := task.tempPath
	task.mu.RUnlock()

	if resumeFrom > 0 && existingTempPath != "" {
		// Try to resume from existing temp file
		var err error
		tmpFile, err = os.OpenFile(existingTempPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			// Can't resume, start fresh
			resumeFrom = 0
		} else {
			tmpPath = existingTempPath
			written = resumeFrom
		}
	}

	if tmpFile == nil {
		var err error
		tmpFile, err = os.CreateTemp(tempDir, "dlrelay-dl-*")
		if err != nil {
			return NewDownloadError(ErrFileSystem, "failed to create temp file", err)
		}
		tmpPath = tmpFile.Name()
		resumeFrom = 0
		written = 0
	}

	task.SetTempPath(tmpPath)
	defer func() {
		tmpFile.Close()
		// Keep temp file on network/transient errors so retry can resume.
		// Only remove on success or validation/filesystem errors where resume won't help.
		if downloadErr == nil {
			os.Remove(tmpPath)
		} else {
			var dlErr *DownloadError
			if errors.As(downloadErr, &dlErr) && (dlErr.Kind == ErrValidation || dlErr.Kind == ErrFileSystem) {
				os.Remove(tmpPath)
				task.SetTempPath("")
			}
			// Network errors: keep temp file for resume on retry
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, task.req.URL, nil)
	if err != nil {
		return NewDownloadError(ErrNetwork, "failed to create request", err)
	}
	for k, v := range task.req.Headers {
		req.Header.Set(k, v)
	}

	// Add Range header for resume
	if resumeFrom > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeFrom))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return NewDownloadError(ErrNetwork, "HTTP request failed", err)
	}
	defer resp.Body.Close()

	// Handle resume response
	if resumeFrom > 0 {
		if resp.StatusCode == http.StatusPartialContent {
			// Server supports resume, continue from where we left off
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Server doesn't support resume, start over
			tmpFile.Close()
			tmpFile, err = os.Create(tmpPath)
			if err != nil {
				return NewDownloadError(ErrFileSystem, "failed to recreate temp file", err)
			}
			written = 0
		} else {
			return NewDownloadError(ErrNetwork, fmt.Sprintf("HTTP status %d", resp.StatusCode), nil)
		}
	} else {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return NewDownloadError(ErrNetwork, fmt.Sprintf("HTTP status %d", resp.StatusCode), nil)
		}
	}

	totalBytes, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if totalBytes > 0 && resumeFrom > 0 && resp.StatusCode == http.StatusPartialContent {
		totalBytes += resumeFrom
	}
	task.SetProgress(written, totalBytes)

	var body io.Reader = resp.Body
	body = NewThrottledReader(ctx, body, bandwidthLimit)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				return NewDownloadError(ErrFileSystem, "failed to write", writeErr)
			}
			written += int64(n)
			task.SetProgress(written, totalBytes)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return NewDownloadError(ErrNetwork, "read error", readErr)
		}
	}

	tmpFile.Close()
	task.SetTempPath("") // Clear temp path on successful completion
	if err := os.Rename(tmpPath, destPath); err != nil {
		// Cross-device: copy instead
		if copyErr := copyFile(tmpPath, destPath); copyErr != nil {
			return NewDownloadError(ErrFileSystem, "failed to move file", copyErr)
		}
	}

	task.SetFilePath(destPath)
	task.SetProgressAndState(written, totalBytes, model.StateCompleted)
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
