package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tjst-t/dlrelay/internal/convert"
	"github.com/tjst-t/dlrelay/internal/download"
	"github.com/tjst-t/dlrelay/internal/server"
)

func main() {
	cfg := server.LoadConfig()

	if err := os.MkdirAll(cfg.DownloadDir, 0o755); err != nil {
		slog.Error("failed to create download directory", "error", err)
		os.Exit(1)
	}
	for _, rule := range cfg.DownloadRules {
		if err := os.MkdirAll(rule.Dir, 0o755); err != nil {
			slog.Error("failed to create rule download directory", "domain", rule.Domain, "dir", rule.Dir, "error", err)
			os.Exit(1)
		}
	}

	dlMgr := download.NewManager(cfg.DownloadDir, cfg.TempDir, cfg.MaxConcurrent, cfg.DownloadRules, cfg.CheckDirs)
	convMgr := convert.NewManager()

	var opts []server.Option
	if cfg.ExtensionDir != "" {
		opts = append(opts, server.WithExtensionDir(cfg.ExtensionDir))
	}
	if cfg.APIKey != "" {
		opts = append(opts, server.WithAPIKey(cfg.APIKey))
	}

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: server.New(dlMgr, convMgr, opts...),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("server starting", "addr", cfg.ListenAddr, "download_dir", cfg.DownloadDir, "download_rules", len(cfg.DownloadRules))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
