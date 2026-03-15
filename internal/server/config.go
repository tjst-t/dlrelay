package server

import (
	"os"
	"strconv"
	"strings"

	"github.com/tjst-t/dlrelay/internal/download"
)

// Config holds server configuration from environment variables.
type Config struct {
	ListenAddr    string
	DownloadDir   string
	TempDir       string
	MaxConcurrent int
	ExtensionDir  string
	APIKey        string
	DownloadRules []download.Rule
}

// LoadConfig reads configuration from environment variables with defaults.
func LoadConfig() Config {
	c := Config{
		ListenAddr:    ":8090",
		DownloadDir:   "/downloads",
		TempDir:       os.TempDir(),
		MaxConcurrent: 3,
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	}
	if v := os.Getenv("DOWNLOAD_DIR"); v != "" {
		c.DownloadDir = v
	}
	if v := os.Getenv("TEMP_DIR"); v != "" {
		c.TempDir = v
	}
	if v := os.Getenv("MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxConcurrent = n
		}
	}
	if v := os.Getenv("EXTENSION_DIR"); v != "" {
		c.ExtensionDir = v
	}
	if v := os.Getenv("API_KEY"); v != "" {
		c.APIKey = v
	}
	// DOWNLOAD_RULES format: "domain1:/path1,domain2:/path2"
	if v := os.Getenv("DOWNLOAD_RULES"); v != "" {
		c.DownloadRules = parseDownloadRules(v)
	}
	return c
}

// parseDownloadRules parses "domain:/path,domain:/path" format.
func parseDownloadRules(s string) []download.Rule {
	var rules []download.Rule
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		domain, dir, ok := strings.Cut(entry, ":")
		if !ok || domain == "" || dir == "" {
			continue
		}
		rules = append(rules, download.Rule{
			Domain: strings.ToLower(strings.TrimSpace(domain)),
			Dir:    strings.TrimSpace(dir),
		})
	}
	return rules
}
