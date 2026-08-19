package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	auth "github.com/febry3/kailopay-be/internal/usecase"
)

func TestRouterRegistersExpandedAuthEndpoints(t *testing.T) {
	service := &fakeAuthService{
		updatedProfile: auth.UserProfile{ID: "user-id", DisplayName: "Name"},
		avatarFile: auth.AvatarFile{
			Body:        io.NopCloser(strings.NewReader("avatar")),
			Size:        6,
			ContentType: "image/png",
		},
	}
	authHandler := testAuthHandler(t, service)
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	router, err := NewRouter(logger, health, authHandler, requireSession)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	tests := []struct {
		method        string
		path          string
		body          string
		contentType   string
		authenticated bool
		wantStatus    int
	}{
		{method: http.MethodPost, path: "/auth/password/forgot", body: `{"email":"user@example.com"}`, contentType: "application/json", wantStatus: http.StatusAccepted},
		{method: http.MethodPatch, path: "/auth/me", body: `{"displayName":"Name"}`, contentType: "application/json", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodPut, path: "/auth/me/avatar", contentType: "multipart/form-data", authenticated: true, wantStatus: http.StatusBadRequest},
		{method: http.MethodGet, path: "/auth/me/avatar", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodDelete, path: "/auth/me/avatar", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodPost, path: "/internal/auth/password-reset-completed", body: `{}`, contentType: "application/json", wantStatus: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			if test.authenticated {
				request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "token"})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestRouterRegistersWeek1Routes(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	router, err := NewRouter(logger, health, authHandler, requireSession,
		WithAPIKeys(NewAPIKeyHandler(&fakeAPIKeyService{}, logger)),
		WithOnramp(NewOnrampHandler(&fakeOnrampService{}, logger), middleware.RequireAPIKey(fixedAPIAuthenticator{})),
		WithXenditCallback(NewXenditCallbackHandler(&callbackServiceFake{}, logger)),
	)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /v1/api-keys", "GET /v1/api-keys", "DELETE /v1/api-keys/:id",
		"POST /v1/onramps", "GET /v1/orders", "GET /v1/orders/:id",
		"POST /callbacks/payments/xendit",
	} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}
}
