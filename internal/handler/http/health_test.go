package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHealthReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", NewHealthHandler(nil, time.Second, nil).Health)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestReadyReturnsServiceUnavailableWhenDependencyFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHealthHandler(func(context.Context) error { return errors.New("database unavailable") }, time.Second, nil)
	router.GET("/ready", handler.Ready)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestLivenessDoesNotCallDependency(t *testing.T) {
	called := false
	handler := NewHealthHandler(func(context.Context) error {
		called = true
		return errors.New("should not be called")
	}, time.Second, nil)

	router := gin.New()
	router.GET("/livez", handler.Liveness)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if called {
		t.Fatal("liveness called a dependency")
	}
}

func TestReadinessHonorsTimeout(t *testing.T) {
	handler := NewHealthHandler(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, 10*time.Millisecond, nil)

	router := gin.New()
	router.GET("/readyz", handler.Readiness)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestStartupChangesAfterInitialization(t *testing.T) {
	handler := NewHealthHandler(nil, time.Second, nil)
	router := gin.New()
	router.GET("/startupz", handler.Startup)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/startupz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("startup status = %d, want %d before initialization", recorder.Code, http.StatusServiceUnavailable)
	}

	handler.MarkStarted()
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/startupz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("startup status = %d, want %d after initialization", recorder.Code, http.StatusOK)
	}
}
