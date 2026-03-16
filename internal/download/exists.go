package download

import (
	"os"
	"path/filepath"
	"strings"
)

// FindExistingFile searches dirs recursively for a file whose base name
// (without extension) matches the given filename's base name (also without extension).
// Comparison is case-insensitive. Returns the full path of the first match, or "" if none found.
func FindExistingFile(filename string, dirs []string) string {
	target := strings.TrimSuffix(filename, filepath.Ext(filename))
	if target == "" {
		return ""
	}
	target = strings.ToLower(target)

	for _, dir := range dirs {
		var found string
		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			nameNoExt := strings.TrimSuffix(name, filepath.Ext(name))
			if strings.ToLower(nameNoExt) == target {
				found = path
				return filepath.SkipAll
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}
