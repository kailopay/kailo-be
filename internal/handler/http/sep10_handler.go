package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type SEP10Handler struct {
	service usecase.SEP10Service
	logger  *slog.Logger
}

func NewSEP10Handler(service usecase.SEP10Service, logger *slog.Logger) *SEP10Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SEP10Handler{service: service, logger: logger}
}

func (h *SEP10Handler) Challenge(c *gin.Context) {
	account := strings.TrimSpace(c.Query("account"))
	if account == "" {
		h.writeError(c, "creating SEP-10 challenge", usecase.ErrSEP10InvalidAccount)
		return
	}
	view, err := h.service.Challenge(c.Request.Context(), account)
	if err != nil {
		h.writeError(c, "creating SEP-10 challenge", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *SEP10Handler) Exchange(c *gin.Context) {
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		writeRequestError(c, http.StatusUnsupportedMediaType)
		return
	}
	var request struct {
		Transaction string `json:"transaction"`
	}
	if err := decodeJSON(c, &request, 1<<20); err != nil || strings.TrimSpace(request.Transaction) == "" {
		h.writeError(c, "exchanging SEP-10 challenge", usecase.ErrSEP10InvalidChallenge)
		return
	}
	view, err := h.service.Exchange(c.Request.Context(), request.Transaction)
	if err != nil {
		h.writeError(c, "exchanging SEP-10 challenge", err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (h *SEP10Handler) writeError(c *gin.Context, operation string, err error) {
	code, status := sep10ErrorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "INTERNAL_ERROR", http.StatusInternalServerError
	}
	c.JSON(status, gin.H{
		"error":      sep10PublicErrorMessage(code),
		"request_id": middleware.RequestIDFromContext(c),
	})
}

func sep10ErrorMapping(err error) (string, int) {
	switch {
	case errors.Is(err, usecase.ErrSEP10InvalidAccount), errors.Is(err, usecase.ErrSEP10InvalidChallenge),
		errors.Is(err, usecase.ErrSEP10ChallengeExpired), errors.Is(err, usecase.ErrSEP10AccountMismatch):
		return "INVALID_REQUEST", http.StatusBadRequest
	case errors.Is(err, usecase.ErrSEP10ChallengeConsumed):
		return "CHALLENGE_REPLAYED", http.StatusConflict
	case errors.Is(err, usecase.ErrSEP10InvalidToken):
		return "INVALID_TOKEN", http.StatusUnauthorized
	default:
		return "", 0
	}
}

func sep10PublicErrorMessage(code string) string {
	switch code {
	case "INVALID_REQUEST":
		return "The SEP-10 request is invalid."
	case "CHALLENGE_REPLAYED":
		return "The SEP-10 challenge was already used."
	case "INVALID_TOKEN":
		return "The SEP-10 token is invalid."
	default:
		return "An internal error occurred."
	}
}
