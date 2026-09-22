package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type sep10PrincipalKey struct{}

type SEP10Authenticator interface {
	Authenticate(ctx context.Context, token string) (usecase.SEP10Principal, error)
}

func RequireSEP10(authenticator SEP10Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator == nil {
			abortSEP10(c)
			return
		}
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			abortSEP10(c)
			return
		}
		principal, err := authenticator.Authenticate(c.Request.Context(), parts[1])
		if err != nil || strings.TrimSpace(principal.Account) == "" {
			abortSEP10(c)
			return
		}
		requestContext := context.WithValue(c.Request.Context(), sep10PrincipalKey{}, principal)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func SEP10Principal(ctx context.Context) (usecase.SEP10Principal, bool) {
	principal, ok := ctx.Value(sep10PrincipalKey{}).(usecase.SEP10Principal)
	return principal, ok
}

func abortSEP10(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":      "authentication failed",
		"request_id": RequestIDFromContext(c),
	})
}
