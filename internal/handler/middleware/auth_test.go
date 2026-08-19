package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeSessionAuthenticator struct {
	user auth.AuthenticatedUser
	err  error
}

func (f fakeSessionAuthenticator) Authenticate(_ context.Context, _ string) (auth.AuthenticatedUser, error) {
	return f.user, f.err
}

func TestRequireSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		cookie     string
		authErr    error
		wantStatus int
		wantUser   bool
	}{
		{name: "missing cookie", wantStatus: http.StatusUnauthorized},
		{name: "invalid session", cookie: "bad-token", authErr: auth.ErrInvalidSession, wantStatus: http.StatusUnauthorized},
		{name: "valid session", cookie: "good-token", wantStatus: http.StatusOK, wantUser: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := auth.AuthenticatedUser{User: auth.UserProfile{ID: "user-id"}, LastUsedAt: time.Now()}
			router := gin.New()
			router.Use(RequireSession(fakeSessionAuthenticator{user: user, err: tt.authErr}))
			router.GET("/protected", func(c *gin.Context) {
				_, ok := AuthenticatedUser(c.Request.Context())
				if ok != tt.wantUser {
					t.Errorf("authenticated user present = %v, want %v", ok, tt.wantUser)
				}
				c.Status(http.StatusOK)
			})

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.cookie != "" {
				request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: tt.cookie})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuthenticatedUserReturnsFalseWithoutContextValue(t *testing.T) {
	if _, ok := AuthenticatedUser(context.Background()); ok {
		t.Fatal("AuthenticatedUser() ok = true without context value")
	}
}
