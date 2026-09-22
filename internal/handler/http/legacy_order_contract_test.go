package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/gin-gonic/gin"
)

func TestLegacyOrderAPIContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	onrampService := &fakeOnrampService{}
	offrampService := &fakeOfframpHandlerService{}
	principal := middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil)
	onrampHandler := NewOnrampHandler(onrampService, nil)
	offrampHandler := NewOfframpHandler(offrampService, nil)
	router := gin.New()
	router.POST("/v1/onramps", principal, onrampHandler.Create)
	router.POST("/v1/offramps", principal, offrampHandler.Create)
	router.GET("/v1/orders", principal, onrampHandler.List)
	router.GET("/v1/orders/:id", principal, onrampHandler.Get)

	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"POST /v1/onramps",
		"POST /v1/offramps",
		"GET /v1/orders",
		"GET /v1/orders/:id",
	} {
		if !routes[route] {
			t.Errorf("missing legacy route %s", route)
		}
	}

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "onramp request shape",
			path: "/v1/onramps",
			body: `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"qris","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","memo":null}}`,
		},
		{
			name: "offramp request shape",
			path: "/v1/offramps",
			body: `{"asset":{"network":"stellar_testnet","code":"XLM","amount":"25.0000000"},"withdrawal":{"currency":"IDR","method":"sandbox_bank_transfer","destination_token":"sandbox-bank-user-01"}}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer pk_test_public_secret")
			request.Header.Set("Idempotency-Key", "legacy-contract-"+testCase.name)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d, body = %q", response.Code, http.StatusCreated, response.Body.String())
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode response: %v; body = %q", err, response.Body.String())
			}
			if len(envelope) != 1 {
				t.Fatalf("top-level response keys = %v, want only order", mapKeys(envelope))
			}
			var order map[string]json.RawMessage
			if err := json.Unmarshal(envelope["order"], &order); err != nil {
				t.Fatalf("decode order: %v; body = %q", err, response.Body.String())
			}
			for _, key := range []string{
				"id", "status", "environment", "network", "fiat", "asset", "quote",
				"payment_method", "stellar_destination", "created_at", "updated_at",
			} {
				if _, ok := order[key]; !ok {
					t.Errorf("order is missing required public key %q: %s", key, response.Body.String())
				}
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/offramps", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("X-Request-ID", "legacy-contract-error")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("error status = %d, want %d, body = %q", response.Code, http.StatusUnsupportedMediaType, response.Body.String())
	}
	var errorEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &errorEnvelope); err != nil {
		t.Fatalf("decode error response: %v; body = %q", err, response.Body.String())
	}
	for _, key := range []string{"error", "request_id"} {
		if _, ok := errorEnvelope[key]; !ok {
			t.Errorf("error response is missing key %q: %s", key, response.Body.String())
		}
	}
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
