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
	registerInput      auth.RegisterInput
	registerProfile    auth.UserProfile
	registerErr        error
	loginEmail         string
	loginPassword      string
	result             auth.SessionResult
	loginErr           error
	googleAvailable    bool
	googleLoginURL     string
	googleLoginErr     error
	googleCompleteErr  error
	logoutErr          error
	logoutToken        string
	verifiedToken      string
	verifyProfile      auth.UserProfile
	verifyErr          error
	resendEmail        string
	resendErr          error
	resetToken         string
	resetPassword      string
	resetErr           error
	changeCurrent      string
	changeNew          string
	changeErr          error
	updatedProfile     auth.UserProfile
	updatedName        string
	developerEnabled   *bool
	profileErr         error
	passwordResetEmail string
	passwordResetErr   error
	avatarUpload       auth.AvatarUpload
	avatarProfile      auth.UserProfile
	avatarFile         auth.AvatarFile
	avatarErr          error
	avatarDeleted      bool
}

func (s *fakeAuthService) Register(_ context.Context, input auth.RegisterInput) (auth.UserProfile, error) {
	s.registerInput = input
	return s.registerProfile, s.registerErr
}

func (s *fakeAuthService) LoginWithPassword(_ context.Context, email, password string) (auth.SessionResult, error) {
	s.loginEmail, s.loginPassword = email, password
	return s.result, s.loginErr
}

func (s *fakeAuthService) GoogleAvailable() bool { return s.googleAvailable }

func (s *fakeAuthService) BeginGoogleLogin(context.Context) (string, error) {
	return s.googleLoginURL, s.googleLoginErr
}

func (s *fakeAuthService) CompleteGoogleLogin(context.Context, string, string) (auth.SessionResult, error) {
	return s.result, s.googleCompleteErr
}

func (s *fakeAuthService) Logout(_ context.Context, token string) error {
	s.logoutToken = token
	return s.logoutErr
}

func (s *fakeAuthService) Authenticate(context.Context, string) (auth.AuthenticatedUser, error) {
	return auth.AuthenticatedUser{}, nil
}

func (s *fakeAuthService) VerifyEmail(_ context.Context, token string) (auth.UserProfile, error) {
	s.verifiedToken = token
	return s.verifyProfile, s.verifyErr
}

func (s *fakeAuthService) ResendVerification(_ context.Context, email string) error {
	s.resendEmail = email
	return s.resendErr
}

func (s *fakeAuthService) RequestPasswordReset(_ context.Context, email string) error {
	s.passwordResetEmail = email
	return s.passwordResetErr
}

func (s *fakeAuthService) ResetPassword(_ context.Context, token, newPassword string) error {
	s.resetToken, s.resetPassword = token, newPassword
	return s.resetErr
}

func (s *fakeAuthService) ChangePassword(_ context.Context, _, currentPassword, newPassword string) error {
	s.changeCurrent, s.changeNew = currentPassword, newPassword
	return s.changeErr
}

func (s *fakeAuthService) UpdateProfile(_ context.Context, _ string, input auth.UpdateProfileInput) (auth.UserProfile, error) {
	s.updatedName = input.DisplayName
	s.developerEnabled = input.DeveloperEnabled
	return s.updatedProfile, s.profileErr
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

func testAuthHandler(t *testing.T, service *fakeAuthService) *AuthHandler {
	t.Helper()
	return NewAuthHandler(service, platform.AuthConfig{
		SuccessRedirectURL: "https://app.example.com/login-complete",
		CookieName:         "kailopay_session",
		CookieSecure:       true,
		AvatarMaxBytes:     5 << 20,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func jsonRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestAuthHandlerRegisterAndLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{
		registerProfile: auth.UserProfile{ID: "user-id", Email: "user@example.com"},
		result: auth.SessionResult{
			RawToken:  "opaque-session-token",
			User:      auth.UserProfile{ID: "user-id"},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/register", handler.Register)
	router.POST("/auth/login", handler.Login)

	registerResponse := httptest.NewRecorder()
	router.ServeHTTP(registerResponse, jsonRequest(http.MethodPost, "/auth/register", `{"email":"user@example.com","password":"super-secret-1","display_name":"User"}`))
	if registerResponse.Code != http.StatusCreated || service.registerInput.Email != "user@example.com" {
		t.Fatalf("register response/input = %d/%+v", registerResponse.Code, service.registerInput)
	}

	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, jsonRequest(http.MethodPost, "/auth/login", `{"email":"user@example.com","password":"super-secret-1"}`))
	if loginResponse.Code != http.StatusOK || service.loginPassword != "super-secret-1" {
		t.Fatalf("login response/password = %d/%q", loginResponse.Code, service.loginPassword)
	}
	setCookie := loginResponse.Header().Get("Set-Cookie")
	for _, want := range []string{"kailopay_session=opaque-session-token", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(setCookie, want) {
			t.Errorf("Set-Cookie %q does not contain %q", setCookie, want)
		}
	}
}

func TestAuthHandlerLoginErrorsDoNotLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{loginErr: auth.ErrInvalidCredentials}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/login", handler.Login)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/login", `{"email":"user@example.com","password":"wrong-password-9"}`))
	if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"invalid email or password\"}" {
		t.Fatalf("login error response = %d/%q", response.Code, response.Body.String())
	}

	unverified := httptest.NewRecorder()
	service.loginErr = auth.ErrEmailNotVerified
	router.ServeHTTP(unverified, jsonRequest(http.MethodPost, "/auth/login", `{"email":"user@example.com","password":"super-secret-1"}`))
	if unverified.Code != http.StatusForbidden || !strings.Contains(unverified.Body.String(), "not verified") {
		t.Fatalf("unverified response = %d/%q", unverified.Code, unverified.Body.String())
	}
}

func TestAuthHandlerGoogleLoginAndCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{
		googleLoginURL: "https://accounts.google.com/o/oauth2/v2/auth",
		result: auth.SessionResult{
			RawToken:  "opaque-session-token",
			User:      auth.UserProfile{ID: "user-id"},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.GET("/auth/google/login", handler.GoogleLogin)
	router.GET("/auth/google/callback", handler.GoogleCallback)

	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, httptest.NewRequest(http.MethodGet, "/auth/google/login", nil))
	if loginResponse.Code != http.StatusFound || loginResponse.Header().Get("Location") != service.googleLoginURL {
		t.Fatalf("google login response = %d/%q", loginResponse.Code, loginResponse.Header().Get("Location"))
	}

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=code-value&state=state-value", nil))
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != "https://app.example.com/login-complete" {
		t.Fatalf("google callback response = %d/%q", callbackResponse.Code, callbackResponse.Header().Get("Location"))
	}
	if !strings.Contains(callbackResponse.Header().Get("Set-Cookie"), "kailopay_session=opaque-session-token") {
		t.Fatal("google callback did not set the session cookie")
	}
}

func TestAuthHandlerGoogleCallbackAndLogoutSanitizeErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{googleCompleteErr: errors.New("provider secret authorization code leaked")}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.GET("/auth/google/callback", handler.GoogleCallback)
	router.POST("/auth/logout", handler.Logout)

	callbackResponse := httptest.NewRecorder()
	router.ServeHTTP(callbackResponse, httptest.NewRequest(http.MethodGet, "/auth/google/callback?code=code&state=state", nil))
	if callbackResponse.Code != http.StatusBadRequest || strings.Contains(callbackResponse.Body.String(), "authorization code") {
		t.Fatalf("google callback error response = %d/%q", callbackResponse.Code, callbackResponse.Body.String())
	}

	logoutRequest := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logoutRequest.AddCookie(&http.Cookie{Name: "kailopay_session", Value: "token"})
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusNoContent || service.logoutToken != "token" {
		t.Fatalf("logout response/token = %d/%q", logoutResponse.Code, service.logoutToken)
	}
}

func TestAuthHandlerGoogleNotConfiguredReportsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{googleLoginErr: auth.ErrGoogleNotConfigured}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.GET("/auth/google/login", handler.GoogleLogin)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/auth/google/login", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("google unavailable status = %d", response.Code)
	}
}

func TestAuthHandlerVerifyEmailAndResend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{verifyProfile: auth.UserProfile{ID: "user-id", EmailVerified: true}}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/email/verify", handler.VerifyEmail)
	router.POST("/auth/email/resend", handler.ResendVerification)

	verifyResponse := httptest.NewRecorder()
	router.ServeHTTP(verifyResponse, jsonRequest(http.MethodPost, "/auth/email/verify", `{"token":"verification-token"}`))
	if verifyResponse.Code != http.StatusOK || service.verifiedToken != "verification-token" {
		t.Fatalf("verify response/token = %d/%q", verifyResponse.Code, service.verifiedToken)
	}

	invalid := httptest.NewRecorder()
	service.verifyErr = auth.ErrInvalidChallenge
	router.ServeHTTP(invalid, jsonRequest(http.MethodPost, "/auth/email/verify", `{"token":"used"}`))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid or expired token") {
		t.Fatalf("invalid verify response = %d/%q", invalid.Code, invalid.Body.String())
	}

	resendResponse := httptest.NewRecorder()
	router.ServeHTTP(resendResponse, jsonRequest(http.MethodPost, "/auth/email/resend", `{"email":"user@example.com"}`))
	if resendResponse.Code != http.StatusAccepted || service.resendEmail != "user@example.com" {
		t.Fatalf("resend response/email = %d/%q", resendResponse.Code, service.resendEmail)
	}
}

func TestAuthHandlerPasswordResetFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/password/forgot", handler.ForgotPassword)
	router.POST("/auth/password/reset", handler.ResetPassword)

	forgotResponse := httptest.NewRecorder()
	router.ServeHTTP(forgotResponse, jsonRequest(http.MethodPost, "/auth/password/forgot", `{"email":"user@example.com"}`))
	if forgotResponse.Code != http.StatusAccepted || service.passwordResetEmail != "user@example.com" {
		t.Fatalf("forgot response/email = %d/%q", forgotResponse.Code, service.passwordResetEmail)
	}

	resetResponse := httptest.NewRecorder()
	router.ServeHTTP(resetResponse, jsonRequest(http.MethodPost, "/auth/password/reset", `{"token":"reset-token","new_password":"fresh-password-1"}`))
	if resetResponse.Code != http.StatusNoContent || service.resetToken != "reset-token" || service.resetPassword != "fresh-password-1" {
		t.Fatalf("reset response/token/password = %d/%q/%q", resetResponse.Code, service.resetToken, service.resetPassword)
	}
}

func TestAuthHandlerChangePasswordRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodPost, "/auth/password/change", handler.ChangePassword)

	response := httptest.NewRecorder()
	changeRequest := jsonRequest(http.MethodPost, "/auth/password/change", `{"current_password":"old-password-00","new_password":"fresh-password-1"}`)
	changeRequest.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	router.ServeHTTP(response, changeRequest)
	if response.Code != http.StatusNoContent || service.changeCurrent != "old-password-00" {
		t.Fatalf("change response/current = %d/%q", response.Code, service.changeCurrent)
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

	request := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"display_name":"New Name"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.updatedName != "New Name" || !strings.Contains(response.Body.String(), `"display_name":"New Name"`) {
		t.Fatalf("profile response/name = %d/%q/%q", response.Code, response.Body.String(), service.updatedName)
	}
}

func TestAuthHandlerEnablesDeveloperMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{updatedProfile: auth.UserProfile{ID: "user-id", DeveloperEnabled: true}}
	handler := testAuthHandler(t, service)
	router := authenticatedAuthRouter(t, handler, http.MethodPatch, "/auth/me", handler.UpdateProfile)

	request := httptest.NewRequest(http.MethodPatch, "/auth/me", strings.NewReader(`{"developer_enabled":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.developerEnabled == nil || !*service.developerEnabled {
		t.Fatalf("status = %d, developer mode = %v, body = %q", response.Code, service.developerEnabled, response.Body.String())
	}
}

func TestAuthHandlerForgotPasswordAlwaysReturnsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAuthService{passwordResetErr: errors.New("unknown account")}
	handler := testAuthHandler(t, service)
	router := gin.New()
	router.POST("/auth/password/forgot", handler.ForgotPassword)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, jsonRequest(http.MethodPost, "/auth/password/forgot", `{"email":"user@example.com"}`))
	if response.Code != http.StatusAccepted || service.passwordResetEmail != "user@example.com" {
		t.Fatalf("forgot response/email = %d/%q", response.Code, service.passwordResetEmail)
	}
	if strings.Contains(response.Body.String(), "unknown account") {
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

func authenticatedAuthRouter(t *testing.T, handler *AuthHandler, method, path string, route gin.HandlerFunc) *gin.Engine {
	t.Helper()
	user := auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id", Email: "user@example.com"}}
	router := gin.New()
	router.Use(middleware.RequireSession(fakeSessionAuthenticator{user: user}))
	router.Handle(method, path, route)
	return router
}
