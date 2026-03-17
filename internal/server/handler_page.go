package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tjst-t/dlrelay/internal/version"
)

func serverURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

// safeServerURL returns the server URL safely escaped for embedding in JS/HTML.
// Uses json.Marshal to escape all special characters (quotes, backslashes, angle brackets, etc.).
func safeServerURL(r *http.Request) string {
	raw := serverURL(r)
	b, _ := json.Marshal(raw)
	// json.Marshal returns `"value"` — strip outer quotes
	return string(b[1 : len(b)-1])
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	html := strings.ReplaceAll(loadTemplate("setup.html"), "{{SERVER_URL}}", base)
	html = strings.ReplaceAll(html, "{{VERSION}}", version.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
