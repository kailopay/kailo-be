package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/febry3/kailopay-be/internal/usecase"
)

const maxXenditCallbackBytes int64 = 1 << 20

type PaymentCallbackService interface {
	Process(ctx context.Context, raw []byte, token string) error
}

type XenditCallbackHandler struct {
	service PaymentCallbackService
	logger  *slog.Logger
}

func NewXenditCallbackHandler(service PaymentCallbackService, logger *slog.Logger) *XenditCallbackHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &XenditCallbackHandler{service: service, logger: logger}
}

func (h *XenditCallbackHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxXenditCallbackBytes+1))
	if err != nil || int64(len(body)) > maxXenditCallbackBytes {
		writeCallbackJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid callback"})
		return
	}
	err = h.service.Process(request.Context(), body, request.Header.Get("x-callback-token"))
	switch {
	case err == nil:
		writeCallbackJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
	case errors.Is(err, usecase.ErrInvalidCallback):
		writeCallbackJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
	case errors.Is(err, usecase.ErrPaymentMismatch), errors.Is(err, usecase.ErrOrderNotFound):
		writeCallbackJSON(w, http.StatusOK, map[string]string{"status": "retained"})
	default:
		h.logger.ErrorContext(request.Context(), "processing Xendit callback failed", slog.Any("error", err))
		writeCallbackJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily unavailable"})
	}
}

func writeCallbackJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
