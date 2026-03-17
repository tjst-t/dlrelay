package download

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tjst-t/dlrelay/internal/model"
)

// progressRe matches yt-dlp progress lines like:
// [download]  42.3% of  125.50MiB at  5.23MiB/s ETA 00:14
var progressRe = regexp.MustCompile(`\[download\]\s+([\d.]+)%\s+of\s+~?([\d.]+)(\w+)`)

// validHeaderName allows only safe HTTP header names.
var validHeaderName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// validQuality allows safe yt-dlp format selector characters.
var validQuality = regexp.MustCompile(`^[a-zA-Z0-9+/\[\]:,\-*><=. ]+$`)

// filenameRe matches yt-dlp destination lines like:
// [download] Destination: /path/to/video.mp4
// [Merger] Merging formats into "/path/to/video.mp4"
var filenameRe = regexp.MustCompile(`(?:Destination:\s*|Merging formats into "?)([^"\n]+)`)

// YtdlpFilename uses yt-dlp --print filename to determine the output filename
// without actually downloading. Returns the base filename (e.g. "video.mp4").
func YtdlpFilename(ctx context.Context, req model.DownloadRequest) (string, error) {
	args := []string{
		"--no-playlist",
		"--print", "filename",
		"--merge-output-format", "mp4",
		"--js-runtimes", "node",
		"--remote-components", "ejs:github",
	}

	// Use same output template logic as YtdlpDownload
	outTemplate := "%(title)s.%(ext)s"
	if req.Filename != "" {
		baseName := filepath.Base(req.Filename)
		ext := filepath.Ext(baseName)
		name := strings.TrimSuffix(baseName, ext)
		name = strings.ReplaceAll(name, "%", "%%")
		if name != "" {
			outTemplate = name + ".%(ext)s"
		}
	}
	args = append(args, "-o", outTemplate)

	quality := req.Quality
	if quality == "" {
		quality = "bestvideo+bestaudio/best"
	}
	if !validQuality.MatchString(quality) {
		return "", fmt.Errorf("invalid quality format selector: %q", quality)
	}
	args = append(args, "-f", quality)

	// Forward cookies via cookie file (same as YtdlpDownload)
	var cookieFile string
	if cookieStr := req.Headers["Cookie"]; cookieStr != "" {
		f, err := os.CreateTemp("", "ytdlp-cookies-*.txt")
		if err == nil {
			cookieFile = f.Name()
			host := extractHost(req.URL)
			writeCookieFile(f, host, cookieStr)
			f.Close()
			args = append(args, "--cookies", cookieFile)
		}
	}

	// Forward non-Cookie headers
	for k, v := range req.Headers {
		if strings.EqualFold(k, "Cookie") {
			continue
		}
		if !validHeaderName.MatchString(k) {
			continue
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			continue
		}
		args = append(args, "--add-header", k+":"+v)
	}

	args = append(args, req.URL)

	if cookieFile != "" {
		defer os.Remove(cookieFile)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp --print filename failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// YtdlpDownload uses yt-dlp to download a video from a page URL.
func YtdlpDownload(ctx context.Context, task *Task, downloadDir string, bandwidthLimit int64) error {
	task.SetState(model.StateDownloading)

	// Determine output directory
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

	// Build output template
	// If a filename is provided, use it (minus extension, yt-dlp adds its own)
	outTemplate := filepath.Join(dir, "%(title)s.%(ext)s")
	if task.req.Filename != "" {
		baseName := filepath.Base(task.req.Filename)
		// Remove extension — yt-dlp will determine the correct one
		ext := filepath.Ext(baseName)
		name := strings.TrimSuffix(baseName, ext)
		// Escape % to prevent yt-dlp template injection (e.g. %(title)s)
		name = strings.ReplaceAll(name, "%", "%%")
		if name != "" {
			outTemplate = filepath.Join(dir, name+".%(ext)s")
		}
	}

	// Build yt-dlp command
	args := []string{
		"--no-playlist",
		"--newline", // Output progress on separate lines
		"-o", outTemplate,
		"--no-overwrites",
		"--continue",                        // Resume partially downloaded files
		"--merge-output-format", "mp4",
		"--js-runtimes", "node",
		"--remote-components", "ejs:github", // YouTube n-challenge solver script
	}

	// Quality/format selection
	quality := task.req.Quality
	if quality == "" {
		quality = "bestvideo+bestaudio/best"
	}
	if !validQuality.MatchString(quality) {
		return fmt.Errorf("invalid quality format selector: %q", quality)
	}
	args = append(args, "-f", quality)

	// Apply bandwidth limit via yt-dlp's --limit-rate
	if bandwidthLimit > 0 {
		args = append(args, "--limit-rate", fmt.Sprintf("%d", bandwidthLimit))
	}

	// Write a Netscape-format cookie file if Cookie header is present.
	// yt-dlp's extractors use an internal cookiejar — passing cookies via
	// --cookies is more reliable than --add-header Cookie: which only
	// affects the initial HTTP request.
	var cookieFile string
	if cookieStr := task.req.Headers["Cookie"]; cookieStr != "" {
		f, err := os.CreateTemp("", "ytdlp-cookies-*.txt")
		if err == nil {
			cookieFile = f.Name()
			host := extractHost(task.req.URL)
			writeCookieFile(f, host, cookieStr)
			f.Close()
			args = append(args, "--cookies", cookieFile)
		}
	}

	// Pass non-Cookie headers (validated to prevent argument injection)
	for k, v := range task.req.Headers {
		if strings.EqualFold(k, "Cookie") {
			continue // Already handled via --cookies file
		}
		if !validHeaderName.MatchString(k) {
			continue
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			continue
		}
		args = append(args, "--add-header", k+":"+v)
	}

	args = append(args, task.req.URL)

	if cookieFile != "" {
		defer os.Remove(cookieFile)
	}

	slog.Info("starting yt-dlp", "url", task.req.URL, "dir", dir)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Dir = dir

	// Capture stderr for error messages
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start yt-dlp: %w (is yt-dlp installed?)", err)
	}

	// Parse progress from stdout; collect stderr in a separate goroutine
	var lastFilename string
	var stderrLines []string
	var stderrMu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			slog.Debug("yt-dlp stderr", "url", task.req.URL, "line", line)
			stderrMu.Lock()
			stderrLines = append(stderrLines, line)
			if len(stderrLines) > 50 {
				stderrLines = stderrLines[len(stderrLines)-50:]
			}
			stderrMu.Unlock()
		}
	}()

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("yt-dlp stdout", "url", task.req.URL, "line", line)

		// Parse progress
		if m := progressRe.FindStringSubmatch(line); m != nil {
			pct, _ := strconv.ParseFloat(m[1], 64)
			sizeVal, _ := strconv.ParseFloat(m[2], 64)
			unit := strings.ToLower(m[3])
			totalBytes := toBytes(sizeVal, unit)
			received := int64(float64(totalBytes) * pct / 100)
			task.SetProgress(received, totalBytes)
		}

		// Capture output filename
		if m := filenameRe.FindStringSubmatch(line); m != nil {
			lastFilename = strings.TrimSpace(m[1])
		}
	}

	// Wait for stderr goroutine to finish before reading stderrLines
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		errMsg := fmt.Sprintf("yt-dlp failed: %v", err)
		if len(stderrLines) > 0 {
			// Include last few lines of stderr for context
			last := stderrLines
			if len(last) > 5 {
				last = last[len(last)-5:]
			}
			errMsg += "\n" + strings.Join(last, "\n")
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Update filename in task if yt-dlp reported one
	if lastFilename != "" {
		task.mu.Lock()
		task.req.Filename = filepath.Base(lastFilename)
		task.mu.Unlock()
		task.SetFilePath(lastFilename)
		slog.Info("yt-dlp completed", "file", lastFilename)
	}

	task.SetState(model.StateCompleted)
	return nil
}

func toBytes(size float64, unit string) int64 {
	switch unit {
	case "kib":
		return int64(size * 1024)
	case "mib":
		return int64(size * 1024 * 1024)
	case "gib":
		return int64(size * 1024 * 1024 * 1024)
	case "kb":
		return int64(size * 1000)
	case "mb":
		return int64(size * 1000 * 1000)
	case "gb":
		return int64(size * 1000 * 1000 * 1000)
	case "b":
		return int64(size)
	default:
		return int64(size * 1024 * 1024) // Default to MiB
	}
}

// extractHost returns the hostname from a URL (e.g. "music.youtube.com").
func extractHost(rawURL string) string {
	// Quick parse — avoid importing net/url just for hostname.
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	// Strip port
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

// writeCookieFile writes a Netscape-format cookie file from a Cookie header string.
// This format is understood by yt-dlp's --cookies flag.
func writeCookieFile(f *os.File, host string, cookieHeader string) {
	fmt.Fprintln(f, "# Netscape HTTP Cookie File")
	// Domain for cookie scoping: ".example.com" to include subdomains
	domain := host
	if !strings.HasPrefix(domain, ".") {
		// Use parent domain for broader matching (e.g. ".youtube.com" for "music.youtube.com")
		parts := strings.Split(domain, ".")
		if len(parts) > 2 {
			domain = "." + strings.Join(parts[len(parts)-2:], ".")
		} else {
			domain = "." + domain
		}
	}
	for _, pair := range strings.Split(cookieHeader, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, _ := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			continue
		}
		// Format: domain include_subdomains path secure expiry name value
		fmt.Fprintf(f, "%s\tTRUE\t/\tFALSE\t0\t%s\t%s\n", domain, name, value)
	}
}
