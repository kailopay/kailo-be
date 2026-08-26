package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

// WebhookService is the endpoint-management surface for the handler.
type WebhookService interface {
	Register(ctx context.Context, clientID, rawURL string, eventTypes []string) (string, string, error)
	List(ctx context.Context, clientID string) ([]usecase.WebhookEndpointView, error)
	Disable(ctx context.Context, clientID, endpointID string) error
}

type WebhookHandler struct {
	service WebhookService
	logger  *slog.Logger
}

func NewWebhookHandler(service WebhookService, logger *slog.Logger) *WebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookHandler{service: service, logger: logger}
}

func (h *WebhookHandler) Create(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		URL        string   `json:"url"`
		EventTypes []string `json:"event_types"`
	}
	if err := decodeJSON(c, &request, 8<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	id, secret, err := h.service.Register(c.Request.Context(), user.User.ID, request.URL, request.EventTypes)
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidWebhookURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "webhook URL must be https with a public host"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "registering webhook endpoint failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"endpoint": gin.H{"id": id}, "secret": secret})
}

func (h *WebhookHandler) List(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	endpoints, err := h.service.List(c.Request.Context(), user.User.ID)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "listing webhook endpoints failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"endpoints": endpoints})
}

func (h *WebhookHandler) Disable(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if err := h.service.Disable(c.Request.Context(), user.User.ID, c.Param("id")); err != nil {
		if err.Error() == "webhook endpoint not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "disabling webhook endpoint failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}
