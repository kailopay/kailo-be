package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeOfframpHandlerService struct {
	command usecase.OfframpCommand
}

func (s *fakeOfframpHandlerService) Create(_ context.Context, command usecase.OfframpCommand) (usecase.OrderView, bool, error) {
	s.command = command
	return usecase.OrderView{
		ID:              "order-off-1",
		Status:          entity.OrderStatusAssetPending,
		FiatAmountMinor: 100_000,
		AssetAmount:     400_000_000,
	}, false, nil
}

func TestOfframpHandlerCreatesRetailSessionOwnedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOfframpHandlerService{}
	handler := NewOfframpHandler(service, nil)
	retailUser := usecase.AuthenticatedUser{
		User:      usecase.UserProfile{ID: "user-1", EmailVerified: true},
		SessionID: "session-1",
	}
	router := gin.New()
	router.POST("/v1/offramps", middleware.RequireOrderPrincipal(nil, fakeSessionAuthenticator{user: retailUser}, middleware.DefaultSessionCookieName, []string{"http://localhost:3000"}), handler.Create)
	body := `{"asset":{"network":"stellar_testnet","code":"XLM","amount":"25.0000000"},"withdrawal":{"currency":"IDR","method":"sandbox_bank_transfer","destination_token":"sandbox-bank-user-01"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/offramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", "idem-off-retail-1")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	wantPrincipal := usecase.OrderPrincipal{Kind: usecase.OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"}
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %q", response.Code, http.StatusCreated, response.Body.String())
	}
	if service.command.Principal != wantPrincipal {
		t.Fatalf("principal = %+v, want %+v", service.command.Principal, wantPrincipal)
	}
	if service.command.AssetNetwork != "stellar_testnet" || service.command.AssetCode != "XLM" || service.command.FiatCurrency != "IDR" {
		t.Fatalf("asset/currency command = %+v", service.command)
	}
}
