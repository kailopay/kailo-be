package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeOnrampService struct {
	command usecase.Command
	view    usecase.OrderView
	err     error
}

func (s *fakeOnrampService) Create(_ context.Context, command usecase.Command) (usecase.OrderView, bool, error) {
	s.command = command
	if s.err != nil {
		return usecase.OrderView{}, false, s.err
	}
	if s.view.ID != "" {
		return s.view, false, nil
	}
	return usecase.OrderView{ID: "order-1", Status: entity.OrderStatusPaymentPending, FiatAmountMinor: 100_000, AssetAmount: 400_000_000}, false, nil
}
func (s *fakeOnrampService) Get(context.Context, usecase.OrderPrincipal, string) (usecase.OrderView, error) {
	return usecase.OrderView{}, nil
}
func (s *fakeOnrampService) List(context.Context, usecase.OrderPrincipal, int, string) ([]usecase.OrderView, string, error) {
	return []usecase.OrderView{}, "", nil
}

type fixedAPIAuthenticator struct{}

func (fixedAPIAuthenticator) Authenticate(context.Context, string) (usecase.Principal, error) {
	return usecase.Principal{ClientID: "client-1", OwnerUserID: "owner-1"}, nil
}

func TestOnrampHandlerCreatesClientOwnedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{}
	handler := NewOnrampHandler(service, nil)
	router := gin.New()
	router.POST("/v1/onramps", middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil), handler.Create)
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"qris","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","memo":null}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("Idempotency-Key", "idem-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.command.Principal.ClientID != "client-1" || service.command.Amount != 100_000 {
		t.Fatalf("status = %d, command = %+v, body = %q", response.Code, service.command, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"amount_minor":"100000"`) || !strings.Contains(response.Body.String(), `"amount":"40.0000000"`) {
		t.Fatalf("amounts are not exact strings: %q", response.Body.String())
	}
}

func TestOnrampHandlerCreatesRetailSessionOwnedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{}
	handler := NewOnrampHandler(service, nil)
	router := gin.New()
	retailUser := usecase.AuthenticatedUser{
		User:      usecase.UserProfile{ID: "user-1", EmailVerified: true},
		SessionID: "session-1",
	}
	router.POST("/v1/onramps", middleware.RequireOrderPrincipal(nil, fakeSessionAuthenticator{user: retailUser}, middleware.DefaultSessionCookieName, []string{"http://localhost:3000"}), handler.Create)
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"xendit","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Idempotency-Key", "idem-retail-1")
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session-token"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	want := usecase.OrderPrincipal{Kind: usecase.OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"}
	if response.Code != http.StatusCreated || service.command.Principal != want {
		t.Fatalf("status = %d, principal = %+v, want %+v, body = %q", response.Code, service.command.Principal, want, response.Body.String())
	}
}

func TestOnrampHandlerExposesHostedCheckoutURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{view: usecase.OrderView{ID: "order-1", Status: entity.OrderStatusPaymentPending,
		FiatAmountMinor: 100_000, AssetAmount: 400_000_000, Checkout: &usecase.Checkout{
			ProviderID: "ps-1", Status: "ACTIVE", PresentationType: "PAYMENT_LINK",
			PresentationValue: "https://checkout-staging.xendit.co/sessions/ps-1",
			PaymentLinkURL:    "https://checkout-staging.xendit.co/sessions/ps-1",
		}}}
	handler := NewOnrampHandler(service, nil)
	router := gin.New()
	router.POST("/v1/onramps", middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil), handler.Create)
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"xendit","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("Idempotency-Key", "idem-hosted-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated || service.command.PaymentMethod != entity.PaymentMethod("xendit") {
		t.Fatalf("status = %d, command = %+v, body = %q", response.Code, service.command, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"payment_link_url":"https://checkout-staging.xendit.co/sessions/ps-1"`) {
		t.Fatalf("body missing hosted checkout URL: %q", response.Body.String())
	}
}

func TestOnrampHandlerMapsStablePublicErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"amount out of range", usecase.ErrAmountOutOfRange, http.StatusUnprocessableEntity, "AMOUNT_OUT_OF_RANGE"},
		{"invalid destination", usecase.ErrInvalidDestination, http.StatusBadRequest, "INVALID_STELLAR_ACCOUNT"},
		{"stale quote", usecase.ErrStalePrice, http.StatusServiceUnavailable, "QUOTE_UNAVAILABLE"},
		{"invalid quote", fmt.Errorf("creating quote: %w", usecase.ErrInvalidQuote), http.StatusServiceUnavailable, "QUOTE_UNAVAILABLE"},
		{"idempotency conflict", usecase.ErrIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED"},
		{"external failure", errors.New("reading treasury balance: connection refused"), http.StatusServiceUnavailable, "EXTERNAL_SERVICE_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &fakeOnrampService{err: testCase.err}
			handler := NewOnrampHandler(service, nil)
			router := gin.New()
			router.POST("/v1/onramps", middleware.RequestID(), middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil), handler.Create)
			body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"qris","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`
			request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer pk_test_public_secret")
			request.Header.Set("Idempotency-Key", "idem-1")
			request.Header.Set("X-Request-ID", "req-error-1")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d, body = %q", response.Code, testCase.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"code":"`+testCase.wantCode+`"`) {
				t.Fatalf("body missing code %q: %q", testCase.wantCode, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"request_id":"req-error-1"`) {
				t.Fatalf("body missing request_id: %q", response.Body.String())
			}
		})
	}
}

func TestOnrampHandlerIncludesOrderIDForUnknownCheckout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeOnrampService{err: &usecase.CheckoutUnknownError{OrderID: "order-unknown-1"}}
	handler := NewOnrampHandler(service, nil)
	router := gin.New()
	router.POST("/v1/onramps", middleware.RequestID(), middleware.RequireOrderPrincipal(fixedAPIAuthenticator{}, nil, middleware.DefaultSessionCookieName, nil), handler.Create)
	body := `{"fiat":{"currency":"IDR","amount_minor":"100000"},"payment_method":"xendit","stellar_destination":{"account":"GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/onramps", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("Idempotency-Key", "idem-unknown-1")
	request.Header.Set("X-Request-ID", "req-unknown-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %q", response.Code, http.StatusAccepted, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"order_id":"order-unknown-1"`) {
		t.Fatalf("body missing trackable order id: %q", response.Body.String())
	}
}
