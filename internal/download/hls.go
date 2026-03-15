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
func HLSDownload(ctx context.Context, task *Task, downloadDir, tempDir string) error {
	task.SetState(model.StateDownloading)

	slog.Info("starting HLS download", "url", task.req.URL)

	segments, err := parseM3U8(ctx, task.req.URL, task.req.Headers)
	if err != nil {
		return fmt.Errorf("failed to parse M3U8: %w", err)
	}
	if len(segments) == 0 {
		return fmt.Errorf("no segments found in M3U8")
	}

	slog.Info("HLS playlist parsed", "url", task.req.URL, "segments", len(segments))

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
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	// Track actual bytes downloaded for accurate progress reporting.
	var bytesDownloaded int64
	totalSegments := len(segments)
	// Estimate total bytes for progress display: assume 500KB per segment.
	estimatedTotal := int64(totalSegments) * 500 * 1024
	task.SetProgress(0, estimatedTotal)

	for i, segURL := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := downloadSegmentCounted(ctx, segURL, task.req.Headers, tmpFile)
		if err != nil {
			return fmt.Errorf("failed to download segment %d/%d: %w", i+1, totalSegments, err)
		}
		bytesDownloaded += n
		// Update estimate based on average segment size so far
		avgSegSize := bytesDownloaded / int64(i+1)
		estimatedTotal = avgSegSize * int64(totalSegments)
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
	task.SetState(model.StateCompleted)
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

func downloadSegment(ctx context.Context, segURL string, headers map[string]string, w io.Writer) error {
	_, err := downloadSegmentCounted(ctx, segURL, headers, w)
	return err
}

func downloadSegmentCounted(ctx context.Context, segURL string, headers map[string]string, w io.Writer) (int64, error) {
	// Retry up to 3 times for transient network failures
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		segCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		body, err := fetchURL(segCtx, segURL, headers)
		if err != nil {
			cancel()
			lastErr = err
			slog.Debug("HLS segment fetch failed, retrying", "url", segURL, "attempt", attempt+1, "err", err)
			continue
		}
		n, err := io.Copy(w, body)
		body.Close()
		cancel()
		if err != nil {
			lastErr = err
			slog.Debug("HLS segment read failed, retrying", "url", segURL, "attempt", attempt+1, "err", err)
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
