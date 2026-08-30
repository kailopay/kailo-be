package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

// OfframpService is the create/read surface the handler consumes. Reads
// reuse the on-ramp service: both directions share GET /v1/orders.
type OfframpService interface {
	Create(ctx context.Context, command usecase.OfframpCommand) (usecase.OrderView, bool, error)
}

type OfframpHandler struct {
	service OfframpService
	logger  *slog.Logger
}

func NewOfframpHandler(service OfframpService, logger *slog.Logger) *OfframpHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &OfframpHandler{service: service, logger: logger}
}

func (h *OfframpHandler) Create(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		writeRequestError(c, http.StatusUnsupportedMediaType)
		return
	}
	var request struct {
		Asset struct {
			Network string `json:"network"`
			Code    string `json:"code"`
			Amount  string `json:"amount"`
		} `json:"asset"`
		Withdrawal struct {
			Currency         string `json:"currency"`
			Method           string `json:"method"`
			DestinationToken string `json:"destination_token"`
		} `json:"withdrawal"`
	}
	if err := decodeJSON(c, &request, 16<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	command := usecase.OfframpCommand{
		Principal:        principal,
		IdempotencyKey:   c.GetHeader("Idempotency-Key"),
		AssetNetwork:     request.Asset.Network,
		AssetCode:        request.Asset.Code,
		FiatCurrency:     request.Withdrawal.Currency,
		AssetAmount:      request.Asset.Amount,
		WithdrawalMethod: entity.WithdrawalMethod(request.Withdrawal.Method),
		DestinationToken: request.Withdrawal.DestinationToken,
	}
	view, replay, err := h.service.Create(c.Request.Context(), command)
	if err != nil {
		h.writeError(c, "creating offramp", err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"order": publicOrder(view), "payout_simulation": true})
}

func (h *OfframpHandler) writeError(c *gin.Context, operation string, err error) {
	code, status := errorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "EXTERNAL_SERVICE_UNAVAILABLE", http.StatusServiceUnavailable
	}
	message := publicErrorMessage(code)
	if errors.Is(err, usecase.ErrInvalidWithdrawal) || errors.Is(err, usecase.ErrWeakPassword) {
		message = "The withdrawal request is invalid."
	}
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message},
		"request_id": middleware.RequestIDFromContext(c)})
}
