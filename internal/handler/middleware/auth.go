package middleware

import (
	"context"
	"net/http"
	"strings"

	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

const DefaultSessionCookieName = "kailopay_session"

type SessionAuthenticator interface {
	Authenticate(ctx context.Context, rawToken string) (auth.AuthenticatedUser, error)
}

type authenticatedUserKey struct{}

func RequireSession(authenticator SessionAuthenticator) gin.HandlerFunc {
	return RequireSessionWithCookie(authenticator, DefaultSessionCookieName)
}

func RequireSessionWithCookie(authenticator SessionAuthenticator, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator == nil || strings.TrimSpace(cookieName) == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		rawToken, err := c.Cookie(cookieName)
		if err != nil || strings.TrimSpace(rawToken) == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		user, err := authenticator.Authenticate(c.Request.Context(), rawToken)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		requestContext := context.WithValue(c.Request.Context(), authenticatedUserKey{}, user)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func AuthenticatedUser(ctx context.Context) (auth.AuthenticatedUser, bool) {
	user, ok := ctx.Value(authenticatedUserKey{}).(auth.AuthenticatedUser)
	return user, ok
}
