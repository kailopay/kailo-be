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

type fakeQuoteService struct {
	command usecase.QuotePreviewCommand
	preview usecase.QuotePreview
	err     error
}

func (s *fakeQuoteService) Preview(_ context.Context, command usecase.QuotePreviewCommand) (usecase.QuotePreview, error) {
	s.command = command
	if s.err != nil {
		return usecase.QuotePreview{}, s.err
	}
	return s.preview, nil
}

func TestQuoteHandlerReturnsPreviewForBuy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeQuoteService{preview: usecase.QuotePreview{Direction: usecase.QuoteDirectionBuy, Quote: usecase.Quote{
		FiatAmount: 100_000, AssetAmount: 400_000_000, Rate: "2500", AdjustedRate: "2500", SourceAt: testQuoteTime(), ExpiresAt: testQuoteTime().Add(5 * time.Minute),
	}}}
	handler := NewQuoteHandler(service, nil)
	router := gin.New()
	router.POST("/v1/quotes", handler.Create)
	request := httptest.NewRequest(http.MethodPost, "/v1/quotes", strings.NewReader(`{"direction":"buy","fiat":{"currency":"IDR","amount_minor":"100000"},"asset":{"network":"stellar_testnet","code":"XLM"}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.command.Direction != usecase.QuoteDirectionBuy || service.command.FiatAmountMinor != 100_000 {
		t.Fatalf("status = %d, command = %+v, body = %q", response.Code, service.command, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"direction":"buy"`) || !strings.Contains(response.Body.String(), `"amount":"40.0000000"`) {
		t.Fatalf("body missing quote preview values: %q", response.Body.String())
	}
}

func TestQuoteHandlerMapsQuoteErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeQuoteService{err: usecase.ErrStalePrice}
	handler := NewQuoteHandler(service, nil)
	router := gin.New()
	router.POST("/v1/quotes", middleware.RequestID(), handler.Create)
	request := httptest.NewRequest(http.MethodPost, "/v1/quotes", strings.NewReader(`{"direction":"buy","fiat":{"currency":"IDR","amount_minor":"100000"},"asset":{"network":"stellar_testnet","code":"XLM"}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer pk_test_public_secret")
	request.Header.Set("X-Request-ID", "req-quote-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"QUOTE_UNAVAILABLE"`) ||
		!strings.Contains(response.Body.String(), `"request_id":"req-quote-1"`) {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

func testQuoteTime() time.Time {
	return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
}
