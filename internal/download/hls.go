package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

// HLSDownload downloads an HLS stream.
func HLSDownload(ctx context.Context, task *Task, downloadDir, tempDir string, bandwidthLimit int64) error {
	task.SetState(model.StateDownloading)

	slog.Info("starting HLS download", "url", task.req.URL)

	segments, err := parseM3U8(ctx, task.req.URL, task.req.Headers)
	if err != nil {
		return NewDownloadError(ErrNetwork, "failed to parse M3U8", err)
	}
	if len(segments) == 0 {
		return NewDownloadError(ErrValidation, "no segments found in M3U8", nil)
	}

	slog.Info("HLS playlist parsed", "url", task.req.URL, "segments", len(segments))

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
		filename = "video.mp4"
	}
	// Replace streaming extensions with .mp4 for the final output
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".m3u8" || ext == ".ts" {
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".mp4"
	}
	destPath := uniquePath(filepath.Join(dir, filename))

	tmpFile, err := os.CreateTemp(tempDir, "dlrelay-hls-*.ts")
	if err != nil {
		return NewDownloadError(ErrFileSystem, "failed to create temp file", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	// Track actual bytes downloaded for accurate progress reporting.
	var bytesDownloaded int64
	totalSegments := len(segments)
	// Start with 0 estimated total — update after first segment
	task.SetProgress(0, 0)

	for i, segURL := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := downloadSegmentCounted(ctx, segURL, task.req.Headers, tmpFile, bandwidthLimit)
		if err != nil {
			return NewDownloadError(ErrNetwork, fmt.Sprintf("failed to download segment %d/%d", i+1, totalSegments), err)
		}
		bytesDownloaded += n
		// Update estimate based on average segment size so far
		avgSegSize := bytesDownloaded / int64(i+1)
		estimatedTotal := avgSegSize * int64(totalSegments)
		task.SetProgress(bytesDownloaded, estimatedTotal)
	}

	slog.Info("HLS segments downloaded", "url", task.req.URL, "segments", totalSegments, "bytes", bytesDownloaded)
	tmpFile.Close()

	// Remux from MPEG-TS to MP4 using ffmpeg (codec copy, no re-encoding).
	// This makes the output playable in standard video players.
	if strings.HasSuffix(strings.ToLower(destPath), ".mp4") {
		if err := remuxToMP4(ctx, tmpPath, destPath); err != nil {
			slog.Warn("ffmpeg remux failed, saving as .ts", "err", err)
			// Fallback: save as .ts if ffmpeg is not available or fails
			destPath = strings.TrimSuffix(destPath, ".mp4") + ".ts"
			destPath = uniquePath(destPath)
			if mvErr := moveFile(tmpPath, destPath); mvErr != nil {
				return fmt.Errorf("failed to save file: %w", mvErr)
			}
		}
	} else {
		if err := moveFile(tmpPath, destPath); err != nil {
			return fmt.Errorf("failed to save file: %w", err)
		}
	}

	// Update filename in task status to reflect actual output
	task.mu.Lock()
	task.req.Filename = filepath.Base(destPath)
	task.mu.Unlock()

	task.SetFilePath(destPath)
	slog.Info("HLS download completed", "url", task.req.URL, "file", destPath)
	task.SetProgressAndState(bytesDownloaded, bytesDownloaded, model.StateCompleted)
	return nil
}

// remuxToMP4 remuxes MPEG-TS to MP4 using ffmpeg with codec copy (no re-encoding).
func remuxToMP4(ctx context.Context, inputPath, outputPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	slog.Info("remuxing HLS to MP4", "input", filepath.Base(inputPath), "output", filepath.Base(outputPath))

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", inputPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remux failed: %w: %s", err, string(output))
	}
	return nil
}

// moveFile moves a file, falling back to copy if rename fails (cross-device).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return copyFile(src, dst)
	}
	return nil
}

func parseM3U8(ctx context.Context, m3u8URL string, headers map[string]string) ([]string, error) {
	// Use a shorter timeout for manifest fetch — if the CDN is unresponsive,
	// fail fast rather than hanging for the full download timeout.
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	body, err := fetchURL(fetchCtx, m3u8URL, headers)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	base, err := url.Parse(m3u8URL)
	if err != nil {
		return nil, err
	}

	var segments []string
	var variantURLs []string
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
				// Master playlist — next line is variant URL
				if scanner.Scan() {
					variantLine := strings.TrimSpace(scanner.Text())
					variantURLs = append(variantURLs, resolveURL(base, variantLine))
				}
			}
			continue
		}
		segments = append(segments, resolveURL(base, line))
	}

	// If this is a master playlist, use the last variant (typically highest quality)
	if len(variantURLs) > 0 && len(segments) == 0 {
		return parseM3U8(ctx, variantURLs[len(variantURLs)-1], headers)
	}

	return segments, nil
}

func resolveURL(base *url.URL, ref string) string {
	refURL, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(refURL).String()
}

func downloadSegmentCounted(ctx context.Context, segURL string, headers map[string]string, w io.Writer, bandwidthLimit int64) (int64, error) {
	// Retry with exponential backoff for transient CDN failures
	const maxRetries = 6
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s, 16s, 32s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			slog.Info("HLS segment retry", "url", segURL, "attempt", attempt+1, "backoff", backoff)
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(backoff):
			}
		}
		segCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		body, err := fetchURL(segCtx, segURL, headers)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		var reader io.Reader = body
		reader = NewThrottledReader(segCtx, reader, bandwidthLimit)
		n, err := io.Copy(w, reader)
		body.Close()
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		return n, nil
	}
	return 0, lastErr
}

func fetchURL(ctx context.Context, rawURL string, headers map[string]string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}
	return resp.Body, nil
}
