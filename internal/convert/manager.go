package convert

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/dlrelay/internal/model"
)

// Task represents a conversion task.
type Task struct {
	mu       sync.RWMutex
	id       string
	state    model.ConvertState
	progress float64
	err      string
	cancel   context.CancelFunc
}

// Status returns the current status of the task.
func (t *Task) Status() model.ConvertStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var errPtr *string
	if t.err != "" {
		errPtr = &t.err
	}
	return model.ConvertStatus{
		ID:       t.id,
		State:    t.state,
		Progress: t.progress,
		Error:    errPtr,
	}
}

// Manager manages conversion tasks.
type Manager struct {
	tasks sync.Map
}

// NewManager creates a new conversion manager.
func NewManager() *Manager {
	return &Manager{}
}

// Submit creates and starts a new conversion task.
func (m *Manager) Submit(req model.ConvertRequest) (string, error) {
	id := uuid.New().String()[:8]
	ctx, cancel := context.WithCancel(context.Background())
	task := &Task{
		id:     id,
		state:  model.ConvertStateRunning,
		cancel: cancel,
	}
	m.tasks.Store(id, task)

	go func() {
		err := RunConvert(ctx, req.Args, 0, func(p float64) {
			task.mu.Lock()
			task.progress = p
			task.mu.Unlock()
		})
		task.mu.Lock()
		defer task.mu.Unlock()
		if err != nil {
			if ctx.Err() != nil {
				task.state = model.ConvertStateCancelled
			} else {
				task.state = model.ConvertStateFailed
				task.err = err.Error()
			}
		} else {
			task.state = model.ConvertStateCompleted
			task.progress = 1.0
		}
	}()

	return id, nil
}

// Get returns the status of a conversion task.
func (m *Manager) Get(id string) (model.ConvertStatus, error) {
	v, ok := m.tasks.Load(id)
	if !ok {
		return model.ConvertStatus{}, fmt.Errorf("task %s not found", id)
	}
	return v.(*Task).Status(), nil
}

// Cancel cancels a conversion task.
func (m *Manager) Cancel(id string) error {
	v, ok := m.tasks.Load(id)
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t := v.(*Task)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = model.ConvertStateCancelled
	t.cancel()
	return nil
}

// RunConvertWithDuration is a convenience wrapper that probes first to get duration.
func RunConvertWithDuration(ctx context.Context, args []string, progressCb func(float64)) error {
	return RunConvert(ctx, args, time.Duration(0), progressCb)
}
