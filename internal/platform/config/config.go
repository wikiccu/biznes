package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	HTTPPort int
}

func Load() (Config, error) {
	cfg := Config{HTTPPort: 8080}

	if value, exists := os.LookupEnv("BIZNES_HTTP_PORT"); exists {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, errors.New("BIZNES_HTTP_PORT must be an integer between 1 and 65535")
		}
		cfg.HTTPPort = port
	}

	return cfg, nil
}
