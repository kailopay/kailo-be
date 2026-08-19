package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/service/apikey"
	"github.com/febry3/kailopay-be/internal/service/onramp"
	"github.com/gin-gonic/gin"
)

type fakeOnrampService struct{ command onramp.Command }

func (s *fakeOnrampService) Create(_ context.Context, command onramp.Command) (onramp.OrderView, bool, error) {
	s.command = command
	return onramp.OrderView{ID: "order-1", Status: entity.OrderStatusPaymentPending, FiatAmountMinor: 100_000, AssetAmount: 400_000_000}, false, nil
}
func (s *fakeOnrampService) Get(context.Context, string, string) (onramp.OrderView, error) {
	return onramp.OrderView{}, nil
}
func (s *fakeOnrampService) List(context.Context, string, int, string) ([]onramp.OrderView, string, error) {
	return []onramp.OrderView{}, "", nil
}

type fixedAPIAuthenticator struct{}

func (fixedAPIAuthenticator) Authenticate(context.Context, string) (apikey.Principal, error) {
	return apikey.Principal{ClientID: "client-1"}, nil
}

func TestOnrampHandlerCreatesClientOwnedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{}
	handler := NewOnrampHandler(service, nil)
	router := gin.New()
	router.POST("/v1/onramps", middleware.RequireAPIKey(fixedAPIAuthenticator{}), handler.Create)
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"qris","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","memo":null}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("Idempotency-Key", "idem-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.command.ClientID != "client-1" || service.command.Amount != 100_000 {
		t.Fatalf("status = %d, command = %+v, body = %q", response.Code, service.command, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"amount_minor":"100000"`) || !strings.Contains(response.Body.String(), `"amount":"40.0000000"`) {
		t.Fatalf("amounts are not exact strings: %q", response.Body.String())
	}
}
