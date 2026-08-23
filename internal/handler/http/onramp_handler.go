package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type OnrampService interface {
	Create(ctx context.Context, command usecase.Command) (usecase.OrderView, bool, error)
	Get(ctx context.Context, clientID, orderID string) (usecase.OrderView, error)
	List(ctx context.Context, clientID string, limit int, cursor string) ([]usecase.OrderView, string, error)
}

type OnrampHandler struct {
	service OnrampService
	logger  *slog.Logger
}

func NewOnrampHandler(service OnrampService, logger *slog.Logger) *OnrampHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &OnrampHandler{service: service, logger: logger}
}

func (h *OnrampHandler) Create(c *gin.Context) {
	principal, ok := middleware.APIPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		writeRequestError(c, http.StatusUnsupportedMediaType)
		return
	}
	var request struct {
		Fiat struct {
			Currency    string `json:"currency"`
			AmountMinor string `json:"amount_minor"`
		} `json:"fiat"`
		PaymentMethod string `json:"payment_method"`
		Destination   struct {
			Account string  `json:"account"`
			Memo    *string `json:"memo"`
		} `json:"stellar_destination"`
	}
	if err := decodeJSON(c, &request, 16<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	amount, err := strconv.ParseInt(request.Fiat.AmountMinor, 10, 64)
	if err != nil || request.Fiat.Currency != "IDR" {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	memo := ""
	if request.Destination.Memo != nil {
		memo = *request.Destination.Memo
	}
	view, replay, err := h.service.Create(c.Request.Context(), usecase.Command{
		ClientID: principal.ClientID, IdempotencyKey: c.GetHeader("Idempotency-Key"), Amount: entity.IDR(amount),
		PaymentMethod: entity.PaymentMethod(request.PaymentMethod), Destination: request.Destination.Account, Memo: memo,
	})
	if err != nil {
		h.writeError(c, "creating onramp", err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"order": publicOrder(view)})
}

func (h *OnrampHandler) Get(c *gin.Context) {
	principal, ok := middleware.APIPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	view, err := h.service.Get(c.Request.Context(), principal.ClientID, c.Param("id"))
	if err != nil {
		h.writeError(c, "getting onramp order", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"order": publicOrder(view)})
}

func (h *OnrampHandler) List(c *gin.Context) {
	principal, ok := middleware.APIPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	views, next, err := h.service.List(c.Request.Context(), principal.ClientID, limit, c.Query("cursor"))
	if err != nil {
		h.writeError(c, "listing onramp orders", err)
		return
	}
	orders := make([]any, 0, len(views))
	for _, view := range views {
		orders = append(orders, publicOrder(view))
	}
	c.JSON(http.StatusOK, gin.H{"orders": orders, "next_cursor": next})
}

func (h *OnrampHandler) writeError(c *gin.Context, operation string, err error) {
	code, status := errorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "EXTERNAL_SERVICE_UNAVAILABLE", http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": publicErrorMessage(code)},
		"request_id": middleware.RequestIDFromContext(c)})
}

func errorMapping(err error) (string, int) {
	switch {
	case errors.Is(err, usecase.ErrInvalidCommand):
		return "INVALID_REQUEST", http.StatusBadRequest
	case errors.Is(err, usecase.ErrInvalidDestination):
		return "INVALID_STELLAR_ACCOUNT", http.StatusBadRequest
	case errors.Is(err, usecase.ErrAmountOutOfRange):
		return "AMOUNT_OUT_OF_RANGE", http.StatusUnprocessableEntity
	case errors.Is(err, usecase.ErrInsufficientLiquidity):
		return "INSUFFICIENT_LIQUIDITY", http.StatusConflict
	case errors.Is(err, usecase.ErrIdempotencyConflict):
		return "IDEMPOTENCY_KEY_REUSED", http.StatusConflict
	case errors.Is(err, usecase.ErrOrderNotFound):
		return "ORDER_NOT_FOUND", http.StatusNotFound
	case errors.Is(err, usecase.ErrCheckoutUnknown):
		return "CHECKOUT_PENDING_RECONCILIATION", http.StatusAccepted
	case errors.Is(err, usecase.ErrInvalidPrice), errors.Is(err, usecase.ErrStalePrice), errors.Is(err, usecase.ErrInvalidQuote):
		return "QUOTE_UNAVAILABLE", http.StatusServiceUnavailable
	default:
		return "", 0
	}
}

func publicErrorMessage(code string) string {
	switch code {
	case "INVALID_REQUEST":
		return "The request payload is invalid."
	case "INVALID_STELLAR_ACCOUNT":
		return "The Stellar testnet destination is invalid."
	case "AMOUNT_OUT_OF_RANGE":
		return "The requested amount is outside the supported range."
	case "INSUFFICIENT_LIQUIDITY":
		return "Insufficient treasury liquidity is available for this order."
	case "IDEMPOTENCY_KEY_REUSED":
		return "This idempotency key was already used with a different request."
	case "ORDER_NOT_FOUND":
		return "The order does not exist or is not visible to this client."
	case "CHECKOUT_PENDING_RECONCILIATION":
		return "The checkout outcome is being reconciled with the payment provider."
	case "QUOTE_UNAVAILABLE":
		return "A fresh quote is currently unavailable; retry shortly."
	default:
		return "An external service is temporarily unavailable."
	}
}

func publicOrder(view usecase.OrderView) gin.H {
	result := gin.H{
		"id": view.ID, "status": view.Status, "environment": "sandbox", "network": "stellar_testnet",
		"fiat":  gin.H{"currency": "IDR", "amount_minor": strconv.FormatInt(int64(view.FiatAmountMinor), 10)},
		"asset": gin.H{"code": "XLM", "amount": view.AssetAmount.String()},
		"quote": gin.H{"rate": view.QuoteRate, "adjusted_rate": view.QuoteAdjustedRate, "spread_bps": view.QuoteSpreadBPS,
			"source_at": view.QuoteSourceAt, "expires_at": view.QuoteExpiresAt},
		"payment_method":      view.PaymentMethod,
		"stellar_destination": gin.H{"account": view.StellarDestination, "memo": view.StellarMemo},
		"created_at":          view.CreatedAt, "updated_at": view.UpdatedAt,
	}
	if view.Checkout != nil {
		result["checkout"] = gin.H{"id": view.Checkout.ProviderID, "status": view.Checkout.Status,
			"presentation_type": view.Checkout.PresentationType, "presentation_value": view.Checkout.PresentationValue,
			"expires_at": view.Checkout.ExpiresAt}
	}
	if view.StellarTransactionHash != "" {
		result["stellar_transaction_hash"] = view.StellarTransactionHash
	}
	if view.FailureCode != "" {
		result["failure_code"] = view.FailureCode
	}
	return result
}
