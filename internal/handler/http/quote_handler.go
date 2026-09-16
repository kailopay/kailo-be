package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type QuoteService interface {
	Preview(ctx context.Context, command usecase.QuotePreviewCommand) (usecase.QuotePreview, error)
}

type QuoteHandler struct {
	service QuoteService
	logger  *slog.Logger
}

func NewQuoteHandler(service QuoteService, logger *slog.Logger) *QuoteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &QuoteHandler{service: service, logger: logger}
}

func (h *QuoteHandler) Create(c *gin.Context) {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		writeRequestError(c, http.StatusUnsupportedMediaType)
		return
	}
	var request struct {
		Direction string `json:"direction"`
		Fiat      struct {
			Currency    string `json:"currency"`
			AmountMinor string `json:"amount_minor"`
		} `json:"fiat"`
		Asset struct {
			Network string `json:"network"`
			Code    string `json:"code"`
			Amount  string `json:"amount"`
		} `json:"asset"`
	}
	if err := decodeJSON(c, &request, 16<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}

	fiatAmount := int64(0)
	if request.Fiat.AmountMinor != "" {
		var err error
		fiatAmount, err = strconv.ParseInt(request.Fiat.AmountMinor, 10, 64)
		if err != nil {
			writeRequestError(c, http.StatusBadRequest)
			return
		}
	}
	preview, err := h.service.Preview(c.Request.Context(), usecase.QuotePreviewCommand{
		Direction: usecase.QuoteDirection(request.Direction), FiatCurrency: request.Fiat.Currency,
		FiatAmountMinor: entity.IDR(fiatAmount), AssetNetwork: request.Asset.Network, AssetCode: request.Asset.Code,
		AssetAmount: request.Asset.Amount,
	})
	if err != nil {
		h.writeError(c, "creating quote", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"quote": publicQuotePreview(preview)})
}

func (h *QuoteHandler) writeError(c *gin.Context, operation string, err error) {
	code, status := errorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "EXTERNAL_SERVICE_UNAVAILABLE", http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": publicErrorMessage(code)},
		"request_id": middleware.RequestIDFromContext(c)})
}

func publicQuotePreview(preview usecase.QuotePreview) gin.H {
	quote := preview.Quote
	return gin.H{
		"direction": string(preview.Direction), "environment": "sandbox", "network": "stellar_testnet",
		"fiat":  gin.H{"currency": usecase.IDRCurrency, "amount_minor": strconv.FormatInt(int64(quote.FiatAmount), 10)},
		"asset": gin.H{"code": usecase.NativeXLMAssetCode, "amount": quote.AssetAmount.String()},
		"rate":  quote.Rate, "adjusted_rate": quote.AdjustedRate, "spread_bps": quote.SpreadBPS,
		"source_at": quote.SourceAt, "expires_at": quote.ExpiresAt,
	}
}
