package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFromTOML(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "config.toml")

	content := `
listen_addr = ":9090"
download_dir = "/data/downloads"
temp_dir = "/data/tmp"
max_concurrent = 5
extension_dir = "/ext"
api_key = "secret123"
check_dirs = ["/media/videos", "/media/archive"]

[[download_rules]]
domain = "youtube.com"
dir = "/data/youtube"

[[download_rules]]
domain = "twitter.com"
dir = "/data/twitter"
`
	os.WriteFile(configFile, []byte(content), 0o644)
	t.Setenv("CONFIG_FILE", configFile)

	// Clear any env vars that would override
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("DOWNLOAD_DIR", "")
	t.Setenv("TEMP_DIR", "")
	t.Setenv("MAX_CONCURRENT", "")
	t.Setenv("EXTENSION_DIR", "")
	t.Setenv("API_KEY", "")
	t.Setenv("DOWNLOAD_RULES", "")
	t.Setenv("CHECK_DIRS", "")

	cfg := LoadConfig()

	if cfg.ListenAddr != ":9090" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, ":9090")
	}
	if cfg.DownloadDir != "/data/downloads" {
		t.Errorf("DownloadDir = %q, want %q", cfg.DownloadDir, "/data/downloads")
	}
	if cfg.TempDir != "/data/tmp" {
		t.Errorf("TempDir = %q, want %q", cfg.TempDir, "/data/tmp")
	}
	if cfg.MaxConcurrent != 5 {
		t.Errorf("MaxConcurrent = %d, want %d", cfg.MaxConcurrent, 5)
	}
	if cfg.ExtensionDir != "/ext" {
		t.Errorf("ExtensionDir = %q, want %q", cfg.ExtensionDir, "/ext")
	}
	if cfg.APIKey != "secret123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "secret123")
	}
	if len(cfg.CheckDirs) != 2 || cfg.CheckDirs[0] != "/media/videos" || cfg.CheckDirs[1] != "/media/archive" {
		t.Errorf("CheckDirs = %v, want [/media/videos /media/archive]", cfg.CheckDirs)
	}
	if len(cfg.DownloadRules) != 2 {
		t.Fatalf("DownloadRules len = %d, want 2", len(cfg.DownloadRules))
	}
	if cfg.DownloadRules[0].Domain != "youtube.com" || cfg.DownloadRules[0].Dir != "/data/youtube" {
		t.Errorf("DownloadRules[0] = %+v", cfg.DownloadRules[0])
	}
}

func TestEnvOverridesToml(t *testing.T) {
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "config.toml")

	content := `
listen_addr = ":9090"
download_dir = "/data/downloads"
max_concurrent = 5
`
	os.WriteFile(configFile, []byte(content), 0o644)
	t.Setenv("CONFIG_FILE", configFile)

	// Set env overrides
	t.Setenv("LISTEN_ADDR", ":7070")
	t.Setenv("MAX_CONCURRENT", "10")
	// Clear others
	t.Setenv("DOWNLOAD_DIR", "")
	t.Setenv("TEMP_DIR", "")
	t.Setenv("EXTENSION_DIR", "")
	t.Setenv("API_KEY", "")
	t.Setenv("DOWNLOAD_RULES", "")
	t.Setenv("CHECK_DIRS", "")

	cfg := LoadConfig()

	if cfg.ListenAddr != ":7070" {
		t.Errorf("ListenAddr = %q, want %q (env should override)", cfg.ListenAddr, ":7070")
	}
	if cfg.MaxConcurrent != 10 {
		t.Errorf("MaxConcurrent = %d, want %d (env should override)", cfg.MaxConcurrent, 10)
	}
	if cfg.DownloadDir != "/data/downloads" {
		t.Errorf("DownloadDir = %q, want %q (should come from TOML)", cfg.DownloadDir, "/data/downloads")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	t.Setenv("CONFIG_FILE", "/nonexistent/config.toml")
	// Clear all env vars
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("DOWNLOAD_DIR", "")
	t.Setenv("TEMP_DIR", "")
	t.Setenv("MAX_CONCURRENT", "")
	t.Setenv("EXTENSION_DIR", "")
	t.Setenv("API_KEY", "")
	t.Setenv("DOWNLOAD_RULES", "")
	t.Setenv("CHECK_DIRS", "")

	cfg := LoadConfig()

	// Should fall back to defaults
	if cfg.ListenAddr != ":8090" {
		t.Errorf("ListenAddr = %q, want default %q", cfg.ListenAddr, ":8090")
	}
	if cfg.MaxConcurrent != 3 {
		t.Errorf("MaxConcurrent = %d, want default %d", cfg.MaxConcurrent, 3)
	}
}
