package download

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tjst-t/dlrelay/internal/model"
)

// Record is a persistent download record saved to disk.
type Record struct {
	ID        string                `json:"id"`
	Request   model.DownloadRequest `json:"request"`
	State     model.DownloadState   `json:"state"`
	FilePath  string                `json:"file_path,omitempty"`
	Error     string                `json:"error,omitempty"`
	SkipInfo  string                `json:"skip_info,omitempty"`
	Bytes     int64                 `json:"bytes_received"`
	Total     int64                 `json:"total_bytes"`
	CreatedAt time.Time             `json:"created_at,omitempty"`
	TempPath  string                `json:"temp_path,omitempty"`
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

// Save writes all records to disk atomically with fsync and backup.
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

	// Backup existing file before overwriting
	if _, err := os.Stat(s.path); err == nil {
		bakPath := s.path + ".bak"
		if copyErr := copyFileSimple(s.path, bakPath); copyErr != nil {
			slog.Warn("failed to create store backup", "err", copyErr)
		}
	}

	tmpPath := s.path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Error("failed to create temp store file", "err", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		slog.Error("failed to write download store", "err", err)
		return
	}
	// Fsync to ensure data is flushed to disk
	if err := f.Sync(); err != nil {
		slog.Warn("failed to fsync download store", "err", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, s.path); err != nil {
		slog.Error("failed to rename download store", "err", err)
	}
}

func copyFileSimple(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
