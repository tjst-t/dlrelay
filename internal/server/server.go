package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/tjst-t/dlrelay/internal/convert"
	"github.com/tjst-t/dlrelay/internal/download"
	"github.com/tjst-t/dlrelay/internal/model"
	"github.com/tjst-t/dlrelay/internal/version"
)

// Server is the dlrelay HTTP API server.
type Server struct {
	router       chi.Router
	dlMgr        *download.Manager
	convMgr      *convert.Manager
	extensionDir string
	apiKey       string
}

// Option is a functional option for Server.
type Option func(*Server)

// WithExtensionDir sets the directory containing the browser extension source.
func WithExtensionDir(dir string) Option {
	return func(s *Server) { s.extensionDir = dir }
}

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(s *Server) { s.apiKey = key }
}

// New creates a new Server.
func New(dlMgr *download.Manager, convMgr *convert.Manager, opts ...Option) *Server {
	s := &Server{
		router:  chi.NewRouter(),
		dlMgr:   dlMgr,
		convMgr: convMgr,
	}
	for _, o := range opts {
		o(s)
	}
	s.routes()
	return s
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow any origin (API key auth is the security boundary, not CORS).
		// This enables bookmarklets and other non-extension clients.
		w.Header().Add("Vary", "Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(key), []byte(s.apiKey)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Logger)
	s.router.Use(s.cors)
	s.router.Use(jsonContentType)

	// HTML pages (no JSON content-type)
	s.router.Group(func(r chi.Router) {
		r.Get("/", s.handleStatusPage)
		r.Get("/setup", s.handlePage)
		r.Get("/bookmarklet", s.handleBookmarkletPage)
	})

	s.router.Route("/api", func(r chi.Router) {
		// Public endpoints (no auth required)
		r.Get("/health", s.handleHealth)
		r.Get("/extension.zip", s.handleExtensionZip)
		r.Get("/codecs", s.handleCodecs)
		r.Get("/formats", s.handleFormats)
		r.Get("/downloads", s.handleListDownloads)
		r.Get("/downloads/{id}", s.handleGetDownload)
		r.Get("/downloads/{id}/file", s.handleDownloadFile)
		r.Get("/convert/{id}", s.handleGetConvert)

		// Protected endpoints (require API key when configured)
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Post("/downloads", s.handleCreateDownload)
			r.Post("/downloads/{id}/retry", s.handleRetryDownload)
			r.Delete("/downloads/{id}", s.handleDeleteDownload)
			r.Post("/convert", s.handleCreateConvert)
			r.Delete("/convert/{id}", s.handleDeleteConvert)
			r.Post("/probe", s.handleProbe)
		})
	})
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, model.ErrorResponse{Error: msg})
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

// handleCreateDownload creates a new download task.
func (s *Server) handleCreateDownload(w http.ResponseWriter, r *http.Request) {
	var req model.DownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	id, err := s.dlMgr.Submit(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	status, _ := s.dlMgr.Get(id)
	writeJSON(w, http.StatusAccepted, status)
}

// handleListDownloads returns all download tasks.
func (s *Server) handleListDownloads(w http.ResponseWriter, r *http.Request) {
	list := s.dlMgr.List()
	if list == nil {
		list = []model.DownloadStatus{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetDownload returns a single download task.
func (s *Server) handleGetDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := s.dlMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleDownloadFile serves the downloaded file for preview/download.
func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := s.dlMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if (status.State != model.StateCompleted && status.State != model.StateSkipped) || status.FilePath == "" {
		writeError(w, http.StatusNotFound, "file not available")
		return
	}

	// Validate the file path is under one of the download directories
	absPath, err := filepath.Abs(status.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid file path")
		return
	}
	allowed := false
	for _, dir := range s.dlMgr.DownloadDirs() {
		dlDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath+string(filepath.Separator), dlDir+string(filepath.Separator)) || absPath == dlDir {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "file outside download directory")
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found on disk")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stat file")
		return
	}

	// Set appropriate content type
	ext := strings.ToLower(filepath.Ext(absPath))
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	// Use RFC 5987 encoding for safe Content-Disposition filename
	filename := filepath.Base(absPath)
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(filename))

	http.ServeContent(w, r, filename, stat.ModTime(), f)
}

// handleRetryDownload retries a failed or cancelled download.
func (s *Server) handleRetryDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.dlMgr.Retry(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := s.dlMgr.Get(id)
	writeJSON(w, http.StatusAccepted, status)
}

// handleDeleteDownload cancels and deletes a download task.
func (s *Server) handleDeleteDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.dlMgr.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCreateConvert creates a new conversion task.
func (s *Server) handleCreateConvert(w http.ResponseWriter, r *http.Request) {
	var req model.ConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := s.convMgr.Submit(req)
	if err != nil {
		slog.Error("failed to submit convert", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	status, _ := s.convMgr.Get(id)
	writeJSON(w, http.StatusAccepted, status)
}

// handleGetConvert returns a single conversion task.
func (s *Server) handleGetConvert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	status, err := s.convMgr.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleDeleteConvert cancels a conversion task.
func (s *Server) handleDeleteConvert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.convMgr.Cancel(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleProbe runs ffprobe on the given URL.
func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	var req model.ProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := convert.Probe(r.Context(), req.URL, req.Headers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCodecs returns available FFmpeg codecs.
func (s *Server) handleCodecs(w http.ResponseWriter, r *http.Request) {
	codecs, err := convert.ListCodecs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, codecs)
}

// handleFormats returns available FFmpeg formats.
func (s *Server) handleFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := convert.ListFormats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, formats)
}
