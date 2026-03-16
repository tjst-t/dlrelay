package download

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/tjst-t/dlrelay/internal/model"
)

// Record is a persistent download record saved to disk.
type Record struct {
	ID       string                `json:"id"`
	Request  model.DownloadRequest `json:"request"`
	State    model.DownloadState   `json:"state"`
	FilePath string                `json:"file_path,omitempty"`
	Error    string                `json:"error,omitempty"`
	SkipInfo string                `json:"skip_info,omitempty"`
	Bytes    int64                 `json:"bytes_received"`
	Total    int64                 `json:"total_bytes"`
}

// Store persists download records to a JSON file.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore creates a new Store that saves to the given directory.
func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, ".dlrelay", "downloads.json")}
}

// Load reads all records from disk.
func (s *Store) Load() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var records []Record
	if err := json.Unmarshal(data, &records); err != nil {
		slog.Warn("failed to parse download store, starting fresh", "err", err)
		return nil, nil
	}
	return records, nil
}

// Save writes all records to disk atomically.
func (s *Store) Save(records []Record) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		slog.Error("failed to marshal download store", "err", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		slog.Error("failed to create store directory", "err", err)
		return
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		slog.Error("failed to write download store", "err", err)
		return
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		slog.Error("failed to rename download store", "err", err)
	}
}
