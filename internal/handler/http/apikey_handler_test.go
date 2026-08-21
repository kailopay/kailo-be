package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	auth "github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeAPIKeyService struct {
	created usecase.CreatedKey
	owner   string
	name    string
	revoked string
}

func (s *fakeAPIKeyService) Create(_ context.Context, owner, name string) (usecase.CreatedKey, error) {
	s.owner, s.name = owner, name
	return s.created, nil
}

func (s *fakeAPIKeyService) List(context.Context, string) ([]usecase.Metadata, error) {
	return []usecase.Metadata{}, nil
}

func (s *fakeAPIKeyService) Revoke(_ context.Context, _ string, keyID string) error {
	s.revoked = keyID
	return nil
}

func TestAPIKeyHandlerCreatesOneTimeKeyForAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAPIKeyService{created: usecase.CreatedKey{ID: "key-1", Plaintext: "pk_test_public_secret"}}
	handler := NewAPIKeyHandler(service, nil)
	router := gin.New()
	router.POST("/v1/api-keys", middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{
		User: auth.UserProfile{ID: "user-1"},
	}}), handler.Create)

	request := httptest.NewRequest(http.MethodPost, "/v1/api-keys", strings.NewReader(`{"name":"Default"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.owner != "user-1" || service.name != "Default" {
		t.Fatalf("status = %d, owner/name = %q/%q, body = %q", response.Code, service.owner, service.name, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"key":"pk_test_public_secret"`) {
		t.Fatalf("response does not contain one-time key: %q", response.Body.String())
	}
}

func TestAPIKeyHandlerRevokesOwnedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeAPIKeyService{}
	handler := NewAPIKeyHandler(service, nil)
	router := gin.New()
	router.DELETE("/v1/api-keys/:id", middleware.RequireSession(fakeSessionAuthenticator{user: auth.AuthenticatedUser{
		User: auth.UserProfile{ID: "user-1"},
	}}), handler.Revoke)

	request := httptest.NewRequest(http.MethodDelete, "/v1/api-keys/key-1", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || service.revoked != "key-1" {
		t.Fatalf("status = %d, revoked = %q", response.Code, service.revoked)
	}
}
