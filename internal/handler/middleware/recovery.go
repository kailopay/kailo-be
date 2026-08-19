package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.ErrorContext(
			c.Request.Context(),
			"panic recovered",
			slog.Any("panic", recovered),
			slog.String("request_id", RequestIDFromContext(c)),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
	})
}
