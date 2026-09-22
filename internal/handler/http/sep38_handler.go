package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type Sep38Service interface {
	IndicativePrice(ctx context.Context, sellAsset, buyAsset string) (usecase.Sep38IndicativePrice, error)
	Price(ctx context.Context, request usecase.Sep38PriceRequest) (usecase.Sep38Price, error)
}

type Sep38Handler struct {
	service Sep38Service
	logger  *slog.Logger
}

func NewSep38Handler(service Sep38Service, logger *slog.Logger) *Sep38Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sep38Handler{service: service, logger: logger}
}

func (h *Sep38Handler) Info(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"assets": []gin.H{
			{"asset": usecase.Sep38XLMAsset},
			{
				"asset": usecase.Sep38IDRAsset,
				"sell_delivery_methods": []gin.H{
					{"name": "xendit", "description": "Pay IDR using the hosted sandbox checkout."},
					{"name": "qris", "description": "Pay IDR using QRIS in the hosted sandbox checkout."},
					{"name": "bri_va", "description": "Pay IDR using a BRI virtual account in the hosted sandbox checkout."},
				},
				"buy_delivery_methods": []gin.H{
					{"name": "sandbox_bank_transfer", "description": "Receive IDR through the sandbox payout simulation."},
				},
				"country_codes": []string{"ID"},
			},
		},
	})
}

func (h *Sep38Handler) Prices(c *gin.Context) {
	hasSellAsset := strings.TrimSpace(c.Query("sell_asset")) != ""
	hasBuyAsset := strings.TrimSpace(c.Query("buy_asset")) != ""
	sellAsset, buyAsset, ok := sep38PairFromQuery(c)
	if !ok {
		h.writeError(c, usecase.ErrInvalidSEP38Request)
		return
	}
	price, err := h.service.IndicativePrice(c.Request.Context(), sellAsset, buyAsset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if hasBuyAsset && !hasSellAsset {
		asset := gin.H{"asset": price.SellAsset, "price": price.Price, "decimals": price.SellDecimals}
		c.JSON(http.StatusOK, gin.H{"sell_assets": []gin.H{asset}})
		return
	}
	asset := gin.H{"asset": price.BuyAsset, "price": price.Price, "decimals": price.BuyDecimals}
	c.JSON(http.StatusOK, gin.H{"buy_assets": []gin.H{asset}})
}

func (h *Sep38Handler) Price(c *gin.Context) {
	request := usecase.Sep38PriceRequest{
		SellAsset:  c.Query("sell_asset"),
		BuyAsset:   c.Query("buy_asset"),
		SellAmount: c.Query("sell_amount"),
		BuyAmount:  c.Query("buy_amount"),
	}
	price, err := h.service.Price(c.Request.Context(), request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total_price": price.TotalPrice,
		"price":       price.Price,
		"sell_asset":  price.SellAsset,
		"sell_amount": price.SellAmount,
		"buy_asset":   price.BuyAsset,
		"buy_amount":  price.BuyAmount,
	})
}

func sep38PairFromQuery(c *gin.Context) (string, string, bool) {
	sellAsset := strings.TrimSpace(c.Query("sell_asset"))
	buyAsset := strings.TrimSpace(c.Query("buy_asset"))
	switch {
	case sellAsset == usecase.Sep38IDRAsset && buyAsset == "":
		buyAsset = usecase.Sep38XLMAsset
	case sellAsset == usecase.Sep38XLMAsset && buyAsset == "":
		buyAsset = usecase.Sep38IDRAsset
	case sellAsset == "" && buyAsset == usecase.Sep38XLMAsset:
		sellAsset = usecase.Sep38IDRAsset
	case sellAsset == "" && buyAsset == usecase.Sep38IDRAsset:
		sellAsset = usecase.Sep38XLMAsset
	}
	return sellAsset, buyAsset, sellAsset != "" && buyAsset != ""
}

func (h *Sep38Handler) writeError(c *gin.Context, err error) {
	status := http.StatusServiceUnavailable
	message := "The quote service is temporarily unavailable."
	if errors.Is(err, usecase.ErrInvalidSEP38Request) || errors.Is(err, usecase.ErrAmountOutOfRange) {
		status = http.StatusBadRequest
		message = "The SEP-38 request is invalid."
	}
	if errors.Is(err, usecase.ErrInvalidPrice) || errors.Is(err, usecase.ErrStalePrice) {
		message = "A fresh indicative price is unavailable."
	}
	if status == http.StatusServiceUnavailable {
		h.logger.ErrorContext(c.Request.Context(), "SEP-38 request failed", slog.Any("error", err))
	}
	c.JSON(status, gin.H{"error": message, "request_id": middleware.RequestIDFromContext(c)})
}
