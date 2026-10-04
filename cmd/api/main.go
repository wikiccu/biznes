package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wikiccu/biznes/internal/platform/config"
	httpserver "github.com/wikiccu/biznes/internal/platform/http"
)

func main() {
	var level slog.LevelVar
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &level})).With("service", "biznes")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid application configuration", "error", err)
		os.Exit(1)
	}

	level.Set(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := httpserver.New(ctx, cfg, logger)
	if err := httpserver.Run(ctx, server, cfg.HTTPShutdownTimeout, logger); err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
