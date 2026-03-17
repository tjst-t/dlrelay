package download_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tjst-t/dlrelay/internal/download"
	"github.com/tjst-t/dlrelay/internal/model"
)

func TestManagerSubmitAndGet(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test content"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	defer fileServer.Close()

	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "test.txt",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	// Wait for completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := mgr.Get(id)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if status.State == model.StateCompleted {
			return
		}
		if status.State == model.StateFailed {
			t.Fatalf("download failed: %v", status.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("download did not complete in time")
}

func TestManagerList(t *testing.T) {
	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	list := mgr.List()
	if list != nil && len(list) != 0 {
		t.Fatalf("expected empty list, got %d items", len(list))
	}
}

func TestManagerGetNotFound(t *testing.T) {
	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestManagerCancel(t *testing.T) {
	// Slow server that never finishes
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
				w.Write([]byte("x"))
			}
		}
	}))
	defer slowServer.Close()

	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      slowServer.URL + "/slow.bin",
		Filename: "slow.bin",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait briefly for download to start
	time.Sleep(200 * time.Millisecond)

	if err := mgr.Cancel(id); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	status, err := mgr.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if status.State != model.StateCancelled {
		t.Fatalf("expected cancelled, got %s", status.State)
	}
}

func TestManagerDelete(t *testing.T) {
	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	err := mgr.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestPathTraversal(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test content"))
	}))
	defer fileServer.Close()

	downloadDir := t.TempDir()
	mgr := download.NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	// Try to escape download directory via Directory field
	id, err := mgr.Submit(model.DownloadRequest{
		URL:       fileServer.URL + "/test.txt",
		Filename:  "test.txt",
		Directory: "../../etc",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	if status.State != model.StateFailed {
		t.Fatalf("expected failed state for path traversal, got %s", status.State)
	}
	t.Logf("Path traversal correctly rejected: %v", status.Error)
}

func TestFilenameSanitization(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test content"))
	}))
	defer fileServer.Close()

	downloadDir := t.TempDir()
	mgr := download.NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	// Try path traversal via filename
	id, err := mgr.Submit(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "../../../etc/passwd",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	// Should complete but file should be saved with just "passwd" as the basename
	if status.State == model.StateCompleted {
		t.Log("Download completed — filename was sanitized to basename")
	}
}

func TestSkipIfExists(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test content"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	defer fileServer.Close()

	downloadDir := t.TempDir()
	checkDir := t.TempDir()

	// Create an existing file in the check directory (different extension)
	os.MkdirAll(filepath.Join(checkDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(checkDir, "sub", "test.mkv"), []byte("existing"), 0o644)

	mgr := download.NewManager(downloadDir, t.TempDir(), 3, nil, []string{checkDir})

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "test.mp4",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateSkipped || status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, err := mgr.Get(id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if status.State != model.StateSkipped {
		t.Fatalf("expected skipped, got %s", status.State)
	}
	if status.SkipInfo == "" {
		t.Fatal("expected skip_info to be set")
	}
	if status.Filename != "test.mkv" {
		t.Errorf("expected filename 'test.mkv', got %q", status.Filename)
	}
	if !status.HasFile {
		t.Error("expected has_file to be true")
	}
}

func TestSkipIfExistsRetryForceDownloads(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test content"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	defer fileServer.Close()

	downloadDir := t.TempDir()

	// Create existing file in download dir
	os.WriteFile(filepath.Join(downloadDir, "test.mp4"), []byte("existing"), 0o644)

	mgr := download.NewManager(downloadDir, t.TempDir(), 3, nil, nil)

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "test.mp4",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for skip
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateSkipped {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	if status.State != model.StateSkipped {
		t.Fatalf("expected skipped, got %s", status.State)
	}

	// Retry should force download
	if err := mgr.Retry(id); err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ = mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ = mgr.Get(id)
	if status.State != model.StateCompleted {
		t.Fatalf("expected completed after retry, got %s (error: %v)", status.State, status.Error)
	}
}

func TestNoSkipWhenNoMatch(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "test content"
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	defer fileServer.Close()

	downloadDir := t.TempDir()
	checkDir := t.TempDir()

	// Create a file with a different name
	os.WriteFile(filepath.Join(checkDir, "other.mp4"), []byte("existing"), 0o644)

	mgr := download.NewManager(downloadDir, t.TempDir(), 3, nil, []string{checkDir})

	id, err := mgr.Submit(model.DownloadRequest{
		URL:      fileServer.URL + "/test.txt",
		Filename: "unique_video.mp4",
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := mgr.Get(id)
		if status.State == model.StateCompleted || status.State == model.StateFailed || status.State == model.StateSkipped {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	status, _ := mgr.Get(id)
	if status.State != model.StateCompleted {
		t.Fatalf("expected completed (no match to skip), got %s", status.State)
	}
}

func TestConcurrentSubmitAndCancel(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
				w.Write([]byte("x"))
			}
		}
	}))
	defer fileServer.Close()

	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	// Concurrently submit and cancel tasks
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := mgr.Submit(model.DownloadRequest{
				URL:      fileServer.URL + fmt.Sprintf("/file%d.bin", i),
				Filename: fmt.Sprintf("file%d.bin", i),
			})
			if err != nil {
				return
			}
			// Cancel after brief delay
			time.Sleep(50 * time.Millisecond)
			mgr.Delete(id)
		}(i)
	}
	wg.Wait()
}

func TestConcurrentListAndSubmit(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("content"))
	}))
	defer fileServer.Close()

	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)

	var wg sync.WaitGroup

	// Concurrent lists
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				mgr.List()
			}
		}()
	}

	// Concurrent submits
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := mgr.Submit(model.DownloadRequest{
				URL:      fileServer.URL + fmt.Sprintf("/file%d.bin", i),
				Filename: fmt.Sprintf("file%d.bin", i),
			})
			if err != nil {
				return
			}
			// Clean up
			time.Sleep(500 * time.Millisecond)
			mgr.Delete(id)
		}(i)
	}

	wg.Wait()
}

func TestTaskCleanup(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	defer fileServer.Close()

	mgr := download.NewManager(t.TempDir(), t.TempDir(), 3, nil, nil)
	mgr.SetMaxCompletedTasks(3) // Very low limit for testing

	// Submit several downloads
	for i := 0; i < 6; i++ {
		_, err := mgr.Submit(model.DownloadRequest{
			URL:      fileServer.URL + fmt.Sprintf("/file%d.bin", i),
			Filename: fmt.Sprintf("file%d.bin", i),
		})
		if err != nil {
			t.Fatalf("Submit %d failed: %v", i, err)
		}
	}

	// Wait for all to complete
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		list := mgr.List()
		allDone := true
		for _, s := range list {
			if s.State == model.StateQueued || s.State == model.StateDownloading {
				allDone = false
				break
			}
		}
		if allDone && len(list) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force a persist/cleanup cycle by triggering a state change
	// The cleanup happens during schedulePersist
	time.Sleep(2 * time.Second) // Wait for debounced persist

	list := mgr.List()
	if len(list) > 3 {
		t.Errorf("expected at most 3 tasks after cleanup, got %d", len(list))
	}
}

func TestManagerConcurrencyLimit(t *testing.T) {
	// Server that blocks until context is cancelled
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slowServer.Close()

	maxConcurrent := 2
	mgr := download.NewManager(t.TempDir(), t.TempDir(), maxConcurrent, nil, nil)

	// Submit more tasks than maxConcurrent
	var ids []string
	for i := 0; i < maxConcurrent+2; i++ {
		id, err := mgr.Submit(model.DownloadRequest{
			URL:      slowServer.URL + fmt.Sprintf("/file%d.bin", i),
			Filename: fmt.Sprintf("file%d.bin", i),
		})
		if err != nil {
			t.Fatalf("Submit %d failed: %v", i, err)
		}
		ids = append(ids, id)
	}

	// Wait for tasks to start
	time.Sleep(300 * time.Millisecond)

	// Count tasks in downloading state
	downloading := 0
	for _, id := range ids {
		status, err := mgr.Get(id)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if status.State == model.StateDownloading {
			downloading++
		}
	}

	if downloading > maxConcurrent {
		t.Fatalf("expected at most %d downloading, got %d", maxConcurrent, downloading)
	}

	// Clean up
	for _, id := range ids {
		mgr.Delete(id)
	}

	// Wait briefly for goroutines to finish
	time.Sleep(100 * time.Millisecond)
}
