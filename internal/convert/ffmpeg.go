package convert

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

var timeRegex = regexp.MustCompile(`time=(\d+):(\d+):(\d+)\.(\d+)`)

// validateProbeHeaders rejects header names or values containing CR, LF, or null bytes.
func validateProbeHeaders(headers map[string]string) error {
	for k, v := range headers {
		if strings.ContainsAny(k, "\r\n\x00") {
			return fmt.Errorf("header name contains forbidden character: %q", k)
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			return fmt.Errorf("header value contains forbidden character for key %q", k)
		}
	}
	return nil
}

// Probe runs ffprobe on the given input and returns the result.
func Probe(ctx context.Context, input string, headers map[string]string) (*model.ProbeResult, error) {
	if err := validateProbeHeaders(headers); err != nil {
		return nil, fmt.Errorf("invalid probe headers: %w", err)
	}

	args := []string{"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams"}
	for k, v := range headers {
		args = append(args, "-headers", fmt.Sprintf("%s: %s\r\n", k, v))
	}
	args = append(args, input)

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var raw struct {
		Format  json.RawMessage `json:"format"`
		Streams json.RawMessage `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	return &model.ProbeResult{Format: raw.Format, Streams: raw.Streams}, nil
}

// ListCodecs returns all available FFmpeg codecs.
func ListCodecs(ctx context.Context) ([]model.Codec, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-codecs", "-hide_banner")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg -codecs failed: %w", err)
	}

	var codecs []model.Codec
	scanner := bufio.NewScanner(bytes.NewReader(out))
	headerDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if !headerDone {
			if strings.HasPrefix(line, " ------") {
				headerDone = true
			}
			continue
		}
		if len(line) < 8 {
			continue
		}
		flags := line[:7]
		rest := strings.TrimSpace(line[7:])
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 2 {
			continue
		}

		codecType := "other"
		if flags[2] == 'V' {
			codecType = "video"
		} else if flags[2] == 'A' {
			codecType = "audio"
		} else if flags[2] == 'S' {
			codecType = "subtitle"
		}

		codecs = append(codecs, model.Codec{
			Name:        parts[0],
			Description: strings.TrimSpace(parts[1]),
			Type:        codecType,
			CanDecode:   flags[0] == 'D',
			CanEncode:   flags[1] == 'E',
		})
	}
	return codecs, nil
}

// ListFormats returns all available FFmpeg formats.
func ListFormats(ctx context.Context) ([]model.Format, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-formats", "-hide_banner")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg -formats failed: %w", err)
	}

	var formats []model.Format
	scanner := bufio.NewScanner(bytes.NewReader(out))
	headerDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if !headerDone {
			if strings.HasPrefix(line, " --") {
				headerDone = true
			}
			continue
		}
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		flags := line[:3]
		rest := strings.TrimSpace(line[3:])
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 2 {
			continue
		}

		formats = append(formats, model.Format{
			Name:        strings.TrimSpace(parts[0]),
			Description: strings.TrimSpace(parts[1]),
			CanDemux:    flags[0] == 'D' || flags[1] == 'D',
			CanMux:      flags[0] == 'E' || flags[1] == 'E',
		})
	}
	return formats, nil
}

// RunConvert runs an FFmpeg conversion with the given arguments and reports progress.
func RunConvert(ctx context.Context, args []string, totalDuration time.Duration, progressCb func(float64)) error {
	fullArgs := append([]string{"-y", "-progress", "pipe:2"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", fullArgs...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if totalDuration > 0 && progressCb != nil {
			if matches := timeRegex.FindStringSubmatch(line); matches != nil {
				h, _ := strconv.Atoi(matches[1])
				m, _ := strconv.Atoi(matches[2])
				s, _ := strconv.Atoi(matches[3])
				ms, _ := strconv.Atoi(matches[4])
				current := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
					time.Duration(s)*time.Second + time.Duration(ms)*10*time.Millisecond
				progress := float64(current) / float64(totalDuration)
				if progress > 1.0 {
					progress = 1.0
				}
				progressCb(progress)
			}
		}
	}

	return cmd.Wait()
}
