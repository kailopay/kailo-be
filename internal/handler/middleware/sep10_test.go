package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type sep10AuthenticatorFake struct {
	principal usecase.SEP10Principal
	err       error
}

func (f sep10AuthenticatorFake) Authenticate(context.Context, string) (usecase.SEP10Principal, error) {
	return f.principal, f.err
}

func TestRequireSEP10StoresWalletPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireSEP10(sep10AuthenticatorFake{principal: usecase.SEP10Principal{Account: "GACCOUNT", TokenID: "token-1"}}))
	router.GET("/protected", func(c *gin.Context) {
		principal, ok := SEP10Principal(c.Request.Context())
		if !ok || principal.Account != "GACCOUNT" || principal.TokenID != "token-1" {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer sep10-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestRequireSEP10RejectsMissingMalformedAndInvalidBearer(t *testing.T) {
	tests := []struct {
		name   string
		header string
		err    error
	}{
		{name: "missing", header: ""},
		{name: "malformed", header: "Basic token"},
		{name: "invalid", header: "Bearer token", err: errors.New("invalid token")},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(RequireSEP10(sep10AuthenticatorFake{err: testCase.err}))
			router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if testCase.header != "" {
				request.Header.Set("Authorization", testCase.header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
		})
	}
}
