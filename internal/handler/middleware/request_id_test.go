package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid", input: "req-123_abc", want: "req-123_abc"},
		{name: "trims whitespace", input: "  req-123  ", want: "req-123"},
		{name: "rejects newline", input: "req-123\nforged", want: ""},
		{name: "rejects oversized", input: string(make([]byte, maxRequestIDLength+1)), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRequestID(tt.input); got != tt.want {
				t.Fatalf("normalizeRequestID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequestIDAddsResolvedIDToRequestHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) {
		requestID := c.Request.Header.Get("X-Request-ID")
		if requestID == "" {
			t.Fatal("request header is missing the resolved request ID")
		}
		if responseID := c.Writer.Header().Get("X-Request-ID"); responseID != requestID {
			t.Fatalf("response request ID = %q, request ID = %q", responseID, requestID)
		}
		c.Status(http.StatusNoContent)
	})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}
