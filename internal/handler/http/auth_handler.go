package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/platform"
	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type AuthService interface {
	BeginLogin(ctx context.Context) (string, error)
	CompleteLogin(ctx context.Context, code, state string) (auth.SessionResult, error)
	Logout(ctx context.Context, rawToken string) error
	Authenticate(ctx context.Context, rawToken string) (auth.AuthenticatedUser, error)
}

type AuthHandler struct {
	service AuthService
	config  platform.AuthConfig
}

func NewAuthHandler(service AuthService, config platform.AuthConfig) *AuthHandler {
	return &AuthHandler{service: service, config: config}
}

func (h *AuthHandler) Login(c *gin.Context) {
	redirectURL, err := h.service.BeginLogin(c.Request.Context())
	if err != nil {
		writeAuthError(c, http.StatusServiceUnavailable)
		return
	}
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" || c.Query("error") != "" {
		writeAuthError(c, http.StatusBadRequest)
		return
	}
	result, err := h.service.CompleteLogin(c.Request.Context(), code, state)
	if err != nil {
		writeAuthError(c, http.StatusBadRequest)
		return
	}
	h.setSessionCookie(c, result.RawToken, result.ExpiresAt)
	c.Redirect(http.StatusFound, h.config.SuccessRedirectURL)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	rawToken, _ := c.Cookie(h.config.CookieName)
	if err := h.service.Logout(c.Request.Context(), rawToken); err != nil {
		writeAuthError(c, http.StatusInternalServerError)
		return
	}
	h.clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) Me(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user.User})
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, value string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func writeAuthError(c *gin.Context, status int) {
	c.AbortWithStatusJSON(status, gin.H{"error": "authentication failed"})
}
