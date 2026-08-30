package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type orderPrincipalKey struct{}

// RequireOrderPrincipal authenticates either an API key or the retail session
// cookie. A present Authorization header has precedence over the cookie so
// developer requests continue to work when sent from a signed-in browser.
func RequireOrderPrincipal(apiAuthenticator APIKeyAuthenticator, sessionAuthenticator SessionAuthenticator, cookieName string, allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		request := c.Request
		authorizationValues := request.Header.Values("Authorization")
		if len(authorizationValues) > 0 {
			if apiAuthenticator == nil {
				abortOrderAuthentication(c, http.StatusUnauthorized)
				return
			}
			if len(authorizationValues) != 1 {
				abortOrderAuthentication(c, http.StatusUnauthorized)
				return
			}
			parts := strings.Fields(authorizationValues[0])
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				abortOrderAuthentication(c, http.StatusUnauthorized)
				return
			}
			principal, err := apiAuthenticator.Authenticate(request.Context(), parts[1])
			if err != nil {
				abortOrderAuthentication(c, http.StatusUnauthorized)
				return
			}
			orderPrincipal, err := usecase.NewAPIClientOrderPrincipal(principal)
			if err != nil {
				abortOrderAuthentication(c, http.StatusUnauthorized)
				return
			}
			setOrderPrincipal(c, orderPrincipal)
			c.Next()
			return
		}

		if sessionAuthenticator == nil || strings.TrimSpace(cookieName) == "" {
			abortOrderAuthentication(c, http.StatusUnauthorized)
			return
		}
		rawToken, err := c.Cookie(cookieName)
		if err != nil || strings.TrimSpace(rawToken) == "" {
			abortOrderAuthentication(c, http.StatusUnauthorized)
			return
		}
		user, err := sessionAuthenticator.Authenticate(request.Context(), rawToken)
		if err != nil {
			abortOrderAuthentication(c, http.StatusUnauthorized)
			return
		}
		orderPrincipal, err := usecase.NewRetailSessionOrderPrincipal(user)
		if err != nil {
			abortOrderAuthentication(c, http.StatusForbidden)
			return
		}
		if request.Method == http.MethodPost && !allowedBrowserOrigin(request, allowedOrigins) {
			abortOrderOrigin(c)
			return
		}
		setOrderPrincipal(c, orderPrincipal)
		c.Next()
	}
}

func abortOrderAuthentication(c *gin.Context, status int) {
	c.AbortWithStatusJSON(status, gin.H{
		"error":      "authentication failed",
		"request_id": RequestIDFromContext(c),
	})
}

func abortOrderOrigin(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error":      "origin is not allowed for this session mutation",
		"request_id": RequestIDFromContext(c),
	})
}

func setOrderPrincipal(c *gin.Context, principal usecase.OrderPrincipal) {
	requestContext := context.WithValue(c.Request.Context(), orderPrincipalKey{}, principal)
	c.Request = c.Request.WithContext(requestContext)
}

func OrderPrincipal(ctx context.Context) (usecase.OrderPrincipal, bool) {
	principal, ok := ctx.Value(orderPrincipalKey{}).(usecase.OrderPrincipal)
	return principal, ok
}

func allowedBrowserOrigin(request *http.Request, allowedOrigins []string) bool {
	if origin := strings.TrimSpace(request.Header.Get("Origin")); origin != "" {
		return matchesAllowedOrigin(origin, allowedOrigins, false)
	}
	if referer := strings.TrimSpace(request.Header.Get("Referer")); referer != "" {
		return matchesAllowedOrigin(referer, allowedOrigins, true)
	}
	return false
}

func matchesAllowedOrigin(raw string, allowedOrigins []string, allowPath bool) bool {
	origin, ok := normalizedOrigin(raw, allowPath)
	if !ok {
		return false
	}
	for _, allowed := range allowedOrigins {
		configured, configuredOK := normalizedOrigin(allowed, false)
		if configuredOK && origin == configured {
			return true
		}
	}
	return false
}

func normalizedOrigin(raw string, allowPath bool) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if !allowPath && parsed.Path != "" {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}
