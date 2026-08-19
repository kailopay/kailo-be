package httpapi

import (
	"errors"
	"log/slog"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/gin-gonic/gin"
)

func NewRouter(logger *slog.Logger, health *HealthHandler, authHandler *AuthHandler, requireSession gin.HandlerFunc) (*gin.Engine, error) {
	if health == nil {
		return nil, errors.New("health handler is required")
	}
	if authHandler == nil || requireSession == nil {
		return nil, errors.New("auth handler and session middleware are required")
	}
	if logger == nil {
		logger = slog.Default()
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
	router.GET("/auth/login", authHandler.Login)
	router.GET("/auth/callback", authHandler.Callback)
	router.POST("/auth/logout", authHandler.Logout)
	router.POST("/auth/password/forgot", authHandler.ForgotPassword)
	router.POST("/internal/auth/password-reset-completed", authHandler.PasswordResetCompleted)
	authRoutes := router.Group("/auth")
	authRoutes.Use(requireSession)
	authRoutes.GET("/me", authHandler.Me)
	authRoutes.PATCH("/me", authHandler.UpdateProfile)
	authRoutes.PUT("/me/avatar", authHandler.UpdateAvatar)
	authRoutes.GET("/me/avatar", authHandler.Avatar)
	authRoutes.DELETE("/me/avatar", authHandler.DeleteAvatar)
	return router, nil
}
