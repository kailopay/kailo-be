package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func Logging(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		attrs := []any{
			slog.String("request_id", RequestIDFromContext(c)),
			slog.String("method", c.Request.Method),
			slog.String("route", routeTemplate(c)),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("duration", time.Since(started)),
		}
		switch {
		case c.Writer.Status() >= 500:
			logger.ErrorContext(c.Request.Context(), "http request completed", attrs...)
		case c.Writer.Status() >= 400:
			logger.WarnContext(c.Request.Context(), "http request completed", attrs...)
		default:
			logger.DebugContext(c.Request.Context(), "http request completed", attrs...)
		}
	}
}

func routeTemplate(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "unknown"
}
