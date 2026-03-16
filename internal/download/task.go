package download

import (
	"context"
	"sync"

	"github.com/tjst-t/dlrelay/internal/model"
)

// Task represents a single download task.
type Task struct {
	mu       sync.RWMutex
	id       string
	url      string
	req      model.DownloadRequest
	state    model.DownloadState
	bytes    int64
	total    int64
	err      string
	filePath string
	cancel   context.CancelFunc
	onChange func()
}

// NewTask creates a new download task.
func NewTask(id string, req model.DownloadRequest, cancel context.CancelFunc) *Task {
	return &Task{
		id:     id,
		url:    req.URL,
		req:    req,
		state:  model.StateQueued,
		cancel: cancel,
	}
}

// SetState updates the task state.
func (t *Task) SetState(state model.DownloadState) {
	t.mu.Lock()
	t.state = state
	cb := t.onChange
	t.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// SetProgress updates the bytes received and total.
func (t *Task) SetProgress(received, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bytes = received
	t.total = total
}

// SetError sets the error message and marks the task as failed.
func (t *Task) SetError(err string) {
	t.mu.Lock()
	t.state = model.StateFailed
	t.err = err
	cb := t.onChange
	t.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// SetFilePath sets the final file path of the downloaded file.
func (t *Task) SetFilePath(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.filePath = path
}

// ResetForRetry resets the task state to retry with a different URL.
func (t *Task) ResetForRetry(newURL string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.url = newURL
	t.req.URL = newURL
	t.req.Method = ""
	t.req.FallbackURL = ""
	t.state = model.StateQueued
	t.bytes = 0
	t.total = 0
	t.err = ""
	t.filePath = ""
}

// Cancel cancels the download task.
func (t *Task) Cancel() {
	t.mu.Lock()
	t.state = model.StateCancelled
	cb := t.onChange
	t.mu.Unlock()
	t.cancel()
	if cb != nil {
		cb()
	}
}

// Status returns the current status of the task.
func (t *Task) Status() model.DownloadStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var errPtr *string
	if t.err != "" {
		errPtr = &t.err
	}
	return model.DownloadStatus{
		ID:            t.id,
		URL:           t.url,
		PageURL:       t.req.PageURL,
		State:         t.state,
		BytesReceived: t.bytes,
		TotalBytes:    t.total,
		Filename:      t.req.Filename,
		HasFile:       t.filePath != "",
		FilePath:      t.filePath,
		Error:         errPtr,
	}
}
