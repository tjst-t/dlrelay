package server

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"

	dlrelay "github.com/tjst-t/dlrelay"
)

// handleExtensionZip serves the browser extension as a zip file
// with the server URL pre-configured so the user doesn't need to set it up manually.
//
// Source preference:
//  1. If extensionDir is configured and exists on disk, read from there (dev override).
//  2. Otherwise fall back to the extension files embedded into the binary.
func (s *Server) handleExtensionZip(w http.ResponseWriter, r *http.Request) {
	extFS, err := s.extensionFS()
	if err != nil {
		slog.Error("extension source unavailable", "error", err)
		writeError(w, http.StatusInternalServerError, "extension source unavailable")
		return
	}

	base := safeServerURL(r)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	err = fs.WalkDir(extFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".svg") {
			return nil
		}

		data, err := fs.ReadFile(extFS, path)
		if err != nil {
			return err
		}

		if path == "background.js" || path == "popup/popup.js" {
			data = []byte(strings.Replace(
				string(data),
				`serverUrl: "",`,
				`serverUrl: "`+base+`",`,
				1,
			))
		}

		f, err := zw.Create(path)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	})

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate extension zip")
		return
	}

	if err := zw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finalize extension zip")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=dlrelay-extension.zip")
	w.Write(buf.Bytes())
}

// extensionFS returns an fs.FS rooted at the extension source — the on-disk
// directory when configured and present, otherwise the embedded copy.
func (s *Server) extensionFS() (fs.FS, error) {
	if s.extensionDir != "" {
		if _, err := os.Stat(s.extensionDir); err == nil {
			return os.DirFS(s.extensionDir), nil
		} else {
			slog.Warn("configured extension_dir not accessible, falling back to embedded", "dir", s.extensionDir, "error", err)
		}
	}
	return fs.Sub(dlrelay.ExtensionFS, "extension")
}
