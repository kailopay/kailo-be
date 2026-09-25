package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type WebhookControlService interface {
	GetEndpoint(ctx context.Context, ownerID, endpointID string) (usecase.WebhookEndpointView, error)
	ListDeliveries(ctx context.Context, ownerID string, query usecase.WebhookDeliveryQuery) (usecase.WebhookDeliveryPage, error)
	EnqueueTest(ctx context.Context, ownerID, endpointID string) (usecase.WebhookQueuedEvent, error)
	ReplayDelivery(ctx context.Context, ownerID, deliveryID string) error
}

type WebhookHandler struct {
	service WebhookService
	control WebhookControlService
	logger  *slog.Logger
}

func NewWebhookHandler(service WebhookService, logger *slog.Logger) *WebhookHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookHandler{service: service, logger: logger}
}

func (h *WebhookHandler) ConfigureControl(service WebhookControlService) {
	h.control = service
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
		switch {
		case errors.Is(err, usecase.ErrInvalidWebhookURL):
			c.JSON(http.StatusBadRequest, gin.H{"error": "webhook URL must be https with a public host"})
			return
		case errors.Is(err, usecase.ErrInvalidWebhookEventType):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook event type"})
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
		if errors.Is(err, usecase.ErrWebhookEndpointNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "endpoint not found"})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "disabling webhook endpoint failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WebhookHandler) Get(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if h.control == nil {
		writeRequestError(c, http.StatusNotImplemented)
		return
	}
	endpoint, err := h.control.GetEndpoint(c.Request.Context(), user.User.ID, c.Param("id"))
	if err != nil {
		h.writeControlError(c, "reading webhook endpoint", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"endpoint": endpoint})
}

func (h *WebhookHandler) Deliveries(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if h.control == nil {
		writeRequestError(c, http.StatusNotImplemented)
		return
	}
	query, valid := parseWebhookDeliveryQuery(c)
	if !valid {
		return
	}
	page, err := h.control.ListDeliveries(c.Request.Context(), user.User.ID, query)
	if err != nil {
		h.writeControlError(c, "listing webhook deliveries", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": page.Deliveries, "paging_id": page.NextPagingID})
}

func (h *WebhookHandler) Test(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if h.control == nil {
		writeRequestError(c, http.StatusNotImplemented)
		return
	}
	queued, err := h.control.EnqueueTest(c.Request.Context(), user.User.ID, c.Param("id"))
	if err != nil {
		h.writeControlError(c, "queueing webhook test", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "event_id": queued.EventID, "outbox_id": queued.OutboxID})
}

func (h *WebhookHandler) Replay(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if h.control == nil {
		writeRequestError(c, http.StatusNotImplemented)
		return
	}
	if err := h.control.ReplayDelivery(c.Request.Context(), user.User.ID, c.Param("id")); err != nil {
		h.writeControlError(c, "replaying webhook delivery", err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func parseWebhookDeliveryQuery(c *gin.Context) (usecase.WebhookDeliveryQuery, bool) {
	query := usecase.WebhookDeliveryQuery{EndpointID: c.Query("endpoint_id"), EventID: c.Query("event_id"), Status: c.Query("status"), Cursor: c.Query("paging_id")}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook delivery query"})
			return usecase.WebhookDeliveryQuery{}, false
		}
		query.Limit = limit
	}
	for name, raw := range map[string]string{"from": c.Query("from"), "to": c.Query("to")} {
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook delivery query"})
			return usecase.WebhookDeliveryQuery{}, false
		}
		if name == "from" {
			query.From = &parsed
		} else {
			query.To = &parsed
		}
	}
	return query, true
}

func (h *WebhookHandler) writeControlError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, usecase.ErrWebhookEndpointNotFound), errors.Is(err, usecase.ErrWebhookDeliveryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook resource not found"})
	case errors.Is(err, usecase.ErrWebhookReplayNotAllowed):
		c.JSON(http.StatusConflict, gin.H{"error": "webhook delivery cannot be replayed"})
	case strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required"):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook request"})
	default:
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
	}
}
