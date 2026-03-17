package server

import (
	"net/http"
	"strings"

	"github.com/tjst-t/dlrelay/internal/version"
)

func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	base := safeServerURL(r)
	html := strings.ReplaceAll(loadTemplate("status.html"), "{{SERVER_URL}}", base)
	html = strings.ReplaceAll(html, "{{VERSION}}", version.Version)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
