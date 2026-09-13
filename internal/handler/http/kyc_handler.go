package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

const maxKYCWebhookBytes int64 = 1 << 20

type KYCHandler struct {
	service usecase.KYCService
	logger  *slog.Logger
}

func NewKYCHandler(service usecase.KYCService, logger *slog.Logger) *KYCHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &KYCHandler{service: service, logger: logger}
}

func (h *KYCHandler) Status(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	status, err := h.service.Status(c.Request.Context(), user.User.ID)
	if err != nil {
		h.writeError(c, "reading kyc status", err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"kyc": publicKYCStatus(status)})
}

func (h *KYCHandler) Inquiry(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	inquiry, err := h.service.StartInquiry(c.Request.Context(), user.User.ID)
	if err != nil {
		h.writeError(c, "starting kyc inquiry", err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, gin.H{"inquiry": publicKYCInquiry(inquiry)})
}

func (h *KYCHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxKYCWebhookBytes+1))
	if err != nil || int64(len(body)) > maxKYCWebhookBytes {
		writeCallbackJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid callback"})
		return
	}
	err = h.service.ProcessPersonaWebhook(request.Context(), body, request.Header.Get("Persona-Signature"), time.Now().UTC())
	switch {
	case err == nil:
		writeCallbackJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
	case errors.Is(err, usecase.ErrKYCInvalidWebhook):
		writeCallbackJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
	case errors.Is(err, usecase.ErrKYCProviderEventConflict):
		writeCallbackJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid callback"})
	case errors.Is(err, usecase.ErrKYCInquiryNotFound):
		writeCallbackJSON(w, http.StatusOK, map[string]string{"status": "retained"})
	default:
		h.logger.ErrorContext(request.Context(), "processing Persona KYC callback failed", slog.Any("error", err))
		writeCallbackJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporarily unavailable"})
	}
}

func (h *KYCHandler) writeError(c *gin.Context, operation string, err error) {
	code, status := kycErrorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "INTERNAL_ERROR", http.StatusInternalServerError
	}
	c.JSON(status, gin.H{
		"error":      gin.H{"code": code, "message": kycPublicErrorMessage(code)},
		"request_id": middleware.RequestIDFromContext(c),
	})
}

func writeKYCRequiredError(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"code":    "KYC_REQUIRED",
			"message": "Complete identity verification before using this feature.",
		},
		"request_id": middleware.RequestIDFromContext(c),
	})
}

func kycErrorMapping(err error) (string, int) {
	switch {
	case errors.Is(err, usecase.ErrKYCInvalidRequest):
		return "INVALID_REQUEST", http.StatusBadRequest
	case errors.Is(err, usecase.ErrKYCProviderUnavailable):
		return "KYC_PROVIDER_UNAVAILABLE", http.StatusServiceUnavailable
	case errors.Is(err, usecase.ErrKYCInquiryNotFound):
		return "KYC_INQUIRY_NOT_FOUND", http.StatusNotFound
	default:
		return "", 0
	}
}

func kycPublicErrorMessage(code string) string {
	switch code {
	case "INVALID_REQUEST":
		return "The KYC request is invalid."
	case "KYC_PROVIDER_UNAVAILABLE":
		return "Identity verification is temporarily unavailable."
	case "KYC_INQUIRY_NOT_FOUND":
		return "The identity verification inquiry was not found."
	case "KYC_REQUIRED":
		return "Complete identity verification before using this feature."
	default:
		return "An internal error occurred."
	}
}

func publicKYCStatus(view usecase.KYCStatusView) gin.H {
	return gin.H{
		"provider":        view.Provider,
		"status":          view.Status,
		"provider_status": view.ProviderStatus,
		"inquiry_id":      view.InquiryID,
		"created_at":      view.CreatedAt,
		"updated_at":      view.UpdatedAt,
		"expires_at":      view.ExpiresAt,
		"approved_at":     view.ApprovedAt,
	}
}

func publicKYCInquiry(view usecase.KYCInquiryView) gin.H {
	inquiry := gin.H{
		"status":          view.Status,
		"provider_status": view.ProviderStatus,
		"inquiry_id":      view.InquiryID,
		"environment_id":  view.EnvironmentID,
		"expires_at":      view.ExpiresAt,
	}
	if view.SessionToken != "" {
		inquiry["session_token"] = view.SessionToken
	}
	return inquiry
}
