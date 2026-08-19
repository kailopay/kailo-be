package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/febry3/kailopay-be/internal/service/apikey"
	"github.com/gin-gonic/gin"
)

type fakeAPIKeyAuthenticator struct {
	raw       string
	principal apikey.Principal
	err       error
}

func (f *fakeAPIKeyAuthenticator) Authenticate(_ context.Context, raw string) (apikey.Principal, error) {
	f.raw = raw
	return f.principal, f.err
}

func TestRequireAPIKeyAuthenticatesBearerKeyAndSetsPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authenticator := &fakeAPIKeyAuthenticator{principal: apikey.Principal{ClientID: "client-1", OwnerUserID: "user-1"}}
	router := gin.New()
	router.Use(RequireAPIKey(authenticator))
	router.GET("/orders", func(c *gin.Context) {
		principal, ok := APIPrincipal(c.Request.Context())
		if !ok || principal.ClientID != "client-1" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/orders", nil)
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || authenticator.raw != "pk_test_public_secret" {
		t.Fatalf("status = %d, raw key = %q", response.Code, authenticator.raw)
	}
}

func TestRequireAPIKeyRejectsMissingCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireAPIKey(&fakeAPIKeyAuthenticator{}))
	router.GET("/orders", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/orders", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
