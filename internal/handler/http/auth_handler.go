package httpapi

import (
	"bytes"
	"context"
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
	Register(ctx context.Context, input auth.RegisterInput) (auth.UserProfile, error)
	LoginWithPassword(ctx context.Context, email, password string) (auth.SessionResult, error)
	GoogleAvailable() bool
	BeginGoogleLogin(ctx context.Context) (string, error)
	CompleteGoogleLogin(ctx context.Context, code, state string) (auth.SessionResult, error)
	Logout(ctx context.Context, rawToken string) error
	Authenticate(ctx context.Context, rawToken string) (auth.AuthenticatedUser, error)
	VerifyEmail(ctx context.Context, token string) (auth.UserProfile, error)
	ResendVerification(ctx context.Context, email string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error
	UpdateProfile(ctx context.Context, userID string, input auth.UpdateProfileInput) (auth.UserProfile, error)
	UpdateAvatar(ctx context.Context, userID string, upload auth.AvatarUpload) (auth.UserProfile, error)
	OpenAvatar(ctx context.Context, userID string) (auth.AvatarFile, error)
	DeleteAvatar(ctx context.Context, userID string) (auth.UserProfile, error)
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

func (h *AuthHandler) Register(c *gin.Context) {
	var request struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	profile, err := h.service.Register(c.Request.Context(), auth.RegisterInput{
		Email:       request.Email,
		Password:    request.Password,
		DisplayName: request.DisplayName,
	})
	if err != nil {
		h.writeError(c, "registering account", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": profile})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	result, err := h.service.LoginWithPassword(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		h.writeError(c, "logging in with password", err)
		return
	}
	h.setSessionCookie(c, result.RawToken, result.ExpiresAt)
	c.JSON(http.StatusOK, gin.H{"user": result.User})
}

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	redirectURL, err := h.service.BeginGoogleLogin(c.Request.Context())
	if err != nil {
		h.writeError(c, "starting google login", err)
		return
	}
	c.Redirect(http.StatusFound, redirectURL)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	providerError := c.Query("error")
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" || providerError != "" {
		h.logger.WarnContext(c.Request.Context(), "google callback rejected before processing",
			slog.String("reason", "missing_code_or_state"),
			slog.String("provider_error", providerError))
		writeAuthError(c, http.StatusBadRequest)
		return
	}
	result, err := h.service.CompleteGoogleLogin(c.Request.Context(), code, state)
	if err != nil {
		h.logger.WarnContext(c.Request.Context(), "completing google login failed",
			slog.String("reason", googleLoginFailureReason(err)),
			slog.Any("error", err))
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

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	profile, err := h.service.VerifyEmail(c.Request.Context(), request.Token)
	if err != nil {
		h.writeError(c, "verifying email", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": profile})
}

func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var request struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if err := h.service.ResendVerification(c.Request.Context(), request.Email); err != nil {
		if errors.Is(err, auth.ErrInvalidChallenge) {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
		// Delivery issues are logged inside the usecase boundary contract;
		// never reveal whether the address exists.
		h.logger.ErrorContext(c.Request.Context(), "resending verification email failed", slog.Any("error", err))
	}
	c.Status(http.StatusAccepted)
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
		h.logger.ErrorContext(c.Request.Context(), "requesting password reset failed", slog.Any("error", err))
	}
	c.Status(http.StatusAccepted)
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var request struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if err := h.service.ResetPassword(c.Request.Context(), request.Token, request.NewPassword); err != nil {
		h.writeError(c, "resetting password", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	if err := h.service.ChangePassword(c.Request.Context(), user.User.ID, request.CurrentPassword, request.NewPassword); err != nil {
		h.writeError(c, "changing password", err)
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

func (h *AuthHandler) writeError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
	case errors.Is(err, auth.ErrEmailNotVerified):
		c.JSON(http.StatusForbidden, gin.H{"error": "email is not verified"})
	case errors.Is(err, auth.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "email is already registered"})
	case errors.Is(err, auth.ErrWeakPassword):
		writeRequestError(c, http.StatusBadRequest)
	case errors.Is(err, auth.ErrInvalidChallenge):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
	case errors.Is(err, auth.ErrGoogleNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "google sign-in is not configured"})
	case errors.Is(err, auth.ErrInvalidProfile):
		writeRequestError(c, http.StatusBadRequest)
	case errors.Is(err, auth.ErrUserDisabled):
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
	default:
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
	}
}

func googleLoginFailureReason(err error) string {
	switch {
	case errors.Is(err, auth.ErrInvalidTransaction):
		return "login_transaction_missing_expired_or_reused"
	case errors.Is(err, auth.ErrInvalidIdentity):
		return "identity_verification_failed"
	case errors.Is(err, auth.ErrProvider):
		return "provider_exchange_failed"
	case errors.Is(err, auth.ErrGoogleNotConfigured):
		return "google_not_configured"
	default:
		return "internal"
	}
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
