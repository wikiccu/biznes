package main

import (
	"log/slog"
	"os"

	"github.com/wikiccu/biznes/internal/platform/config"
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
	logger.Info("application initialized", "http_port", cfg.HTTPPort)
}
