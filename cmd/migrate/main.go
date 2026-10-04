package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/wikiccu/biznes/internal/platform/config"
	"github.com/wikiccu/biznes/internal/platform/database"
)

func main() {
	dir := flag.String("dir", "migrations", "directory containing SQL migrations")
	timeout := flag.Duration("timeout", 5*time.Minute, "work deadline, including connection and lock waiting")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: migrate [-dir migrations] [-timeout 5m] up|down|status")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 || *timeout <= 0 {
		flag.Usage()
		os.Exit(2)
	}
	command := flag.Arg(0)
	if command != "up" && command != "down" && command != "status" {
		flag.Usage()
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("service", "biznes", "command", command)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := run(ctx, *dir, command, logger); err != nil {
		logger.Error("migration command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dir, command string, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("initialize PostgreSQL migration lock failed")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir),
		goose.WithTableName("public.goose_db_version"),
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		return errors.New("load migrations failed; check directory, SQL filenames, and unique versions")
	}

	switch command {
	case "up":
		var results []*goose.MigrationResult
		results, err = provider.Up(ctx)
		if err == nil {
			logger.Info("migrations applied", "count", len(results))
		}
	case "down":
		var result *goose.MigrationResult
		result, err = provider.Down(ctx)
		if errors.Is(err, goose.ErrNoNextVersion) {
			logger.Info("no migration to roll back")
			return nil
		}
		if err == nil {
			logger.Info("migration rolled back", "version", result.Source.Version)
		}
	case "status":
		var statuses []*goose.MigrationStatus
		statuses, err = provider.Status(ctx)
		if err == nil {
			for _, status := range statuses {
				logger.Info("migration status", "version", status.Source.Version, "state", status.State)
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Migration errors can contain SQL, credentials, or database values.
		var partial *goose.PartialError
		if errors.As(err, &partial) {
			return fmt.Errorf("migration version %d failed; inspect SQL and database state before retrying", partial.Failed.Source.Version)
		}
		return errors.New("migration operation failed; check database access and migration history")
	}
	return nil
}
