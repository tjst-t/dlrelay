package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/tjst-t/dlrelay/internal/download"
)

// expandHome replaces a leading "~" or "~/" with the user's home directory.
func expandHome(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// Config holds server configuration.
type Config struct {
	ListenAddr    string
	DownloadDir   string
	TempDir       string
	MaxConcurrent int
	ExtensionDir  string
	APIKey        string
	DownloadRules []download.Rule
	CheckDirs     []string
}

// tomlConfig is the TOML file representation.
type tomlConfig struct {
	ListenAddr    string            `toml:"listen_addr"`
	DownloadDir   string            `toml:"download_dir"`
	TempDir       string            `toml:"temp_dir"`
	MaxConcurrent int               `toml:"max_concurrent"`
	ExtensionDir  string            `toml:"extension_dir"`
	APIKey        string            `toml:"api_key"`
	CheckDirs     []string          `toml:"check_dirs"`
	DownloadRules []tomlDownloadRule `toml:"download_rules"`
}

type tomlDownloadRule struct {
	Domain string `toml:"domain"`
	Dir    string `toml:"dir"`
}

// LoadConfig reads configuration from a TOML config file (if present),
// then applies environment variable overrides. Env vars always take precedence.
func LoadConfig() Config {
	c := Config{
		ListenAddr:    ":8090",
		DownloadDir:   "/downloads",
		TempDir:       os.TempDir(),
		MaxConcurrent: 3,
	}

	// Load TOML config file if available
	configFile := "/etc/dlrelay/config.toml"
	if v := os.Getenv("CONFIG_FILE"); v != "" {
		configFile = v
	}
	loadConfigFile(&c, configFile)

	// Environment variable overrides
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
	// CHECK_DIRS format: "/path1,/path2" — directories to check for existing files
	if v := os.Getenv("CHECK_DIRS"); v != "" {
		c.CheckDirs = nil
		for _, d := range strings.Split(v, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				c.CheckDirs = append(c.CheckDirs, d)
			}
		}
	}

	// Expand ~ in all path fields
	c.DownloadDir = expandHome(c.DownloadDir)
	c.TempDir = expandHome(c.TempDir)
	c.ExtensionDir = expandHome(c.ExtensionDir)
	for i := range c.CheckDirs {
		c.CheckDirs[i] = expandHome(c.CheckDirs[i])
	}
	for i := range c.DownloadRules {
		c.DownloadRules[i].Dir = expandHome(c.DownloadRules[i].Dir)
	}

	return c
}

// loadConfigFile reads a TOML config file and applies values to c.
func loadConfigFile(c *Config, path string) {
	var tc tomlConfig
	if _, err := toml.DecodeFile(path, &tc); err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read config file", "path", path, "err", err)
		}
		return
	}
	slog.Info("loaded config file", "path", path)

	if tc.ListenAddr != "" {
		c.ListenAddr = tc.ListenAddr
	}
	if tc.DownloadDir != "" {
		c.DownloadDir = tc.DownloadDir
	}
	if tc.TempDir != "" {
		c.TempDir = tc.TempDir
	}
	if tc.MaxConcurrent > 0 {
		c.MaxConcurrent = tc.MaxConcurrent
	}
	if tc.ExtensionDir != "" {
		c.ExtensionDir = tc.ExtensionDir
	}
	if tc.APIKey != "" {
		c.APIKey = tc.APIKey
	}
	if len(tc.CheckDirs) > 0 {
		c.CheckDirs = tc.CheckDirs
	}
	if len(tc.DownloadRules) > 0 {
		c.DownloadRules = nil
		for _, r := range tc.DownloadRules {
			if r.Domain != "" && r.Dir != "" {
				c.DownloadRules = append(c.DownloadRules, download.Rule{
					Domain: strings.ToLower(strings.TrimSpace(r.Domain)),
					Dir:    strings.TrimSpace(r.Dir),
				})
			}
		}
	}
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
