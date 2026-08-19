package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/febry3/kailopay-be/internal/adapter/auth0"
	httpapi "github.com/febry3/kailopay-be/internal/handler/http"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
	auth "github.com/febry3/kailopay-be/internal/usecase"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("api stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := platform.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	appLogger, err := platform.NewLogger(cfg.Logging.Level, cfg.Logging.Format, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	appLogger = platform.WithMetadata(appLogger, "kailopay", "api", cfg.App.Environment, cfg.App.Version)
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

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("getting sql database: %w", err)
	}
	health := httpapi.NewHealthHandler(
		func(checkCtx context.Context) error {
			return sqlDB.PingContext(checkCtx)
		},
		cfg.Health.CheckTimeout,
		appLogger,
	)
	health.MarkStarted()
	encryptionKey, err := base64.StdEncoding.DecodeString(cfg.Auth.TransactionEncryptionKey)
	if err != nil {
		return fmt.Errorf("decoding auth transaction key: %w", err)
	}
	sessionHMACKey, err := base64.StdEncoding.DecodeString(cfg.Auth.SessionHMACKey)
	if err != nil {
		return fmt.Errorf("decoding auth session key: %w", err)
	}
	authProvider, err := auth0.NewClient(ctx, cfg.Auth, &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return fmt.Errorf("creating Auth0 client: %w", err)
	}
	authRepository := repository.NewAuthRepository(db, cfg.Auth.SessionIdleLifetime)
	authService, err := auth.NewAuthUsecase(authProvider, authRepository, authRepository, nil, auth.Config{
		TransactionEncryptionKey: encryptionKey,
		SessionHMACKey:           sessionHMACKey,
		SessionAbsoluteLifetime:  cfg.Auth.SessionAbsoluteLifetime,
		SessionIdleLifetime:      cfg.Auth.SessionIdleLifetime,
		TransactionLifetime:      cfg.Auth.TransactionLifetime,
	})
	if err != nil {
		return fmt.Errorf("creating auth service: %w", err)
	}
	authHandler := httpapi.NewAuthHandler(authService, cfg.Auth)
	sessionMiddleware := middleware.RequireSessionWithCookie(authService, cfg.Auth.CookieName)
	router, err := httpapi.NewRouter(appLogger, health, authHandler, sessionMiddleware)
	if err != nil {
		return fmt.Errorf("creating http router: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving http: %w", err)
	case <-shutdownCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down http server: %w", err)
		}
		return nil
	}
}
