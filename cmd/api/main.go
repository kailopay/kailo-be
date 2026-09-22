package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/febry3/kailopay-be/internal/adapter/coinmarketcap"
	"github.com/febry3/kailopay-be/internal/adapter/google"
	"github.com/febry3/kailopay-be/internal/adapter/objectstorage"
	personaadapter "github.com/febry3/kailopay-be/internal/adapter/persona"
	stellaradapter "github.com/febry3/kailopay-be/internal/adapter/stellar"
	"github.com/febry3/kailopay-be/internal/adapter/xendit"
	"github.com/febry3/kailopay-be/internal/entity"
	httpapi "github.com/febry3/kailopay-be/internal/handler/http"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
	"github.com/febry3/kailopay-be/internal/usecase"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type avatarBucketStore interface {
	EnsureBucket(ctx context.Context, allowCreate bool) error
}

func ensureAvatarBucket(ctx context.Context, store avatarBucketStore, environment string) error {
	bucketCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	allowBucketCreate := strings.EqualFold(environment, "local") || strings.EqualFold(environment, "test")
	if err := store.EnsureBucket(bucketCtx, allowBucketCreate); err != nil {
		return fmt.Errorf("initializing avatar storage: %w", err)
	}
	return nil
}

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
	if err := ensureAvatarBucket(ctx, avatarStore, cfg.App.Environment); err != nil {
		appLogger.Warn("avatar storage unavailable; avatar operations may fail", slog.Any("error", err))
	}
	health := httpapi.NewHealthHandler(func(checkCtx context.Context) error {
		if err := sqlDB.PingContext(checkCtx); err != nil {
			return err
		}
		return nil
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
	personaClient, err := personaadapter.New(personaadapter.Config{
		BaseURL:            cfg.Persona.BaseURL,
		APIKey:             cfg.Persona.APIKey,
		TemplateID:         cfg.Persona.TemplateID,
		WebhookSecret:      cfg.Persona.WebhookSecret,
		Timeout:            cfg.Persona.Timeout,
		SignatureTolerance: cfg.Persona.SignatureTolerance,
		MaxResponseBytes:   cfg.Persona.MaxResponseBytes,
		HTTPClient:         &http.Client{Timeout: cfg.Persona.Timeout},
	})
	if err != nil {
		return fmt.Errorf("creating Persona client: %w", err)
	}
	kycRepository := repository.NewKYCRepository(db)
	kycService, err := usecase.NewKYCUsecase(usecase.KYCDependencies{
		Repository: kycRepository,
		Persona:    personaClient,
		Clock:      systemClock{},
		NewID:      platform.NewID,
	}, usecase.KYCServiceConfig{EnvironmentID: cfg.Persona.EnvironmentID, HostedFlowURL: cfg.Persona.HostedFlowURL})
	if err != nil {
		return fmt.Errorf("creating KYC service: %w", err)
	}
	kycHandler := httpapi.NewKYCHandlerWithConfig(kycService, appLogger, httpapi.KYCHandlerConfig{
		ExposeProviderDiagnostics: strings.EqualFold(cfg.App.Environment, "local"),
	})
	apiKeyRepository := repository.NewAPIKeyRepository(db)
	apiKeyService, err := usecase.NewAPIKeyUsecase(usecase.APIKeyDependencies{
		Repository: apiKeyRepository,
		KYC:        kycService,
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
	quotePolicy := usecase.QuotePolicy{TTL: cfg.Week1.Onramp.QuoteTTL, MaxAge: cfg.Week1.Onramp.QuoteMaxAge,
		SpreadBPS: cfg.Week1.Onramp.QuoteSpreadBPS}
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
	orderKYC := usecase.KYCStatusReader(kycService)
	if cfg.Anchor.TestAutoApproveKYC {
		orderKYC = usecase.TestAutoApproveKYCStatusReader{Delegate: kycService}
	}
	onrampRepository := repository.NewOnrampRepository(db, cfg.Week1.Stellar.TreasuryAccount, usecase.StellarTestnetNetwork,
		entity.Stroops(cfg.Week1.Stellar.OperatingBufferStroops))
	onrampService, err := usecase.NewOnrampUsecase(usecase.OnrampDependencies{Repository: onrampRepository, Prices: priceClient,
		Treasury: treasuryReader, Gateway: paymentClient, Destinations: treasuryReader, KYC: orderKYC}, usecase.ServiceConfig{
		QuotePolicy: quotePolicy, MinIDR: entity.IDR(cfg.Week1.Onramp.MinIDR),
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
	offrampRepository := repository.NewOfframpRepository(db, cfg.Week1.Offramp.DepositAccount, usecase.StellarTestnetNetwork)
	offrampService, err := usecase.NewOfframpUsecase(usecase.OfframpDependencies{
		Repository: offrampRepository, Prices: priceClient, Destinations: treasuryReader, KYC: orderKYC}, usecase.OfframpServiceConfig{
		QuotePolicy: quotePolicy, MinIDR: entity.IDR(cfg.Week1.Onramp.MinIDR),
		MaxIDR: entity.IDR(cfg.Week1.Onramp.MaxIDR), DepositAccount: cfg.Week1.Offramp.DepositAccount,
		DepositExpiry: cfg.Week1.Offramp.DepositExpiry, NewID: platform.NewID, Now: time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating offramp service: %w", err)
	}
	quoteService, err := usecase.NewQuotePreviewUsecase(priceClient, usecase.QuotePreviewServiceConfig{
		QuotePolicy: quotePolicy, MinIDR: entity.IDR(cfg.Week1.Onramp.MinIDR), MaxIDR: entity.IDR(cfg.Week1.Onramp.MaxIDR), Now: time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating quote preview service: %w", err)
	}
	sep38Service, err := usecase.NewSep38Usecase(priceClient, usecase.Sep38UsecaseConfig{
		QuotePolicy: quotePolicy,
		MinIDR:      entity.IDR(cfg.Week1.Onramp.MinIDR),
		MaxIDR:      entity.IDR(cfg.Week1.Onramp.MaxIDR),
		Now:         time.Now,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-38 service: %w", err)
	}
	offrampHandler := httpapi.NewOfframpHandler(offrampService, appLogger)
	onrampHandler := httpapi.NewOnrampHandler(onrampService, appLogger)
	quoteHandler := httpapi.NewQuoteHandler(quoteService, appLogger)
	sep38Handler := httpapi.NewSep38Handler(sep38Service, appLogger)
	sep24Repository := repository.NewSEP24Repository(db)
	sep24Service, err := usecase.NewSep24Usecase(usecase.Sep24Dependencies{
		Onramp: onrampService, Offramp: offrampService, Orders: onrampService, Transactions: sep24Repository,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-24 service: %w", err)
	}
	publicBaseURL := strings.TrimRight(cfg.Anchor.BaseURL, "/")
	anchorURL, err := url.Parse(publicBaseURL)
	if err != nil {
		return fmt.Errorf("parsing anchor base URL: %w", err)
	}
	if anchorURL.Host == "" {
		return errors.New("anchor base URL has no host")
	}
	sep10Signer, err := stellaradapter.NewSEP10Signer(stellaradapter.SEP10SignerConfig{
		ServerSecret:  cfg.Anchor.SEP10.SigningSecret,
		Network:       cfg.Week1.Stellar.NetworkPassphrase,
		WebAuthDomain: anchorURL.Host,
		HomeDomain:    anchorURL.Host,
		Timebound:     5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-10 signer: %w", err)
	}
	sep10Repository := repository.NewSEP10Repository(db)
	sep10Service, err := usecase.NewSEP10Usecase(usecase.SEP10Dependencies{
		Challenges: sep10Repository,
		Signer:     sep10Signer,
	}, usecase.SEP10ServiceConfig{
		Network:       cfg.Week1.Stellar.NetworkPassphrase,
		HomeDomain:    anchorURL.Host,
		ChallengeTTL:  5 * time.Minute,
		TokenTTL:      15 * time.Minute,
		TokenIssuer:   "kailopay",
		TokenAudience: "kailopay-sep10",
		TokenSecret:   cfg.Anchor.SEP10.SigningSecret,
		Now:           time.Now,
		NewID:         platform.NewID,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-10 service: %w", err)
	}
	sep10Handler := httpapi.NewSEP10Handler(sep10Service, appLogger)
	sep38QuoteRepository := repository.NewSEP38QuoteRepository(db)
	sep38QuoteService, err := usecase.NewSep38QuoteUsecase(usecase.Sep38QuoteDependencies{
		Prices: priceClient, Quotes: sep38QuoteRepository,
	}, usecase.Sep38QuoteConfig{
		QuotePolicy: quotePolicy,
		MinIDR:      entity.IDR(cfg.Week1.Onramp.MinIDR),
		MaxIDR:      entity.IDR(cfg.Week1.Onramp.MaxIDR),
		Now:         time.Now,
		NewID:       platform.NewID,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-38 quote service: %w", err)
	}
	sep38Handler.ConfigureFirmQuotes(sep38QuoteService, sep10Service)
	sep24Config := httpapi.Sep24Config{
		DepositAccount:        cfg.Week1.Offramp.DepositAccount,
		NetworkPassphrase:     cfg.Week1.Stellar.NetworkPassphrase,
		TransferServerURL:     publicBaseURL + "/sep24",
		QuoteServerURL:        publicBaseURL + "/sep38",
		FederationURL:         publicBaseURL + "/federation",
		WebAuthEndpoint:       publicBaseURL + "/auth",
		SigningKey:            sep10Signer.ServerAccount(),
		DepositMinAmountMinor: cfg.Week1.Onramp.MinIDR,
		DepositMaxAmountMinor: cfg.Week1.Onramp.MaxIDR,
	}
	sep24Handler := httpapi.NewSep24Handler(sep24Service, sep24Config, appLogger)
	sep24InteractiveRepository := repository.NewSEP24InteractiveRepository(db)
	sep24InteractiveService, err := usecase.NewSEP24InteractiveUsecase(usecase.Sep24InteractiveDependencies{
		Sessions: sep24InteractiveRepository, Transfers: sep24Service, KYC: kycService, Quotes: sep38QuoteService,
	}, usecase.Sep24InteractiveConfig{
		InteractiveURLBase: publicBaseURL + "/sep24/interactive",
		SessionTTL:         30 * time.Minute,
		Environment:        cfg.App.Environment,
		Network:            usecase.StellarTestnetNetwork,
		TestAutoApproveKYC: cfg.Anchor.TestAutoApproveKYC,
		Now:                time.Now,
		NewID:              platform.NewID,
		Random:             rand.Reader,
	})
	if err != nil {
		return fmt.Errorf("creating SEP-24 interactive service: %w", err)
	}
	sep24Handler.ConfigureInteractive(sep24InteractiveService, authService, sep10Service, cfg.Auth.CookieName)
	webhookRepository := repository.NewWebhookRepository(db)
	webhookService, err := usecase.NewWebhookUsecase(webhookRepository, platform.NewID, time.Now)
	if err != nil {
		return fmt.Errorf("creating webhook service: %w", err)
	}
	webhookHandler := httpapi.NewWebhookHandler(webhookService, appLogger)
	callbackHandler := httpapi.NewXenditCallbackHandler(callbackService, appLogger)
	orderPrincipalMiddleware := middleware.RequireOrderPrincipal(apiKeyService, authService, cfg.Auth.CookieName, cfg.HTTP.AllowedOrigins)
	router, err := httpapi.NewRouter(appLogger, health, authHandler, sessionMiddleware,
		httpapi.WithAPIKeys(apiKeyHandler), httpapi.WithKYC(kycHandler),
		httpapi.WithOnramp(onrampHandler, orderPrincipalMiddleware),
		httpapi.WithOfframp(offrampHandler), httpapi.WithQuote(quoteHandler, orderPrincipalMiddleware),
		httpapi.WithSEP10(sep10Handler),
		httpapi.WithSep38(sep38Handler),
		httpapi.WithSep38Quotes(sep38Handler, middleware.RequireSEP10(sep10Service)),
		httpapi.WithSep24(sep24Handler, orderPrincipalMiddleware),
		httpapi.WithSep24Interactive(sep24Handler, middleware.RequireSEP10(sep10Service)),
		httpapi.WithWebhooks(webhookHandler), httpapi.WithXenditCallback(callbackHandler))
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
