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

type APIKeyService interface {
	Create(ctx context.Context, ownerUserID, name string) (usecase.CreatedKey, error)
	List(ctx context.Context, ownerUserID string) ([]usecase.Metadata, error)
	Revoke(ctx context.Context, ownerUserID, keyID string) error
}

type APIKeyHandler struct {
	service APIKeyService
	logger  *slog.Logger
}

func NewAPIKeyHandler(service APIKeyService, logger *slog.Logger) *APIKeyHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &APIKeyHandler{service: service, logger: logger}
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(c, &request, 4<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	created, err := h.service.Create(c.Request.Context(), user.User.ID, request.Name)
	if err != nil {
		h.writeError(c, "creating api key", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"api_key": created})
}

func (h *APIKeyHandler) List(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	keys, err := h.service.List(c.Request.Context(), user.User.ID)
	if err != nil {
		h.writeError(c, "listing api keys", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"api_keys": keys})
}

func (h *APIKeyHandler) Revoke(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if err := h.service.Revoke(c.Request.Context(), user.User.ID, c.Param("id")); err != nil {
		h.writeError(c, "revoking api key", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *APIKeyHandler) writeError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, usecase.ErrDeveloperModeRequired):
		c.JSON(http.StatusForbidden, gin.H{"error": "Developer Mode is required"})
	case errors.Is(err, usecase.ErrInvalidName):
		writeRequestError(c, http.StatusBadRequest)
	case errors.Is(err, usecase.ErrInvalidKey):
		writeRequestError(c, http.StatusNotFound)
	default:
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
	}
}
