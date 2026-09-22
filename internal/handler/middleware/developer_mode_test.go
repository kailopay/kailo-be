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

type developerModeReaderFake struct {
	enabled bool
	err     error
	userID  string
}

func (f *developerModeReaderFake) DeveloperModeEnabled(_ context.Context, userID string) (bool, error) {
	f.userID = userID
	return f.enabled, f.err
}

func TestRequireDeveloperModeAllowsEnabledUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &developerModeReaderFake{enabled: true}
	router := gin.New()
	router.Use(RequestID(), RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), RequireDeveloperMode(reader, nil))
	router.GET("/developer", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/developer", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || reader.userID != "user-1" {
		t.Fatalf("status = %d, user = %q, want 204 and user-1", response.Code, reader.userID)
	}
}

func TestRequireDeveloperModeRejectsDisabledUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &developerModeReaderFake{}
	nextCalled := false
	router := gin.New()
	router.Use(RequestID(), RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), RequireDeveloperMode(reader, nil))
	router.GET("/developer", func(c *gin.Context) { nextCalled = true })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/developer", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || nextCalled {
		t.Fatalf("status = %d, nextCalled = %t, want 403 and false", response.Code, nextCalled)
	}
	if response.Body.String() != `{"error":"Developer Mode is required","request_id":"`+response.Header().Get("X-Request-ID")+`"}` {
		t.Fatalf("body = %q, want Developer Mode error with request id", response.Body.String())
	}
}

func TestRequireDeveloperModeRejectsMissingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequireDeveloperMode(&developerModeReaderFake{enabled: true}, nil))
	router.GET("/developer", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/developer", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestRequireDeveloperModeReturnsServerErrorWhenReaderFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID(), RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), RequireDeveloperMode(&developerModeReaderFake{err: errors.New("database unavailable")}, nil))
	router.GET("/developer", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/developer", nil)
	request.AddCookie(&http.Cookie{Name: DefaultSessionCookieName, Value: "session-token"})
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
}
