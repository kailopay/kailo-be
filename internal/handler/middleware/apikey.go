package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type APIKeyAuthenticator interface {
	Authenticate(ctx context.Context, rawKey string) (usecase.Principal, error)
}

type apiPrincipalKey struct{}

func RequireAPIKey(authenticator APIKeyAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator == nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		principal, err := authenticator.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		requestContext := context.WithValue(c.Request.Context(), apiPrincipalKey{}, principal)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func APIPrincipal(ctx context.Context) (usecase.Principal, bool) {
	principal, ok := ctx.Value(apiPrincipalKey{}).(usecase.Principal)
	return principal, ok
}
