package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	stellaradapter "github.com/febry3/kailopay-be/internal/adapter/stellar"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("worker stopped", slog.Any("error", err))
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
	logger = platform.WithMetadata(logger, "kailopay", "worker", cfg.App.Environment, cfg.App.Version)
	slog.SetDefault(logger)
	db, closeDB, err := platform.Open(ctx, platform.DBConfig{DSN: cfg.Database.DSN, MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns, ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime, PingTimeout: cfg.Database.PingTimeout}, logger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := closeDB(); err != nil {
			logger.Error("closing worker database failed", slog.Any("error", err))
		}
	}()
	treasurySecret, err := platform.LoadWorkerTreasurySecret()
	if err != nil {
		return fmt.Errorf("loading treasury secret: %w", err)
	}
	network, err := stellaradapter.New(stellaradapter.Config{HorizonURL: cfg.Week1.Stellar.HorizonURL,
		NetworkPassphrase: cfg.Week1.Stellar.NetworkPassphrase, TreasurySecret: treasurySecret,
		HTTPClient: &http.Client{Timeout: cfg.Week1.Stellar.Timeout}, TransactionTimeout: cfg.Week1.Worker.SubmissionTimeout})
	if err != nil {
		return fmt.Errorf("creating Stellar client: %w", err)
	}
	store := repository.NewSettlementRepository(db, cfg.Week1.Worker.MaxAttempts)
	service, err := usecase.NewSettlementUsecase(store, network, usecase.SettlementConfig{LeaseDuration: cfg.Week1.Worker.LeaseDuration,
		RetryDelay: cfg.Week1.Worker.RetryDelay, Now: time.Now})
	if err != nil {
		return fmt.Errorf("creating settlement service: %w", err)
	}
	workerID, err := platform.NewID()
	if err != nil {
		return fmt.Errorf("generating worker id: %w", err)
	}
	ticker := time.NewTicker(cfg.Week1.Worker.PollInterval)
	defer ticker.Stop()
	var lastSweep time.Time
	for {
		if time.Since(lastSweep) >= cfg.Week1.Worker.PollInterval {
			released, err := service.ReleaseExpired(ctx)
			if err != nil {
				logger.ErrorContext(ctx, "releasing expired reservations failed", slog.Any("error", err))
			} else if released > 0 {
				logger.InfoContext(ctx, "released expired treasury reservations", slog.Int("released", released))
			}
			lastSweep = time.Now()
		}
		processed, err := service.RunOnce(ctx, workerID)
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "settlement job failed", slog.Any("error", err))
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
