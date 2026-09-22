package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DeveloperModeReader reports whether a session owner may use developer
// management routes.
type DeveloperModeReader interface {
	DeveloperModeEnabled(ctx context.Context, userID string) (bool, error)
}

// RequireDeveloperMode allows only authenticated users who have enabled
// Developer Mode to reach developer management routes.
func RequireDeveloperMode(reader DeveloperModeReader, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		user, authenticated := AuthenticatedUser(c.Request.Context())
		if !authenticated || strings.TrimSpace(user.User.ID) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "authentication failed",
				"request_id": RequestIDFromContext(c),
			})
			return
		}
		if reader == nil {
			logger.ErrorContext(c.Request.Context(), "developer mode reader is not configured")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":      "internal server error",
				"request_id": RequestIDFromContext(c),
			})
			return
		}
		enabled, err := reader.DeveloperModeEnabled(c.Request.Context(), user.User.ID)
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "checking developer mode failed", slog.Any("error", err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":      "internal server error",
				"request_id": RequestIDFromContext(c),
			})
			return
		}
		if !enabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":      "Developer Mode is required",
				"request_id": RequestIDFromContext(c),
			})
			return
		}
		c.Next()
	}
}
