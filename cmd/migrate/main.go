package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/febry3/kailopay-be/internal/platform"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("migration failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := platform.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger, err := platform.NewLogger(cfg.Logging.Level, cfg.Logging.Format, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	db, closeDB, err := platform.Open(ctx, platform.DBConfig{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
		PingTimeout:     cfg.Database.PingTimeout,
	}, logger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if closeErr := closeDB(); closeErr != nil {
			logger.Error("closing database failed", slog.Any("error", closeErr))
		}
	}()
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("getting SQL database: %w", err)
	}
	if err := platform.ApplyMigrations(ctx, sqlDB, os.DirFS("migrations")); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	logger.Info("database migrations completed")
	return nil
}
