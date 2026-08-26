package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/febry3/kailopay-be/internal/adapter/coinmarketcap"
	"github.com/febry3/kailopay-be/internal/adapter/google"
	mail "github.com/febry3/kailopay-be/internal/adapter/mail"
	"github.com/febry3/kailopay-be/internal/adapter/objectstorage"
	stellaradapter "github.com/febry3/kailopay-be/internal/adapter/stellar"
	"github.com/febry3/kailopay-be/internal/adapter/xendit"
	"github.com/febry3/kailopay-be/internal/entity"
	httpapi "github.com/febry3/kailopay-be/internal/handler/http"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
	"github.com/febry3/kailopay-be/internal/usecase"
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
	avatarStore, err := objectstorage.New(cfg.ObjectStorage)
	if err != nil {
		return fmt.Errorf("creating avatar storage: %w", err)
	}
	bucketCtx, cancelBucketCheck := context.WithTimeout(ctx, 10*time.Second)
	allowBucketCreate := strings.EqualFold(cfg.App.Environment, "local") || strings.EqualFold(cfg.App.Environment, "test")
	if err := avatarStore.EnsureBucket(bucketCtx, allowBucketCreate); err != nil {
		cancelBucketCheck()
		return fmt.Errorf("initializing avatar storage: %w", err)
	}
	cancelBucketCheck()
	health := httpapi.NewHealthHandler(func(checkCtx context.Context) error {
		if err := sqlDB.PingContext(checkCtx); err != nil {
			return err
		}
		return avatarStore.EnsureBucket(checkCtx, false)
	}, cfg.Health.CheckTimeout, appLogger)
	encryptionKey, err := base64.StdEncoding.DecodeString(cfg.Auth.TransactionEncryptionKey)
	if err != nil {
		return fmt.Errorf("decoding auth transaction key: %w", err)
	}
	sessionHMACKey, err := base64.StdEncoding.DecodeString(cfg.Auth.SessionHMACKey)
	if err != nil {
		return fmt.Errorf("decoding auth session key: %w", err)
	}
	// Google sign-in is optional: empty credentials disable the endpoints.
	var authProvider usecase.Provider
	if cfg.Auth.Google.ClientID != "" {
		googleClient, err := google.NewClient(ctx, cfg.Auth.Google, &http.Client{Timeout: 10 * time.Second})
		if err != nil {
			return fmt.Errorf("creating Google client: %w", err)
		}
		authProvider = googleClient
	}
	authRepository := repository.NewAuthRepository(db, cfg.Auth.SessionIdleLifetime)
	authService, err := usecase.NewAuthUsecase(usecase.AuthDependencies{
		Provider:   authProvider,
		Repository: authRepository,
		Avatars:    avatarStore,
		Mailer:     mail.NewConsoleSender(appLogger),
	}, nil, usecase.AuthConfig{
		TransactionEncryptionKey: encryptionKey,
		SessionHMACKey:           sessionHMACKey,
		SessionAbsoluteLifetime:  cfg.Auth.SessionAbsoluteLifetime,
		SessionIdleLifetime:      cfg.Auth.SessionIdleLifetime,
		TransactionLifetime:      cfg.Auth.TransactionLifetime,
		AvatarMaxBytes:           cfg.Auth.AvatarMaxBytes,
		EmailLinkBaseURL:         cfg.Auth.EmailLinkBaseURL,
	})
	if err != nil {
		return fmt.Errorf("creating auth service: %w", err)
	}
	authHandler := httpapi.NewAuthHandler(authService, cfg.Auth, appLogger)
	sessionMiddleware := middleware.RequireSessionWithCookie(authService, cfg.Auth.CookieName)
	apiKeyRepository := repository.NewAPIKeyRepository(db)
	apiKeyService, err := usecase.NewAPIKeyUsecase(usecase.APIKeyDependencies{
		Repository: apiKeyRepository,
	}, usecase.APIKeyConfig{
		Pepper: []byte(cfg.Week1.APIKeyPepper),
		Random: rand.Reader,
		NewID:  platform.NewID,
		Now:    time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating api key service: %w", err)
	}
	apiKeyHandler := httpapi.NewAPIKeyHandler(apiKeyService, appLogger)
	priceClient, err := coinmarketcap.New(coinmarketcap.Config{BaseURL: cfg.Week1.CoinMarketCap.BaseURL,
		APIKey: cfg.Week1.CoinMarketCap.APIKey, HTTPClient: &http.Client{Timeout: cfg.Week1.CoinMarketCap.Timeout}})
	if err != nil {
		return fmt.Errorf("creating CoinMarketCap client: %w", err)
	}
	paymentClient, err := xendit.New(xendit.Config{BaseURL: cfg.Week1.Xendit.BaseURL, SecretKey: cfg.Week1.Xendit.SecretKey,
		CallbackToken: cfg.Week1.Xendit.CallbackToken, APIVersion: cfg.Week1.Xendit.APIVersion,
		QRISChannel: cfg.Week1.Xendit.QRISChannel, VAChannel: cfg.Week1.Xendit.VAChannel,
		HTTPClient: &http.Client{Timeout: cfg.Week1.Xendit.Timeout}})
	if err != nil {
		return fmt.Errorf("creating Xendit client: %w", err)
	}
	treasuryReader, err := stellaradapter.NewBalanceReader(cfg.Week1.Stellar.HorizonURL, &http.Client{Timeout: cfg.Week1.Stellar.Timeout})
	if err != nil {
		return fmt.Errorf("creating Stellar balance reader: %w", err)
	}
	onrampRepository := repository.NewOnrampRepository(db, cfg.Week1.Stellar.TreasuryAccount, "testnet",
		entity.Stroops(cfg.Week1.Stellar.OperatingBufferStroops))
	onrampService, err := usecase.NewOnrampUsecase(usecase.OnrampDependencies{Repository: onrampRepository, Prices: priceClient,
		Treasury: treasuryReader, Gateway: paymentClient, Destinations: treasuryReader}, usecase.ServiceConfig{
		QuotePolicy: usecase.QuotePolicy{TTL: cfg.Week1.Onramp.QuoteTTL, MaxAge: cfg.Week1.Onramp.QuoteMaxAge,
			SpreadBPS: cfg.Week1.Onramp.QuoteSpreadBPS}, MinIDR: entity.IDR(cfg.Week1.Onramp.MinIDR),
		MaxIDR: entity.IDR(cfg.Week1.Onramp.MaxIDR), TreasuryAccount: cfg.Week1.Stellar.TreasuryAccount,
		NewID: platform.NewID, Now: time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating onramp service: %w", err)
	}
	callbackService, err := usecase.NewOnrampCallbackUsecase(paymentClient, onrampRepository)
	if err != nil {
		return fmt.Errorf("creating payment callback service: %w", err)
	}
	offrampRepository := repository.NewOfframpRepository(db, cfg.Week1.Offramp.DepositAccount, "testnet")
	offrampService, err := usecase.NewOfframpUsecase(usecase.OfframpDependencies{
		Repository: offrampRepository, Prices: priceClient, Destinations: treasuryReader}, usecase.OfframpServiceConfig{
		QuotePolicy: usecase.QuotePolicy{TTL: cfg.Week1.Onramp.QuoteTTL, MaxAge: cfg.Week1.Onramp.QuoteMaxAge,
			SpreadBPS: cfg.Week1.Onramp.QuoteSpreadBPS}, MinIDR: entity.IDR(cfg.Week1.Onramp.MinIDR),
		MaxIDR: entity.IDR(cfg.Week1.Onramp.MaxIDR), DepositAccount: cfg.Week1.Offramp.DepositAccount,
		DepositExpiry: cfg.Week1.Offramp.DepositExpiry, NewID: platform.NewID, Now: time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating offramp service: %w", err)
	}
	offrampHandler := httpapi.NewOfframpHandler(offrampService, appLogger)
	onrampHandler := httpapi.NewOnrampHandler(onrampService, appLogger)
	callbackHandler := httpapi.NewXenditCallbackHandler(callbackService, appLogger)
	apiKeyMiddleware := middleware.RequireAPIKey(apiKeyService)
	router, err := httpapi.NewRouter(appLogger, health, authHandler, sessionMiddleware,
		httpapi.WithAPIKeys(apiKeyHandler), httpapi.WithOnramp(onrampHandler, apiKeyMiddleware),
		httpapi.WithOfframp(offrampHandler), httpapi.WithXenditCallback(callbackHandler))
	if err != nil {
		return fmt.Errorf("creating http router: %w", err)
	}
	health.MarkStarted()

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
