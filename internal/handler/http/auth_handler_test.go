package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/platform"
	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeAuthService struct {
	loginURL           string
	loginErr           error
	result             auth.SessionResult
	completeErr        error
	logoutErr          error
	logoutToken        string
	updatedProfile     auth.UserProfile
	updatedName        string
	profileErr         error
	passwordResetEmail string
	passwordResetErr   error
	avatarUpload       auth.AvatarUpload
	avatarProfile      auth.UserProfile
	avatarFile         auth.AvatarFile
	avatarErr          error
	avatarDeleted      bool
	resetSubject       string
	resetErr           error
}

func (s *fakeAuthService) BeginLogin(context.Context) (string, error) { return s.loginURL, s.loginErr }

func (s *fakeAuthService) CompleteLogin(context.Context, string, string) (auth.SessionResult, error) {
	return s.result, s.completeErr
}

func (s *fakeAuthService) Logout(_ context.Context, token string) error {
	s.logoutToken = token
	return s.logoutErr
}

func (s *fakeAuthService) Authenticate(context.Context, string) (auth.AuthenticatedUser, error) {
	return auth.AuthenticatedUser{}, nil
}

func (s *fakeAuthService) UpdateProfile(_ context.Context, _ string, input auth.UpdateProfileInput) (auth.UserProfile, error) {
	s.updatedName = input.DisplayName
	return s.updatedProfile, s.profileErr
}

func (s *fakeAuthService) RequestPasswordReset(_ context.Context, email string) error {
	s.passwordResetEmail = email
	return s.passwordResetErr
}

func (s *fakeAuthService) UpdateAvatar(_ context.Context, _ string, upload auth.AvatarUpload) (auth.UserProfile, error) {
	s.avatarUpload = upload
	return s.avatarProfile, s.avatarErr
}

func (s *fakeAuthService) OpenAvatar(context.Context, string) (auth.AvatarFile, error) {
	return s.avatarFile, s.avatarErr
}

func (s *fakeAuthService) DeleteAvatar(context.Context, string) (auth.UserProfile, error) {
	s.avatarDeleted = true
	return s.avatarProfile, s.avatarErr
}

func (s *fakeAuthService) CompletePasswordReset(_ context.Context, subject string) error {
	s.resetSubject = subject
	return s.resetErr
}

func testAuthHandler(t *testing.T, service *fakeAuthService) *AuthHandler {
	t.Helper()
	return NewAuthHandler(service, platform.AuthConfig{
		SuccessRedirectURL:         "https://app.example.com/login-complete",
		CookieName:                 "kailopay_session",
		CookieSecure:               true,
		PasswordResetWebhookSecret: "01234567890123456789012345678901",
		AvatarMaxBytes:             5 << 20,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestAuthHandlerLoginAndCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{
		loginURL: "https://tenant.example.com/authorize",
		result: auth.SessionResult{
			RawToken:  "opaque-session-token",
			User:      auth.UserProfile{ID: "user-id"},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	handler := testAuthHandler(t, service)

	router := gin.New()
	router.GET("/auth/login", handler.Login)
	router.GET("/auth/callback", handler.Callback)

	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if loginResponse.Code != http.StatusFound || loginResponse.Header().Get("Location") != service.loginURL {
		t.Fatalf("login response = %d/%q", loginResponse.Code, loginResponse.Header().Get("Location"))
	}

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, httptest.NewRequest(http.MethodGet, "/auth/callback?code=code-value&state=state-value", nil))
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != "https://app.example.com/login-complete" {
		t.Fatalf("callback response = %d/%q", callbackResponse.Code, callbackResponse.Header().Get("Location"))
	}
	setCookie := callbackResponse.Header().Get("Set-Cookie")
	for _, want := range []string{"kailopay_session=opaque-session-token", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(setCookie, want) {
			t.Errorf("Set-Cookie %q does not contain %q", setCookie, want)
		}
	}
}

func TestAuthHandlerCallbackAndLogoutSanitizeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{completeErr: errors.New("provider secret authorization code leaked")}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.GET("/auth/callback", handler.Callback)
	router.POST("/auth/logout", handler.Logout)

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, httptest.NewRequest(http.MethodGet, "/auth/callback?code=code&state=state", nil))
	if callbackResponse.Code != http.StatusBadRequest || strings.Contains(callbackResponse.Body.String(), "authorization code") {
		t.Fatalf("callback error response = %d/%q", callbackResponse.Code, callbackResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: "kailopay_session", Value: "token"})
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent || service.logoutToken != "token" {
		t.Fatalf("logout response/token = %d/%q", logoutResponse.Code, service.logoutToken)
	}
}

func TestAuthHandlerMeRequiresLocalSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{}
	handler := testAuthHandler(t, service)
	user := auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id", Email: "user@example.com"}}
	router := gin.New()
	router.Use(middleware.RequireSession(fakeSessionAuthenticator{user: user}))
	router.GET("/auth/me", handler.Me)

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}

	authenticated := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	authenticated.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, authenticated)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "user-id") {
		t.Fatalf("authenticated response = %d/%q", response.Code, response.Body.String())
	}
}

type fakeSessionAuthenticator struct{ user auth.AuthenticatedUser }

func (f fakeSessionAuthenticator) Authenticate(context.Context, string) (auth.AuthenticatedUser, error) {
	return f.user, nil
}

func TestAuthHandlerUpdatesAuthenticatedProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{updatedProfile: auth.UserProfile{ID: "user-id", DisplayName: "New Name"}}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodPatch, "/auth/me", handler.UpdateProfile)

	request := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"displayName":"New Name"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.updatedName != "New Name" || !strings.Contains(response.Body.String(), `"displayName":"New Name"`) {
		t.Fatalf("profile response/name = %d/%q/%q", response.Code, response.Body.String(), service.updatedName)
	}
}

func TestAuthHandlerForgotPasswordAlwaysReturnsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{passwordResetErr: errors.New("Auth0 user not found")}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/password/forgot", handler.ForgotPassword)

	request := httptest.NewRequest(http.MethodPost, "/auth/password/forgot", strings.NewReader(`{"email":"user@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || service.passwordResetEmail != "user@example.com" {
		t.Fatalf("forgot response/email = %d/%q", response.Code, service.passwordResetEmail)
	}
	if strings.Contains(response.Body.String(), "not found") {
		t.Fatalf("forgot response leaked provider result: %q", response.Body.String())
	}
}

func TestAuthHandlerUploadsValidatedAvatar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{avatarProfile: auth.UserProfile{ID: "user-id", AvatarURL: "/auth/me/avatar"}}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodPut, "/auth/me/avatar", handler.UpdateAvatar)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("avatar-body")...)
	if _, err := part.Write(png); err != nil {
		t.Fatalf("writing avatar: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPut, "/auth/me/avatar", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.avatarUpload.ContentType != "image/png" || service.avatarUpload.Size != int64(len(png)) {
		t.Fatalf("avatar response/type/size = %d/%q/%d", response.Code, service.avatarUpload.ContentType, service.avatarUpload.Size)
	}
}

func TestAuthHandlerStreamsPrivateAvatar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{avatarFile: auth.AvatarFile{
		Body:        io.NopCloser(strings.NewReader("avatar-bytes")),
		Size:        12,
		ContentType: "image/webp",
		ETag:        "etag-value",
	}}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodGet, "/auth/me/avatar", handler.Avatar)
	request := httptest.NewRequest(http.MethodGet, "/auth/me/avatar", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "avatar-bytes" || response.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("avatar response = %d/%q/%q", response.Code, response.Body.String(), response.Header().Get("Content-Type"))
	}
	if response.Header().Get("ETag") != `"etag-value"` {
		t.Fatalf("ETag = %q", response.Header().Get("ETag"))
	}
}

func TestAuthHandlerDeletesAuthenticatedUsersAvatar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{avatarProfile: auth.UserProfile{ID: "user-id"}}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodDelete, "/auth/me/avatar", handler.DeleteAvatar)
	request := httptest.NewRequest(http.MethodDelete, "/auth/me/avatar", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !service.avatarDeleted {
		t.Fatalf("delete avatar status/called = %d/%t", response.Code, service.avatarDeleted)
	}
}

func TestAuthHandlerPasswordResetCallbackRequiresConfiguredSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/internal/auth/password-reset-completed", handler.PasswordResetCompleted)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/auth/password-reset-completed", strings.NewReader(`{"subject":"auth0|user-1"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/internal/auth/password-reset-completed", strings.NewReader(`{"subject":"auth0|user-1"}`))
	request.Header.Set("Authorization", "Bearer 01234567890123456789012345678901")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || service.resetSubject != "auth0|user-1" {
		t.Fatalf("reset callback status/subject = %d/%q", response.Code, service.resetSubject)
	}
}

func authenticatedAuthRouter(t *testing.T, handler *AuthHandler, method, path string, route gin.HandlerFunc) *gin.Engine {
	t.Helper()
	user := auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id", Email: "user@example.com"}}
	router := gin.New()
	router.Use(middleware.RequireSession(fakeSessionAuthenticator{user: user}))
	router.Handle(method, path, route)
	return router
}
