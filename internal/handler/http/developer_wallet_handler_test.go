package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type developerWalletServiceSpy struct {
	input  usecase.DeveloperWalletInput
	userID string
	view   usecase.DeveloperWalletView
	views  []usecase.DeveloperWalletView
	err    error
}

func (s *developerWalletServiceSpy) Register(_ context.Context, userID string, input usecase.DeveloperWalletInput) (usecase.DeveloperWalletView, error) {
	s.userID, s.input = userID, input
	return s.view, s.err
}

func (s *developerWalletServiceSpy) List(_ context.Context, userID string) ([]usecase.DeveloperWalletView, error) {
	s.userID = userID
	return s.views, s.err
}

func (s *developerWalletServiceSpy) Update(context.Context, string, string, usecase.DeveloperWalletUpdate) (usecase.DeveloperWalletView, error) {
	return s.view, s.err
}

func (s *developerWalletServiceSpy) Revoke(context.Context, string, string) error { return s.err }

func TestDeveloperWalletHandlerRegistersSEP10VerifiedWallet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &developerWalletServiceSpy{view: usecase.DeveloperWalletView{
		ID: "wallet-1", UserID: "user-1", Network: usecase.StellarTestnetNetwork,
		WalletAccount: "GACCOUNT", Label: "Treasury", Status: "active", VerifiedAt: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC),
	}}
	handler := NewDeveloperWalletHandler(service, nil)
	router := gin.New()
	router.POST("/v1/developer/wallets", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Create)

	request := httptest.NewRequest(http.MethodPost, "/v1/developer/wallets", strings.NewReader(`{"network":"stellar_testnet","wallet_account":"GACCOUNT","label":"Treasury","is_primary":true,"sep10_token":"token-1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.userID != "user-1" || service.input.SEP10Token != "token-1" || service.input.WalletAccount != "GACCOUNT" {
		t.Fatalf("status = %d, user/input = %q/%#v, body = %q", response.Code, service.userID, service.input, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"wallet_account":"GACCOUNT"`) || strings.Contains(response.Body.String(), "token-1") {
		t.Fatalf("wallet response leaked proof or omitted account: %q", response.Body.String())
	}
}

func TestDeveloperWalletHandlerMapsProofMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &developerWalletServiceSpy{err: usecase.ErrDeveloperWalletAccountMismatch}
	handler := NewDeveloperWalletHandler(service, nil)
	router := gin.New()
	router.POST("/v1/developer/wallets", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Create)

	request := httptest.NewRequest(http.MethodPost, "/v1/developer/wallets", strings.NewReader(`{"network":"stellar_testnet","wallet_account":"GACCOUNT","label":"Treasury","sep10_token":"token-1"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "wallet account proof does not match") || strings.Contains(response.Body.String(), "token-1") {
		t.Fatalf("status/body = %d/%q", response.Code, response.Body.String())
	}
}
