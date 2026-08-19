package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type ReadyCheck func(context.Context) error

type HealthHandler struct {
	ready        ReadyCheck
	checkTimeout time.Duration
	logger       *slog.Logger
	started      atomic.Bool
}

func NewHealthHandler(ready ReadyCheck, checkTimeout time.Duration, logger *slog.Logger) *HealthHandler {
	if checkTimeout <= 0 {
		checkTimeout = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	handler := &HealthHandler{
		ready:        ready,
		checkTimeout: checkTimeout,
		logger:       logger,
	}
	return handler
}

// MarkStarted marks the process as initialized and ready to receive traffic.
// Call it only after required startup dependencies have been initialized.
func (h *HealthHandler) MarkStarted() {
	h.started.Store(true)
}

// Liveness reports whether the process is running. It must not call external
// dependencies so a database outage does not trigger a restart loop.
func (h *HealthHandler) Liveness(c *gin.Context) {
	h.writeStatus(c, http.StatusOK, "ok")
}

// Readiness reports whether the process can serve requests that require its
// configured dependencies.
func (h *HealthHandler) Readiness(c *gin.Context) {
	if h.ready == nil {
		h.writeStatus(c, http.StatusServiceUnavailable, "not_ready")
		return
	}

	checkCtx, cancel := context.WithTimeout(c.Request.Context(), h.checkTimeout)
	defer cancel()
	if err := h.ready(checkCtx); err != nil {
		h.logger.WarnContext(
			checkCtx,
			"readiness check failed",
			slog.Any("error", err),
			slog.String("request_id", requestID(c)),
		)
		h.writeStatus(c, http.StatusServiceUnavailable, "not_ready")
		return
	}
	h.writeStatus(c, http.StatusOK, "ready")
}

// Startup reports whether the process completed its one-time initialization.
// It is intentionally independent from readiness so orchestrators can restart
// a process that never finished startup without coupling that decision to a
// temporary dependency outage.
func (h *HealthHandler) Startup(c *gin.Context) {
	if !h.started.Load() {
		h.writeStatus(c, http.StatusServiceUnavailable, "starting")
		return
	}
	h.writeStatus(c, http.StatusOK, "started")
}

// Health and Ready preserve the original handler names for callers that used
// the initial bootstrap API.
func (h *HealthHandler) Health(c *gin.Context) { h.Liveness(c) }

func (h *HealthHandler) Ready(c *gin.Context) { h.Readiness(c) }

func (h *HealthHandler) writeStatus(c *gin.Context, statusCode int, status string) {
	c.Header("Cache-Control", "no-store")
	c.JSON(statusCode, gin.H{"status": status})
}

func requestID(c *gin.Context) string {
	value, ok := c.Get("request_id")
	if !ok {
		return ""
	}
	id, ok := value.(string)
	if !ok {
		return ""
	}
	return id
}
