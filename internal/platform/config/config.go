package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort               int
	LogLevel               slog.Level
	HTTPReadHeaderTimeout  time.Duration
	HTTPReadTimeout        time.Duration
	HTTPWriteTimeout       time.Duration
	HTTPIdleTimeout        time.Duration
	HTTPShutdownTimeout    time.Duration
	DatabaseURL            string
	DatabaseConnectTimeout time.Duration
	DatabaseHealthTimeout  time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPPort:               8080,
		LogLevel:               slog.LevelInfo,
		HTTPReadHeaderTimeout:  5 * time.Second,
		HTTPReadTimeout:        15 * time.Second,
		HTTPWriteTimeout:       15 * time.Second,
		HTTPIdleTimeout:        60 * time.Second,
		HTTPShutdownTimeout:    10 * time.Second,
		DatabaseConnectTimeout: 5 * time.Second,
		DatabaseHealthTimeout:  2 * time.Second,
	}

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

	for _, setting := range []struct {
		name  string
		value *time.Duration
	}{
		{"BIZNES_HTTP_READ_HEADER_TIMEOUT", &cfg.HTTPReadHeaderTimeout},
		{"BIZNES_HTTP_READ_TIMEOUT", &cfg.HTTPReadTimeout},
		{"BIZNES_HTTP_WRITE_TIMEOUT", &cfg.HTTPWriteTimeout},
		{"BIZNES_HTTP_IDLE_TIMEOUT", &cfg.HTTPIdleTimeout},
		{"BIZNES_HTTP_SHUTDOWN_TIMEOUT", &cfg.HTTPShutdownTimeout},
		{"BIZNES_DATABASE_CONNECT_TIMEOUT", &cfg.DatabaseConnectTimeout},
		{"BIZNES_DATABASE_HEALTH_TIMEOUT", &cfg.DatabaseHealthTimeout},
	} {
		if value, exists := os.LookupEnv(setting.name); exists {
			duration, err := time.ParseDuration(value)
			if err != nil || duration <= 0 {
				return Config{}, fmt.Errorf("%s must be a positive duration", setting.name)
			}
			*setting.value = duration
		}
	}

	cfg.DatabaseURL = os.Getenv("BIZNES_DATABASE_URL")
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return Config{}, errors.New("BIZNES_DATABASE_URL is required")
	}

	return cfg, nil
}
