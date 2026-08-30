package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type orderAPIAuthenticatorFake struct {
	principal usecase.Principal
	err       error
	calls     int
}

func (f *orderAPIAuthenticatorFake) Authenticate(_ context.Context, _ string) (usecase.Principal, error) {
	f.calls++
	return f.principal, f.err
}

type orderSessionAuthenticatorFake struct {
	user  usecase.AuthenticatedUser
	err   error
	calls int
}

func (f *orderSessionAuthenticatorFake) Authenticate(_ context.Context, _ string) (usecase.AuthenticatedUser, error) {
	f.calls++
	return f.user, f.err
}

func TestRequireOrderPrincipalUsesBearerKeyWhenBothCredentialsExist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := &orderAPIAuthenticatorFake{principal: usecase.Principal{ClientID: "client-1", OwnerUserID: "owner-1"}}
	session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
	router := gin.New()
	router.Use(RequireOrderPrincipal(api, session, DefaultSessionCookieName, []string{"https://app.example"}))
	router.GET("/orders", func(c *gin.Context) {
		principal, ok := OrderPrincipal(c.Request.Context())
		if !ok || principal.Kind != usecase.OrderPrincipalAPIClient || principal.ClientID != "client-1" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/orders", nil)
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if api.calls != 1 || session.calls != 0 {
		t.Fatalf("authenticator calls = api %d, session %d; want api 1, session 0", api.calls, session.calls)
	}
}

func TestRequireOrderPrincipalDoesNotFallBackFromPresentEmptyAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
	router := gin.New()
	router.Use(RequireOrderPrincipal(nil, session, DefaultSessionCookieName, []string{"https://app.example"}))
	router.GET("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/orders", nil)
	request.Header["Authorization"] = []string{""}
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	if session.calls != 0 {
		t.Fatalf("session authenticator calls = %d, want 0", session.calls)
	}
}

func TestRequireOrderPrincipalAuthenticatesVerifiedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
	router := gin.New()
	router.Use(RequireOrderPrincipal(nil, session, DefaultSessionCookieName, []string{"https://app.example"}))
	router.POST("/orders", func(c *gin.Context) {
		principal, ok := OrderPrincipal(c.Request.Context())
		if !ok || principal.Kind != usecase.OrderPrincipalRetailSession || principal.OwnerUserID != "user-1" || principal.SessionID != "session-1" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/orders", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	request.Header.Set("Origin", "https://app.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRequireOrderPrincipalUsesRefererWhenOriginIsAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
	router := gin.New()
	router.Use(RequireOrderPrincipal(nil, session, DefaultSessionCookieName, []string{"https://app.example"}))
	router.POST("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodPost, "/orders", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	request.Header.Set("Referer", "https://app.example/checkout/confirm")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRequireOrderPrincipalRejectsSessionMutationWithoutAllowedOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		origin  string
		referer string
	}{
		{name: "missing origin", origin: "", referer: ""},
		{name: "mismatched origin", origin: "https://evil.example"},
		{name: "mismatched referer", referer: "https://evil.example/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
			router := gin.New()
			router.Use(RequireOrderPrincipal(nil, session, DefaultSessionCookieName, []string{"https://app.example"}))
			router.POST("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			request := httptest.NewRequest(http.MethodPost, "/orders", nil)
			request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				request.Header.Set("Referer", tt.referer)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
		})
	}
}

func TestRequireOrderPrincipalDoesNotRequireOriginForSessionReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")}
	router := gin.New()
	router.Use(RequireOrderPrincipal(nil, session, DefaultSessionCookieName, []string{"https://app.example"}))
	router.GET("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/orders", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRequireOrderPrincipalRejectsInvalidAndUnverifiedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		api        *orderAPIAuthenticatorFake
		session    *orderSessionAuthenticatorFake
		wantStatus int
	}{
		{
			name:       "missing credentials",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed bearer does not fall back to session",
			api:        &orderAPIAuthenticatorFake{principal: usecase.Principal{ClientID: "client-1", OwnerUserID: "owner-1"}},
			session:    &orderSessionAuthenticatorFake{user: verifiedOrderUser("user-1", "session-1")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid api key",
			api:        &orderAPIAuthenticatorFake{err: errors.New("invalid key")},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid session",
			session:    &orderSessionAuthenticatorFake{err: usecase.ErrInvalidSession},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "unverified session",
			session: &orderSessionAuthenticatorFake{user: usecase.AuthenticatedUser{
				User:      usecase.UserProfile{ID: "user-1", EmailVerified: false},
				SessionID: "session-1",
			}},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(RequireOrderPrincipal(tt.api, tt.session, DefaultSessionCookieName, []string{"https://app.example"}))
			router.GET("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			request := httptest.NewRequest(http.MethodGet, "/orders", nil)
			if tt.name == "malformed bearer does not fall back to session" {
				request.Header.Set("Authorization", "Basic abc")
				request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
			} else if tt.api != nil {
				request.Header.Set("Authorization", "Bearer pk_test_secret")
			} else if tt.session != nil {
				request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
		})
	}
}

func verifiedOrderUser(userID, sessionID string) usecase.AuthenticatedUser {
	return usecase.AuthenticatedUser{
		User:       usecase.UserProfile{ID: userID, EmailVerified: true},
		SessionID:  sessionID,
		LastUsedAt: time.Now().UTC(),
	}
}
