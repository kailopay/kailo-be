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
	"github.com/gin-gonic/gin"
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
		{method: http.MethodPost, path: "/auth/register", body: `{"email":"user@example.com","password":"super-secret-1"}`, contentType: "application/json", wantStatus: http.StatusCreated},
		{method: http.MethodPost, path: "/auth/login", body: `{"email":"user@example.com","password":"super-secret-1"}`, contentType: "application/json", wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/auth/google/login", wantStatus: http.StatusFound},
		{method: http.MethodPost, path: "/auth/email/verify", body: `{"token":"token"}`, contentType: "application/json", wantStatus: http.StatusOK},
		{method: http.MethodPost, path: "/auth/email/resend", body: `{"email":"user@example.com"}`, contentType: "application/json", wantStatus: http.StatusAccepted},
		{method: http.MethodPost, path: "/auth/password/forgot", body: `{"email":"user@example.com"}`, contentType: "application/json", wantStatus: http.StatusAccepted},
		{method: http.MethodPost, path: "/auth/password/reset", body: `{"token":"token","new_password":"fresh-password-1"}`, contentType: "application/json", wantStatus: http.StatusNoContent},
		{method: http.MethodPost, path: "/auth/password/change", body: `{"current_password":"old-password-00","new_password":"fresh-password-1"}`, contentType: "application/json", authenticated: true, wantStatus: http.StatusNoContent},
		{method: http.MethodPatch, path: "/auth/me", body: `{"display_name":"Name"}`, contentType: "application/json", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodPut, path: "/auth/me/avatar", contentType: "multipart/form-data", authenticated: true, wantStatus: http.StatusBadRequest},
		{method: http.MethodGet, path: "/auth/me/avatar", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodDelete, path: "/auth/me/avatar", authenticated: true, wantStatus: http.StatusOK},
		{method: http.MethodGet, path: "/internal/auth/password-reset-completed", wantStatus: http.StatusNotFound},
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

func TestRouterRegistersSEP10AuthEndpoints(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	sep10Handler := NewSEP10Handler(sep10HandlerServiceFake{}, logger)
	router, err := NewRouter(logger, health, authHandler, requireSession, WithSEP10(sep10Handler))
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{"GET /auth", "POST /auth"} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}
}

func TestRouterRegistersWeek1Routes(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	router, err := NewRouter(logger, health, authHandler, requireSession,
		WithAPIKeys(NewAPIKeyHandler(&fakeAPIKeyService{}, logger)),
		WithOnramp(NewOnrampHandler(&fakeOnrampService{}, logger), middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil)),
		WithOfframp(NewOfframpHandler(&fakeOfframpHandlerService{}, logger)),
		WithQuote(NewQuoteHandler(&fakeQuoteService{}, logger), middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil)),
		WithSep38(NewSep38Handler(&sep38HandlerServiceFake{}, logger)),
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
		"POST /v1/api-keys", "GET /v1/api-keys", "DELETE /v1/api-keys/:id", "POST /v1/offramps",
		"POST /v1/onramps", "POST /v1/quotes", "GET /v1/orders", "GET /v1/orders/:id",
		"GET /sep38/info", "GET /sep38/prices", "GET /sep38/price",
		"POST /callbacks/payments/xendit",
	} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/quotes", strings.NewReader(`{"direction":"buy","fiat":{"currency":"IDR","amount_minor":"100000"},"asset":{"network":"stellar_testnet","code":"XLM"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("public quote route status = %d, want %d; body = %q", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestRouterRegistersDeveloperDashboardRoutes(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	service := &developerDashboardServiceSpy{}
	router, err := NewRouter(logger, health, authHandler, requireSession,
		WithDeveloperDashboard(NewDeveloperDashboardHandler(service, logger)),
		WithDeveloperWallet(NewDeveloperWalletHandler(&developerWalletServiceSpy{}, logger)),
	)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /v1/developer/overview",
		"GET /v1/developer/analytics",
		"GET /v1/developer/revenue/summary",
		"GET /v1/developer/revenue/entries",
		"GET /v1/developer/orders",
		"GET /v1/developer/wallets",
		"POST /v1/developer/wallets",
		"PATCH /v1/developer/wallets/:id",
		"DELETE /v1/developer/wallets/:id",
	} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}
}

func TestRouterRegistersKYCRoutes(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	router, err := NewRouter(logger, health, authHandler, requireSession, WithKYC(NewKYCHandler(&fakeKYCService{}, logger)))
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /v1/kyc", "POST /v1/kyc/inquiry", "POST /callbacks/kyc/persona",
	} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}
}

func TestRouterServesOpenAPIDocumentation(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	router, err := NewRouter(logger, health, authHandler, requireSession)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	tests := []struct {
		name         string
		path         string
		contentType  string
		bodyContains string
	}{
		{name: "swagger ui", path: "/docs/", contentType: "text/html", bodyContains: `id="swagger-ui"`},
		{name: "swagger sandbox notice", path: "/docs/", contentType: "text/html", bodyContains: "Complete approved Persona KYC"},
		{name: "openapi document", path: "/docs/openapi.yaml", contentType: "text/yaml", bodyContains: "openapi: 3.0.3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %q", response.Code, http.StatusOK, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, test.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", contentType, test.contentType)
			}
			if !strings.Contains(response.Body.String(), test.bodyContains) {
				t.Fatalf("body does not contain %q", test.bodyContains)
			}
		})
	}
}

func TestRouterAcceptsRetailSessionForOrderCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{}
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-1"}}})
	orderMiddleware := middleware.RequireOrderPrincipal(nil, fakeSessionAuthenticator{user: auth.AuthenticatedUser{
		User: auth.UserProfile{ID: "user-1", EmailVerified: true}, SessionID: "session-1",
	}}, middleware.DefaultSessionCookieName, []string{"http://localhost:3000"})
	router, err := NewRouter(logger, health, authHandler, requireSession,
		WithOnramp(NewOnrampHandler(service, logger), orderMiddleware))
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"xendit","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", "router-retail-1")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.command.Principal.Kind != auth.OrderPrincipalRetailSession {
		t.Fatalf("status = %d, principal = %+v, body = %q", response.Code, service.command.Principal, response.Body.String())
	}
}

func TestRouterRegistersAuthenticatedSEP24Routes(t *testing.T) {
	authHandler := testAuthHandler(t, &fakeAuthService{})
	health := NewHealthHandler(func(context.Context) error { return nil }, time.Second, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	requireSession := middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}}})
	orderMiddleware := middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil)
	sep24 := NewSep24Handler(&sep24HandlerServiceFake{}, Sep24Config{TransferServerURL: "https://anchor.example/sep24"}, logger)
	router, err := NewRouter(logger, health, authHandler, requireSession, WithSep24(sep24, orderMiddleware))
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /sep24/transactions/deposit/interactive",
		"POST /sep24/transactions/withdraw/interactive",
		"GET /sep24/transactions",
		"GET /sep24/transaction",
		"GET /sep24/interactive/:id",
		"POST /sep24/deposit",
		"POST /sep24/withdraw",
	} {
		if !routes[route] {
			t.Errorf("missing route %s", route)
		}
	}

	infoResponse := httptest.NewRecorder()
	router.ServeHTTP(infoResponse, httptest.NewRequest(http.MethodGet, "/sep24/info", nil))
	if infoResponse.Code != http.StatusOK {
		t.Fatalf("info status = %d, want %d", infoResponse.Code, http.StatusOK)
	}
	transactionResponse := httptest.NewRecorder()
	router.ServeHTTP(transactionResponse, httptest.NewRequest(http.MethodGet, "/sep24/transaction?id=tx-1", nil))
	if transactionResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated transaction status = %d, want %d", transactionResponse.Code, http.StatusUnauthorized)
	}
}
