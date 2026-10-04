package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort int
	LogLevel slog.Level
}

func Load() (Config, error) {
	cfg := Config{HTTPPort: 8080, LogLevel: slog.LevelInfo}

	if value, exists := os.LookupEnv("BIZNES_HTTP_PORT"); exists {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New("BIZNES_HTTP_PORT must be an integer between 1 and 65535")
		}
		cfg.HTTPPort = port
	}

	if value, exists := os.LookupEnv("BIZNES_LOG_LEVEL"); exists {
		switch strings.ToLower(value) {
		case "debug":
			cfg.LogLevel = slog.LevelDebug
		case "info":
			cfg.LogLevel = slog.LevelInfo
		case "warn":
			cfg.LogLevel = slog.LevelWarn
		case "error":
			cfg.LogLevel = slog.LevelError
		default:
			return Config{}, errors.New("BIZNES_LOG_LEVEL must be debug, info, warn, or error")
		}
	}

	return cfg, nil
}
