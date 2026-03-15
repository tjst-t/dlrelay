package server

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleExtensionZip serves the browser extension as a zip file
// with the server URL pre-configured so the user doesn't need to set it up manually.
func (s *Server) handleExtensionZip(w http.ResponseWriter, r *http.Request) {
	extDir := s.extensionDir
	if extDir == "" {
		writeError(w, http.StatusNotFound, "extension directory not configured")
		return
	}

	if _, err := os.Stat(extDir); err != nil {
		writeError(w, http.StatusNotFound, "extension directory not found")
		return
	}

	base := safeServerURL(r)

	// Buffer the zip in memory so we can return a proper error on failure
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	err := filepath.WalkDir(extDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Skip SVG source files and hidden files
		name := d.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".svg") {
			return nil
		}

		relPath, err := filepath.Rel(extDir, path)
		if err != nil {
			return err
		}
		// Zip spec requires forward slashes
		relPath = filepath.ToSlash(relPath)

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Inject server URL into background.js and popup.js defaults
		if relPath == "background.js" || relPath == "popup/popup.js" {
			data = []byte(strings.Replace(
				string(data),
				`serverUrl: "",`,
				`serverUrl: "`+base+`",`,
				1,
			))
		}

		f, err := zw.Create(relPath)
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
