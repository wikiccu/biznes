package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wikiccu/biznes/internal/platform/config"
	"github.com/wikiccu/biznes/internal/platform/database"
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
	pool, err := database.Open(ctx, cfg)
	if err != nil {
		if ctx.Err() != nil {
			logger.Info("application startup canceled")
			return
		}
		logger.Error("PostgreSQL startup failed", "error", err)
		os.Exit(1)
	}
	logger.Info("PostgreSQL pool opened", "max_conns", pool.Config().MaxConns)
	server := httpserver.New(ctx, cfg, logger, pool)
	err = httpserver.Run(ctx, server, cfg.HTTPShutdownTimeout, logger)
	pool.Close()
	logger.Info("PostgreSQL pool closed")
	if err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
