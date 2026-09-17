package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type sep38HandlerServiceFake struct {
	indicative usecase.Sep38IndicativePrice
	price      usecase.Sep38Price
	err        error
}

func (f sep38HandlerServiceFake) IndicativePrice(context.Context, string, string) (usecase.Sep38IndicativePrice, error) {
	return f.indicative, f.err
}

func (f sep38HandlerServiceFake) Price(context.Context, usecase.Sep38PriceRequest) (usecase.Sep38Price, error) {
	return f.price, f.err
}

func TestSEP38PriceReturnsStandardShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSep38Handler(sep38HandlerServiceFake{price: usecase.Sep38Price{
		TotalPrice: "2525", Price: "2500", SellAsset: usecase.Sep38IDRAsset,
		SellAmount: "100000", BuyAsset: usecase.Sep38XLMAsset, BuyAmount: "39.6039603",
	}}, nil)
	router := gin.New()
	router.GET("/sep38/price", handler.Price)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sep38/price?sell_asset=iso4217%3AIDR&buy_asset=stellar%3Anative&sell_amount=100000", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if payload["total_price"] != "2525" || payload["price"] != "2500" ||
		payload["sell_amount"] != "100000" || payload["buy_amount"] != "39.6039603" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSEP38InfoReturnsSupportedAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSep38Handler(sep38HandlerServiceFake{}, nil)
	router := gin.New()
	router.GET("/sep38/info", handler.Info)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sep38/info", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), usecase.Sep38IDRAsset) ||
		!strings.Contains(response.Body.String(), usecase.Sep38XLMAsset) {
		t.Fatalf("body does not advertise supported assets: %q", response.Body.String())
	}
}
