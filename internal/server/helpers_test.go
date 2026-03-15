package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func startTestFileServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write([]byte(content))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func startSlowFileServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		for i := 0; i < 100; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
				w.Write([]byte("x"))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}
