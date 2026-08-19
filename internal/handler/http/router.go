package httpapi

import (
	"errors"
	"log/slog"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/gin-gonic/gin"
)

func NewRouter(logger *slog.Logger, health *HealthHandler) (*gin.Engine, error) {
	if health == nil {
		return nil, errors.New("health handler is required")
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
	return router, nil
}
