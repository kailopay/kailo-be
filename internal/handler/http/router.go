package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	openapi "github.com/febry3/kailopay-be/openapi"
	"github.com/gin-gonic/gin"
)

type routerOptions struct {
	apiKeys               *APIKeyHandler
	kyc                   *KYCHandler
	quote                 *QuoteHandler
	sep38                 *Sep38Handler
	sep38Quotes           bool
	sep10                 *SEP10Handler
	onramp                *OnrampHandler
	offramp               *OfframpHandler
	sep24                 *Sep24Handler
	sep24Interactive      bool
	webhooks              *WebhookHandler
	developerDashboard    *DeveloperDashboardHandler
	developerWallet       *DeveloperWalletHandler
	requireOrderPrincipal gin.HandlerFunc
	requireSEP10          gin.HandlerFunc
	xenditCallback        *XenditCallbackHandler
}

func WithWebhooks(handler *WebhookHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("webhook handler is required")
		}
		options.webhooks = handler
		return nil
	}
}

func WithDeveloperDashboard(handler *DeveloperDashboardHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("developer dashboard handler is required")
		}
		options.developerDashboard = handler
		return nil
	}
}

func WithDeveloperWallet(handler *DeveloperWalletHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("developer wallet handler is required")
		}
		options.developerWallet = handler
		return nil
	}
}

func WithKYC(handler *KYCHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("kyc handler is required")
		}
		options.kyc = handler
		return nil
	}
}

func WithSep24(handler *Sep24Handler, requireOrderPrincipal gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || requireOrderPrincipal == nil {
			return errors.New("sep24 handler and order principal middleware are required")
		}
		options.sep24 = handler
		options.requireOrderPrincipal = requireOrderPrincipal
		return nil
	}
}

// WithSep24Interactive enables the canonical wallet-owned initiation routes.
// The existing order-principal middleware remains attached to the legacy
// routes and to transaction history/lookups.
func WithSep24Interactive(handler *Sep24Handler, requireSEP10 gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || requireSEP10 == nil || handler.interactive == nil {
			return errors.New("configured SEP-24 interactive handler and SEP-10 middleware are required")
		}
		options.sep24 = handler
		options.sep24Interactive = true
		options.requireSEP10 = requireSEP10
		return nil
	}
}

func WithOfframp(handler *OfframpHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("offramp handler is required")
		}
		options.offramp = handler
		return nil
	}
}

func WithXenditCallback(handler *XenditCallbackHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("Xendit callback handler is required")
		}
		options.xenditCallback = handler
		return nil
	}
}

func WithOnramp(handler *OnrampHandler, requireOrderPrincipal gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || requireOrderPrincipal == nil {
			return errors.New("onramp handler and order principal middleware are required")
		}
		options.onramp = handler
		options.requireOrderPrincipal = requireOrderPrincipal
		return nil
	}
}

func WithQuote(handler *QuoteHandler, requireOrderPrincipal gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || requireOrderPrincipal == nil {
			return errors.New("quote handler and order principal middleware are required")
		}
		options.quote = handler
		options.requireOrderPrincipal = requireOrderPrincipal
		return nil
	}
}

func WithSep38(handler *Sep38Handler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("sep-38 handler is required")
		}
		options.sep38 = handler
		return nil
	}
}

func WithSep38Quotes(handler *Sep38Handler, requireSEP10 gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || handler.quotes == nil || requireSEP10 == nil {
			return errors.New("configured SEP-38 quote handler and SEP-10 middleware are required")
		}
		options.sep38 = handler
		options.sep38Quotes = true
		options.requireSEP10 = requireSEP10
		return nil
	}
}

func WithSEP10(handler *SEP10Handler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("SEP-10 handler is required")
		}
		options.sep10 = handler
		return nil
	}
}

type RouterOption func(*routerOptions) error

func WithAPIKeys(handler *APIKeyHandler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("api key handler is required")
		}
		options.apiKeys = handler
		return nil
	}
}

func NewRouter(logger *slog.Logger, health *HealthHandler, authHandler *AuthHandler, requireSession gin.HandlerFunc, options ...RouterOption) (*gin.Engine, error) {
	if health == nil {
		return nil, errors.New("health handler is required")
	}
	if authHandler == nil || requireSession == nil {
		return nil, errors.New("auth handler and session middleware are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	documentationHandler := openapi.UIHandler()
	configured := routerOptions{}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("router option is required")
		}
		if err := option(&configured); err != nil {
			return nil, err
		}
	}

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(logger),
		middleware.Logging(logger),
		protocolCORS,
	)
	// Kubernetes-style probes are the canonical endpoints. The aliases keep
	// the original bootstrap contract available to existing local callers.
	router.GET("/livez", health.Liveness)
	router.GET("/readyz", health.Readiness)
	router.GET("/startupz", health.Startup)
	router.GET("/health", health.Health)
	router.GET("/healthz", health.Liveness)
	router.GET("/ready", health.Ready)
	router.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusPermanentRedirect, "/docs/")
	})
	router.GET("/docs/*path", gin.WrapH(http.StripPrefix("/docs", documentationHandler)))
	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	if configured.sep10 != nil {
		router.GET("/auth", configured.sep10.Challenge)
		router.POST("/auth", configured.sep10.Exchange)
	}
	router.GET("/auth/google/login", authHandler.GoogleLogin)
	router.GET("/auth/google/callback", authHandler.GoogleCallback)
	router.POST("/auth/logout", authHandler.Logout)
	router.POST("/auth/email/verify", authHandler.VerifyEmail)
	router.POST("/auth/email/resend", authHandler.ResendVerification)
	router.POST("/auth/password/forgot", authHandler.ForgotPassword)
	router.POST("/auth/password/reset", authHandler.ResetPassword)
	authRoutes := router.Group("/auth")
	authRoutes.Use(requireSession)
	authRoutes.POST("/password/change", authHandler.ChangePassword)
	authRoutes.GET("/me", authHandler.Me)
	authRoutes.PATCH("/me", authHandler.UpdateProfile)
	authRoutes.PUT("/me/avatar", authHandler.UpdateAvatar)
	authRoutes.GET("/me/avatar", authHandler.Avatar)
	authRoutes.DELETE("/me/avatar", authHandler.DeleteAvatar)
	if configured.apiKeys != nil {
		apiKeyRoutes := router.Group("/v1/api-keys")
		apiKeyRoutes.Use(requireSession)
		apiKeyRoutes.POST("", configured.apiKeys.Create)
		apiKeyRoutes.GET("", configured.apiKeys.List)
		apiKeyRoutes.DELETE("/:id", configured.apiKeys.Revoke)
	}
	if configured.kyc != nil {
		kycRoutes := router.Group("/v1/kyc")
		kycRoutes.Use(requireSession)
		kycRoutes.GET("", configured.kyc.Status)
		kycRoutes.POST("/inquiry", configured.kyc.Inquiry)
		router.POST("/callbacks/kyc/persona", gin.WrapH(configured.kyc))
	}
	if configured.webhooks != nil {
		webhookRoutes := router.Group("/v1/webhook-endpoints")
		webhookRoutes.Use(requireSession)
		webhookRoutes.POST("", configured.webhooks.Create)
		webhookRoutes.GET("", configured.webhooks.List)
		webhookRoutes.GET("/:id", configured.webhooks.Get)
		webhookRoutes.POST("/:id/test", configured.webhooks.Test)
		webhookRoutes.DELETE("/:id", configured.webhooks.Disable)
		deliveryRoutes := router.Group("/v1/webhook-deliveries")
		deliveryRoutes.Use(requireSession)
		deliveryRoutes.GET("", configured.webhooks.Deliveries)
		deliveryRoutes.POST("/:id/replay", configured.webhooks.Replay)
	}
	if configured.developerDashboard != nil {
		developerRoutes := router.Group("/v1/developer")
		developerRoutes.Use(requireSession)
		developerRoutes.GET("/overview", configured.developerDashboard.Overview)
		developerRoutes.GET("/analytics", configured.developerDashboard.Analytics)
		developerRoutes.GET("/revenue/summary", configured.developerDashboard.RevenueSummary)
		developerRoutes.GET("/revenue/entries", configured.developerDashboard.RevenueEntries)
		developerRoutes.GET("/orders", configured.developerDashboard.Orders)
	}
	if configured.developerWallet != nil {
		developerWalletRoutes := router.Group("/v1/developer/wallets")
		developerWalletRoutes.Use(requireSession)
		developerWalletRoutes.GET("", configured.developerWallet.List)
		developerWalletRoutes.POST("", configured.developerWallet.Create)
		developerWalletRoutes.PATCH("/:id", configured.developerWallet.Update)
		developerWalletRoutes.DELETE("/:id", configured.developerWallet.Delete)
	}
	if configured.onramp != nil {
		onrampRoutes := router.Group("/v1")
		onrampRoutes.Use(configured.requireOrderPrincipal)
		onrampRoutes.POST("/onramps", configured.onramp.Create)
		if configured.offramp != nil {
			onrampRoutes.POST("/offramps", configured.offramp.Create)
		}
		onrampRoutes.GET("/orders", configured.onramp.List)
		onrampRoutes.GET("/orders/:id", configured.onramp.Get)
	}
	if configured.quote != nil {
		router.POST("/v1/quotes", configured.quote.Create)
	}
	if configured.xenditCallback != nil {
		router.POST("/callbacks/payments/xendit", gin.WrapH(configured.xenditCallback))
	}
	if configured.sep24 != nil {
		router.GET("/.well-known/stellar.toml", configured.sep24.StellarToml)
		router.GET("/federation", configured.sep24.Federation)
		router.GET("/sep24/info", configured.sep24.Info)
		sep24Routes := router.Group("/sep24")
		sep24Routes.Use(configured.requireOrderPrincipal)
		if configured.sep24Interactive {
			walletSep24Routes := router.Group("/sep24")
			walletSep24Routes.Use(configured.requireSEP10)
			walletSep24Routes.POST("/transactions/deposit/interactive", configured.sep24.InteractiveDeposit)
			walletSep24Routes.POST("/transactions/withdraw/interactive", configured.sep24.InteractiveWithdraw)
		} else {
			sep24Routes.POST("/transactions/deposit/interactive", configured.sep24.Deposit)
			sep24Routes.POST("/transactions/withdraw/interactive", configured.sep24.Withdraw)
		}
		if configured.sep24Interactive {
			walletSep24Routes := router.Group("/sep24")
			walletSep24Routes.Use(configured.requireSEP10)
			walletSep24Routes.GET("/transactions", configured.sep24.WalletTransactions)
			walletSep24Routes.GET("/transaction", configured.sep24.WalletTransaction)
			router.GET("/sep24/interactive/:id", configured.sep24.Interactive)
			router.POST("/sep24/interactive/:id", configured.sep24.InteractiveComplete)
		} else {
			sep24Routes.GET("/transactions", configured.sep24.Transactions)
			sep24Routes.GET("/transaction", configured.sep24.Transaction)
			sep24Routes.GET("/interactive/:id", configured.sep24.Interactive)
		}
		// Keep the original sandbox paths for existing local clients while the
		// standard nested SEP-24 paths become the documented contract.
		sep24Routes.POST("/deposit", configured.sep24.Deposit)
		sep24Routes.POST("/withdraw", configured.sep24.Withdraw)
	}
	if configured.sep38 != nil {
		sep38Routes := router.Group("/sep38")
		sep38Routes.GET("/info", configured.sep38.Info)
		sep38Routes.GET("/prices", configured.sep38.Prices)
		sep38Routes.GET("/price", configured.sep38.Price)
		if configured.sep38Quotes {
			quoteRoutes := router.Group("/sep38")
			quoteRoutes.Use(configured.requireSEP10)
			quoteRoutes.POST("/quote", configured.sep38.CreateQuote)
			quoteRoutes.GET("/quote/:id", configured.sep38.GetQuote)
		}
	}
	return router, nil
}

func protocolCORS(c *gin.Context) {
	path := c.Request.URL.Path
	protocol := path == "/auth" || path == "/.well-known/stellar.toml" || strings.HasPrefix(path, "/sep24") || strings.HasPrefix(path, "/sep38")
	if !protocol {
		c.Next()
		return
	}
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
	c.Header("Access-Control-Expose-Headers", "Content-Type")
	if c.Request.Method == http.MethodOptions {
		c.AbortWithStatus(http.StatusNoContent)
		return
	}
	c.Next()
}
