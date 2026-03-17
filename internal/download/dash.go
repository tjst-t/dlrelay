package download

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

// DASHDownload downloads separate video and audio streams and muxes them with ffmpeg.
func DASHDownload(ctx context.Context, task *Task, downloadDir, tempDir string, bandwidthLimit int64) error {
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
		filename = "video.mp4"
	}
	destPath := uniquePath(filepath.Join(dir, filename))

	// Download video stream
	videoTmp, err := os.CreateTemp(tempDir, "dlrelay-dash-video-*")
	if err != nil {
		return NewDownloadError(ErrFileSystem, "failed to create video temp file", err)
	}
	videoPath := videoTmp.Name()
	videoTmp.Close()
	defer os.Remove(videoPath)

	task.SetProgress(0, 3) // 3 steps: video, audio, mux

	if err := downloadToFile(ctx, task.req.URL, task.req.Headers, videoPath, bandwidthLimit); err != nil {
		return NewDownloadError(ErrNetwork, "failed to download video stream", err)
	}
	task.SetProgress(1, 3)

	// Download audio stream
	audioTmp, err := os.CreateTemp(tempDir, "dlrelay-dash-audio-*")
	if err != nil {
		return NewDownloadError(ErrFileSystem, "failed to create audio temp file", err)
	}
	audioPath := audioTmp.Name()
	audioTmp.Close()
	defer os.Remove(audioPath)

	// NOTE: This passes the same headers (including cookies) to both video and audio URLs.
	// If audio_url is on a different domain, cookies may leak cross-domain.
	// A proper fix would require per-URL header maps in the DownloadRequest model.
	if err := downloadToFile(ctx, task.req.AudioURL, task.req.Headers, audioPath, bandwidthLimit); err != nil {
		return NewDownloadError(ErrNetwork, "failed to download audio stream", err)
	}
	task.SetProgress(2, 3)

	// Mux video + audio with ffmpeg
	if err := muxStreams(ctx, videoPath, audioPath, destPath); err != nil {
		return NewDownloadError(ErrExternal, "failed to mux streams", err)
	}
	task.SetFilePath(destPath)
	task.SetProgressAndState(3, 3, model.StateCompleted)
	return nil
}

// downloadToFile downloads a URL to a file path.
func downloadToFile(ctx context.Context, rawURL string, headers map[string]string, destPath string, bandwidthLimit int64) error {
	body, err := fetchURL(ctx, rawURL, headers)
	if err != nil {
		return err
	}
	defer body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var reader io.Reader = body
	reader = NewThrottledReader(ctx, reader, bandwidthLimit)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}
	return nil
}

// muxStreams combines video and audio into a single file using ffmpeg.
func muxStreams(ctx context.Context, videoPath, audioPath, outputPath string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", videoPath,
		"-i", audioPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg mux failed: %w: %s", err, string(output))
	}
	return nil
}
