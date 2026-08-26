package httpapi

import (
	"errors"
	"log/slog"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/gin-gonic/gin"
)

type routerOptions struct {
	apiKeys        *APIKeyHandler
	onramp         *OnrampHandler
	offramp        *OfframpHandler
	sep24          *Sep24Handler
	webhooks       *WebhookHandler
	requireAPIKey  gin.HandlerFunc
	xenditCallback *XenditCallbackHandler
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

func WithSep24(handler *Sep24Handler) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil {
			return errors.New("sep24 handler is required")
		}
		options.sep24 = handler
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

func WithOnramp(handler *OnrampHandler, requireAPIKey gin.HandlerFunc) RouterOption {
	return func(options *routerOptions) error {
		if handler == nil || requireAPIKey == nil {
			return errors.New("onramp handler and api key middleware are required")
		}
		options.onramp = handler
		options.requireAPIKey = requireAPIKey
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
	)
	// Kubernetes-style probes are the canonical endpoints. The aliases keep
	// the original bootstrap contract available to existing local callers.
	router.GET("/livez", health.Liveness)
	router.GET("/readyz", health.Readiness)
	router.GET("/startupz", health.Startup)
	router.GET("/health", health.Health)
	router.GET("/healthz", health.Liveness)
	router.GET("/ready", health.Ready)
	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
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
	if configured.webhooks != nil {
		webhookRoutes := router.Group("/v1/webhook-endpoints")
		webhookRoutes.Use(requireSession)
		webhookRoutes.POST("", configured.webhooks.Create)
		webhookRoutes.GET("", configured.webhooks.List)
		webhookRoutes.DELETE("/:id", configured.webhooks.Disable)
	}
	if configured.onramp != nil {
		onrampRoutes := router.Group("/v1")
		onrampRoutes.Use(configured.requireAPIKey)
		onrampRoutes.POST("/onramps", configured.onramp.Create)
		if configured.offramp != nil {
			onrampRoutes.POST("/offramps", configured.offramp.Create)
		}
		onrampRoutes.GET("/orders", configured.onramp.List)
		onrampRoutes.GET("/orders/:id", configured.onramp.Get)
	}
	if configured.xenditCallback != nil {
		router.POST("/callbacks/payments/xendit", gin.WrapH(configured.xenditCallback))
	}
	if configured.sep24 != nil {
		router.GET("/.well-known/stellar.toml", configured.sep24.StellarToml)
		router.GET("/federation", configured.sep24.Federation)
		router.GET("/sep24/info", configured.sep24.Info)
		router.POST("/sep24/deposit", configured.sep24.Deposit)
		router.POST("/sep24/withdraw", configured.sep24.Withdraw)
		router.GET("/sep24/transaction", configured.sep24.Transaction)
	}
	return router, nil
}
