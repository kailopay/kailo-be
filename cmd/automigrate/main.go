package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("automigrate failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := platform.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.App.Environment != "local" && cfg.App.Environment != "test" {
		return fmt.Errorf("automigrate is disabled for environment %q", cfg.App.Environment)
	}

	appLogger, err := platform.NewLogger(cfg.Logging.Level, cfg.Logging.Format, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	appLogger = platform.WithMetadata(appLogger, "kailopay", "automigrate", cfg.App.Environment, cfg.App.Version)
	slog.SetDefault(appLogger)
	db, closeDB, err := platform.Open(ctx, platform.DBConfig{
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime,
		PingTimeout:     cfg.Database.PingTimeout,
	}, appLogger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if closeErr := closeDB(); closeErr != nil {
			appLogger.Error("closing database failed", slog.Any("error", closeErr))
		}
	}()

	if err := db.WithContext(ctx).AutoMigrate(repository.MigrationModels()...); err != nil {
		return fmt.Errorf("running automigrate: %w", err)
	}
	appLogger.Info("database automigrate completed", slog.String("environment", cfg.App.Environment))
	return nil
}
