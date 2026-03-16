package download

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/dlrelay/internal/model"
)

// Rule maps a domain to a specific download directory.
type Rule struct {
	Domain string // e.g. "youtube.com" (matches subdomains too)
	Dir    string // e.g. "/downloads/youtube"
}

// Manager manages concurrent download tasks.
type Manager struct {
	tasks        sync.Map
	sem          chan struct{}
	downloadDir  string
	tempDir      string
	rules        []Rule
	checkDirs    []string
	store        *Store
	persistTimer *time.Timer
	persistMu    sync.Mutex
}

// NewManager creates a new download manager.
func NewManager(downloadDir, tempDir string, maxConcurrent int, rules []Rule, checkDirs []string) *Manager {
	m := &Manager{
		sem:         make(chan struct{}, maxConcurrent),
		downloadDir: downloadDir,
		tempDir:     tempDir,
		rules:       rules,
		checkDirs:   checkDirs,
		store:       NewStore(downloadDir),
	}
	m.loadAndResume()
	return m
}

// resolveDownloadDir returns the download directory for the given URL,
// checking domain-based rules. Falls back to the default download directory.
func (m *Manager) resolveDownloadDir(rawURL string) string {
	host := extractDomain(rawURL)
	if host == "" {
		return m.downloadDir
	}
	host = strings.ToLower(host)
	for _, rule := range m.rules {
		if host == rule.Domain || strings.HasSuffix(host, "."+rule.Domain) {
			return rule.Dir
		}
	}
	return m.downloadDir
}

// extractDomain extracts the hostname from a URL (without port).
func extractDomain(rawURL string) string {
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

// loadAndResume loads persisted download records and resumes incomplete ones.
func (m *Manager) loadAndResume() {
	records, err := m.store.Load()
	if err != nil {
		slog.Error("failed to load download store", "err", err)
		return
	}

	for _, rec := range records {
		switch rec.State {
		case model.StateCompleted, model.StateFailed, model.StateCancelled, model.StateSkipped:
			// Restore finished tasks as-is (no goroutine needed)
			task := &Task{
				id:       rec.ID,
				url:      rec.Request.URL,
				req:      rec.Request,
				state:    rec.State,
				bytes:    rec.Bytes,
				total:    rec.Total,
				err:      rec.Error,
				filePath: rec.FilePath,
				skipInfo: rec.SkipInfo,
				cancel:   func() {}, // no-op for finished tasks
			}
			m.tasks.Store(rec.ID, task)

		case model.StateQueued, model.StateDownloading:
			// Re-submit incomplete downloads
			slog.Info("resuming download", "id", rec.ID, "url", rec.Request.URL)
			ctx, cancel := context.WithCancel(context.Background())
			task := NewTask(rec.ID, rec.Request, cancel)
			task.onChange = func() { m.schedulePersist() }
			m.tasks.Store(rec.ID, task)

			go m.executeDownload(ctx, task, rec.Request)
		}
	}

	if len(records) > 0 {
		slog.Info("loaded download records", "count", len(records))
	}
}

// resolveFilename determines the expected filename for a download request
// without actually downloading. For yt-dlp, this calls yt-dlp --print filename,
// falling back to req.Filename or FallbackURL if that fails.
func (m *Manager) resolveFilename(ctx context.Context, req model.DownloadRequest) string {
	if req.Method == "ytdlp" {
		printCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		name, err := YtdlpFilename(printCtx, req)
		if err != nil {
			slog.Warn("failed to resolve yt-dlp filename for skip check, falling back to request filename", "err", err)
			// Fall back to req.Filename (extension-provided) or FallbackURL basename
			return filenameFromRequest(req)
		}
		return filepath.Base(name)
	}
	return filenameFromRequest(req)
}

// filenameFromRequest extracts the expected filename from a download request
// using the Filename field, falling back to FallbackURL or URL basename.
func filenameFromRequest(req model.DownloadRequest) string {
	filename := filepath.Base(req.Filename)
	if filename != "" && filename != "." && filename != "/" {
		return filename
	}
	if req.FallbackURL != "" {
		filename = filepath.Base(req.FallbackURL)
		if filename != "" && filename != "." && filename != "/" {
			return filename
		}
	}
	filename = filepath.Base(req.URL)
	if filename != "" && filename != "." && filename != "/" {
		return filename
	}
	return ""
}

// searchDirs returns all directories to check for existing files.
func (m *Manager) searchDirs(dlDir string) []string {
	seen := map[string]bool{dlDir: true}
	dirs := []string{dlDir}
	for _, r := range m.rules {
		if !seen[r.Dir] {
			seen[r.Dir] = true
			dirs = append(dirs, r.Dir)
		}
	}
	for _, d := range m.checkDirs {
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// executeDownload runs the appropriate download handler for the request.
func (m *Manager) executeDownload(ctx context.Context, task *Task, req model.DownloadRequest) {
	if ctx.Err() != nil {
		return
	}

	dlDir := m.resolveDownloadDir(req.URL)

	// Skip-if-exists check (before acquiring semaphore to avoid blocking other downloads)
	if !task.forceDownload {
		filename := m.resolveFilename(ctx, req)
		if filename != "" {
			if existingPath := FindExistingFile(filename, m.searchDirs(dlDir)); existingPath != "" {
				slog.Info("file already exists, skipping download",
					"filename", filename, "existing", existingPath)
				task.SetFilePath(existingPath)
				task.SetSkipInfo(existingPath)
				task.mu.Lock()
				task.req.Filename = filepath.Base(existingPath)
				task.mu.Unlock()
				task.SetState(model.StateSkipped)
				return
			}
		}
	}

	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	if ctx.Err() != nil {
		return
	}

	var err error
	switch {
	case req.Method == "ytdlp":
		err = YtdlpDownload(ctx, task, dlDir)
		// Fallback: if yt-dlp fails and a fallback URL is available, retry with it
		if err != nil && ctx.Err() == nil && req.FallbackURL != "" {
			slog.Info("yt-dlp failed, trying fallback URL", "url", req.FallbackURL, "ytdlp_err", err)
			task.ResetForRetry(req.FallbackURL)
			err = m.downloadByURL(ctx, task, req.FallbackURL, dlDir)
		}
	case req.AudioURL != "":
		err = DASHDownload(ctx, task, dlDir, m.tempDir)
	default:
		err = m.downloadByURL(ctx, task, req.URL, dlDir)
	}

	if err != nil && ctx.Err() == nil {
		task.SetError(err.Error())
	}
}

// downloadByURL picks the right downloader (HLS or HTTP) based on the URL.
func (m *Manager) downloadByURL(ctx context.Context, task *Task, url string, dlDir string) error {
	urlLower := strings.ToLower(url)
	if strings.Contains(urlLower, ".m3u8") || strings.Contains(urlLower, "m3u8") {
		return HLSDownload(ctx, task, dlDir, m.tempDir)
	}
	return HTTPDownload(ctx, task, dlDir, m.tempDir)
}

// schedulePersist debounces persist calls, writing at most once per second.
func (m *Manager) schedulePersist() {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	if m.persistTimer != nil {
		m.persistTimer.Stop()
	}
	m.persistTimer = time.AfterFunc(time.Second, func() {
		m.persist()
	})
}

// persist saves the current state of all tasks to disk.
func (m *Manager) persist() {
	var records []Record
	m.tasks.Range(func(_, v any) bool {
		t := v.(*Task)
		t.mu.RLock()
		// Strip sensitive headers (cookies, auth) from persisted request
		req := t.req
		if len(req.Headers) > 0 {
			safe := make(map[string]string, len(req.Headers))
			for k, v := range req.Headers {
				lower := strings.ToLower(k)
				if lower == "cookie" || lower == "authorization" {
					continue
				}
				safe[k] = v
			}
			req.Headers = safe
		}
		rec := Record{
			ID:       t.id,
			Request:  req,
			State:    t.state,
			FilePath: t.filePath,
			Error:    t.err,
			SkipInfo: t.skipInfo,
			Bytes:    t.bytes,
			Total:    t.total,
		}
		t.mu.RUnlock()
		records = append(records, rec)
		return true
	})
	m.store.Save(records)
}

// Submit creates and starts a new download task.
func (m *Manager) Submit(req model.DownloadRequest) (string, error) {
	if err := ValidateDownloadURL(req.URL); err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if req.AudioURL != "" {
		if err := ValidateDownloadURL(req.AudioURL); err != nil {
			return "", fmt.Errorf("invalid audio URL: %w", err)
		}
	}

	id := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	task := NewTask(id, req, cancel)
	task.onChange = func() { m.schedulePersist() }
	m.tasks.Store(id, task)
	m.persist()

	go m.executeDownload(ctx, task, req)

	return id, nil
}

// Get returns the status of a download task.
func (m *Manager) Get(id string) (model.DownloadStatus, error) {
	v, ok := m.tasks.Load(id)
	if !ok {
		return model.DownloadStatus{}, fmt.Errorf("task %s not found", id)
	}
	return v.(*Task).Status(), nil
}

// List returns the status of all download tasks.
func (m *Manager) List() []model.DownloadStatus {
	var result []model.DownloadStatus
	m.tasks.Range(func(_, v any) bool {
		result = append(result, v.(*Task).Status())
		return true
	})
	return result
}

// Cancel cancels a download task.
func (m *Manager) Cancel(id string) error {
	v, ok := m.tasks.Load(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	v.(*Task).Cancel()
	return nil
}

// Delete cancels and removes a download task.
func (m *Manager) Delete(id string) error {
	v, ok := m.tasks.LoadAndDelete(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	v.(*Task).Cancel()
	m.persist()
	return nil
}

// Retry re-submits a failed or cancelled download task.
func (m *Manager) Retry(id string) error {
	v, ok := m.tasks.Load(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	old := v.(*Task)
	st := old.Status()
	if st.State != model.StateFailed && st.State != model.StateCancelled && st.State != model.StateSkipped {
		return fmt.Errorf("task %s is not in a retryable state (%s)", id, st.State)
	}

	// Remove old task and create a new one with the same request
	wasSkipped := st.State == model.StateSkipped
	m.tasks.Delete(id)
	ctx, cancel := context.WithCancel(context.Background())
	task := NewTask(id, old.req, cancel)
	task.forceDownload = wasSkipped // bypass skip check when retrying a skipped task
	task.onChange = func() { m.schedulePersist() }
	m.tasks.Store(id, task)
	m.persist()

	go m.executeDownload(ctx, task, old.req)
	return nil
}

// DownloadDir returns the default download directory path.
func (m *Manager) DownloadDir() string {
	return m.downloadDir
}

// DownloadDirs returns all download directories (default + rules + check dirs).
func (m *Manager) DownloadDirs() []string {
	dirs := []string{m.downloadDir}
	for _, r := range m.rules {
		dirs = append(dirs, r.Dir)
	}
	dirs = append(dirs, m.checkDirs...)
	return dirs
}
