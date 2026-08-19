package httpapi

import (
	"context"
	"errors"
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
	loginURL    string
	loginErr    error
	result      auth.SessionResult
	completeErr error
	logoutErr   error
	logoutToken string
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

func testAuthHandler(t *testing.T, service *fakeAuthService) *AuthHandler {
	t.Helper()
	return NewAuthHandler(service, platform.AuthConfig{
		SuccessRedirectURL: "https://app.example.com/login-complete",
		CookieName:         "kailopay_session",
		CookieSecure:       true,
	})
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
