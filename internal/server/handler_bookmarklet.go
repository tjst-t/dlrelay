package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tjst-t/dlrelay/internal/version"
)

// safeAPIKey returns the API key safely escaped for embedding in JS strings.
func safeAPIKey(key string) string {
	b, _ := json.Marshal(key)
	return string(b[1 : len(b)-1])
}

func (s *Server) handleBookmarkletPage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	key := safeAPIKey(s.apiKey)
	html := strings.ReplaceAll(loadTemplate("bookmarklet.html"), "{{SERVER_URL}}", base)
	html = strings.ReplaceAll(html, "{{API_KEY}}", key)
	html = strings.ReplaceAll(html, "{{VERSION}}", version.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
