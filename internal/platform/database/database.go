package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wikiccu/biznes/internal/platform/config"
)

func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		// Driver errors can contain credentials from malformed connection strings.
		return nil, errors.New("BIZNES_DATABASE_URL must be a valid PostgreSQL connection string")
	}
	if poolConfig.HealthCheckPeriod <= 0 || poolConfig.MinConns < 0 || poolConfig.MinIdleConns < 0 ||
		poolConfig.MinConns > poolConfig.MaxConns || poolConfig.MinIdleConns > poolConfig.MaxConns {
		return nil, errors.New("BIZNES_DATABASE_URL contains invalid pool settings")
	}
	if poolConfig.ConnConfig.ConnectTimeout <= 0 || poolConfig.ConnConfig.ConnectTimeout > cfg.DatabaseConnectTimeout {
		poolConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout
	}
	poolConfig.PingTimeout = cfg.DatabaseHealthTimeout

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, errors.New("create PostgreSQL connection pool failed")
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		if err := connectCtx.Err(); err != nil {
			return nil, fmt.Errorf("verify PostgreSQL connection: %w", err)
		}
		return nil, errors.New("verify PostgreSQL connection: database unavailable or credentials rejected")
	}
	return pool, nil
}
