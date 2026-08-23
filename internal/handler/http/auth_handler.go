package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
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
	UpdateProfile(ctx context.Context, userID string, input auth.UpdateProfileInput) (auth.UserProfile, error)
	RequestPasswordReset(ctx context.Context, email string) error
	UpdateAvatar(ctx context.Context, userID string, upload auth.AvatarUpload) (auth.UserProfile, error)
	OpenAvatar(ctx context.Context, userID string) (auth.AvatarFile, error)
	DeleteAvatar(ctx context.Context, userID string) (auth.UserProfile, error)
	CompletePasswordReset(ctx context.Context, subject string) error
}

type AuthHandler struct {
	service AuthService
	config  platform.AuthConfig
	logger  *slog.Logger
}

func NewAuthHandler(service AuthService, config platform.AuthConfig, logger *slog.Logger) *AuthHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthHandler{service: service, config: config, logger: logger}
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

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		DisplayName      string `json:"display_name"`
		DeveloperEnabled *bool  `json:"developer_enabled"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	profile, err := h.service.UpdateProfile(c.Request.Context(), user.User.ID, auth.UpdateProfileInput{
		DisplayName:      request.DisplayName,
		DeveloperEnabled: request.DeveloperEnabled,
	})
	if err != nil {
		if errors.Is(err, auth.ErrInvalidProfile) {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "updating auth profile failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile})
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if err := h.service.RequestPasswordReset(c.Request.Context(), request.Email); err != nil {
		if errors.Is(err, auth.ErrInvalidPasswordReset) {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
		h.logger.WarnContext(c.Request.Context(), "requesting Auth0 password reset failed", slog.Any("error", err))
	}
	c.Status(http.StatusAccepted)
}

func (h *AuthHandler) UpdateAvatar(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.config.AvatarMaxBytes+(1<<20))
	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > h.config.AvatarMaxBytes {
		writeRequestError(c, http.StatusRequestEntityTooLarge)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	defer file.Close()
	header := make([]byte, 512)
	headerSize, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	header = header[:headerSize]
	contentType := http.DetectContentType(header)
	profile, err := h.service.UpdateAvatar(c.Request.Context(), user.User.ID, auth.AvatarUpload{
		Body:        io.MultiReader(bytes.NewReader(header), file),
		Size:        fileHeader.Size,
		ContentType: contentType,
	})
	if err != nil {
		if errors.Is(err, auth.ErrInvalidAvatar) {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "updating auth avatar failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile})
}

func (h *AuthHandler) Avatar(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	file, err := h.service.OpenAvatar(c.Request.Context(), user.User.ID)
	if err != nil {
		if errors.Is(err, auth.ErrAvatarNotFound) {
			writeRequestError(c, http.StatusNotFound)
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "opening auth avatar failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	defer file.Body.Close()
	c.Header("Content-Type", file.ContentType)
	c.Header("Cache-Control", "private, max-age=300")
	if file.ETag != "" {
		c.Header("ETag", strconv.Quote(strings.Trim(file.ETag, `"`)))
	}
	if file.Size >= 0 {
		c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
	}
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, file.Body); err != nil {
		h.logger.ErrorContext(c.Request.Context(), "streaming auth avatar failed", slog.Any("error", err))
	}
}

func (h *AuthHandler) DeleteAvatar(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	profile, err := h.service.DeleteAvatar(c.Request.Context(), user.User.ID)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "deleting auth avatar failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile})
}

func (h *AuthHandler) PasswordResetCompleted(c *gin.Context) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	provided := sha256.Sum256([]byte(token))
	expected := sha256.Sum256([]byte(h.config.PasswordResetWebhookSecret))
	if subtle.ConstantTimeCompare(provided[:], expected[:]) != 1 {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		Subject string `json:"subject"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if err := h.service.CompletePasswordReset(c.Request.Context(), request.Subject); err != nil {
		if errors.Is(err, auth.ErrInvalidPasswordReset) {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "completing Auth0 password reset failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
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

func writeRequestError(c *gin.Context, status int) {
	c.AbortWithStatusJSON(status, gin.H{"error": http.StatusText(status)})
}

func decodeJSON(c *gin.Context, destination any, maxBytes int64) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decoding JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON body must contain one object")
	}
	return nil
}
