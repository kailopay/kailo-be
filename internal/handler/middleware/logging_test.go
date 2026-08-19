package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoggingUsesSeverityForHTTPStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantLevel string
		notLevel  string
	}{
		{name: "successful request is debug", status: http.StatusOK, wantLevel: "level=DEBUG", notLevel: "level=INFO"},
		{name: "client error is warning", status: http.StatusBadRequest, wantLevel: "level=WARN", notLevel: "level=INFO"},
		{name: "server error is error", status: http.StatusInternalServerError, wantLevel: "level=ERROR", notLevel: "level=INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			var output bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			router := gin.New()
			router.Use(Logging(logger))
			router.GET("/", func(c *gin.Context) { c.Status(tt.status) })

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			logOutput := output.String()
			if !strings.Contains(logOutput, tt.wantLevel) {
				t.Fatalf("log output = %q, want %s", logOutput, tt.wantLevel)
			}
			if strings.Contains(logOutput, tt.notLevel) {
				t.Fatalf("log output = %q, must not contain %s", logOutput, tt.notLevel)
			}
		})
	}
}

func TestLoggingUsesRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	router := gin.New()
	router.Use(Logging(logger))
	router.GET("/orders/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/orders/ord_test", nil))

	if !strings.Contains(output.String(), "route=/orders/:id") {
		t.Fatalf("log output = %q, want route template", output.String())
	}
}
