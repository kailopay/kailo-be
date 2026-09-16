package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

const maxKYCWebhookBytes int64 = 1 << 20

const (
	kycCallbackInvalidBodyCode      = "INVALID_CALLBACK_BODY"
	kycCallbackInvalidSignatureCode = "INVALID_CALLBACK_SIGNATURE"
	kycCallbackConflictCode         = "CALLBACK_EVENT_CONFLICT"
	kycCallbackInquiryMissingCode   = "KYC_INQUIRY_NOT_FOUND"
	kycCallbackProcessingCode       = "CALLBACK_PROCESSING_FAILED"
)

type KYCHandler struct {
	service                   usecase.KYCService
	logger                    *slog.Logger
	exposeProviderDiagnostics bool
}

func NewKYCHandler(service usecase.KYCService, logger *slog.Logger) *KYCHandler {
	return NewKYCHandlerWithConfig(service, logger, KYCHandlerConfig{})
}

type KYCHandlerConfig struct {
	ExposeProviderDiagnostics bool
}

func NewKYCHandlerWithConfig(service usecase.KYCService, logger *slog.Logger, config KYCHandlerConfig) *KYCHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &KYCHandler{service: service, logger: logger, exposeProviderDiagnostics: config.ExposeProviderDiagnostics}
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
	if err != nil {
		h.writeCallbackError(w, request, http.StatusBadRequest, kycCallbackInvalidBodyCode, "Callback body could not be read.", false, err)
		return
	}
	if int64(len(body)) > maxKYCWebhookBytes {
		h.writeCallbackError(w, request, http.StatusBadRequest, kycCallbackInvalidBodyCode, "Callback body is too large.", false, errors.New("callback body exceeds maximum size"))
		return
	}
	err = h.service.ProcessPersonaWebhook(request.Context(), body, request.Header.Get("Persona-Signature"), time.Now().UTC())
	switch {
	case err == nil:
		h.writeCallbackJSON(w, request, http.StatusOK, map[string]string{"status": "accepted"})
	case errors.Is(err, usecase.ErrKYCInvalidWebhook):
		h.writeCallbackError(w, request, http.StatusUnauthorized, kycCallbackInvalidSignatureCode, "Callback signature verification failed.", false, err)
	case errors.Is(err, usecase.ErrKYCProviderEventConflict):
		h.writeCallbackError(w, request, http.StatusBadRequest, kycCallbackConflictCode, "The callback event conflicts with an existing event.", false, err)
	case errors.Is(err, usecase.ErrKYCInquiryNotFound):
		// A callback can race the short window between provider inquiry
		// creation and local attachment. Keep it retryable so approval cannot
		// be lost before the local correlation record exists.
		h.writeCallbackError(w, request, http.StatusServiceUnavailable, kycCallbackInquiryMissingCode, "The referenced KYC inquiry is not available yet.", true, err)
	default:
		h.logger.ErrorContext(request.Context(), "processing Persona KYC callback failed", slog.Any("error", err))
		h.writeCallbackError(w, request, http.StatusServiceUnavailable, kycCallbackProcessingCode, "Callback processing failed temporarily.", true, err)
	}
}

func (h *KYCHandler) writeCallbackJSON(w http.ResponseWriter, request *http.Request, status int, body map[string]string) {
	response := make(map[string]any, len(body)+1)
	for key, value := range body {
		response[key] = value
	}
	if requestID := strings.TrimSpace(request.Header.Get("X-Request-ID")); requestID != "" {
		response["request_id"] = requestID
	}
	writeCallbackJSON(w, status, response)
}

func (h *KYCHandler) writeCallbackError(w http.ResponseWriter, request *http.Request, status int, code, message string, retryable bool, err error) {
	errorBody := map[string]any{
		"code":      code,
		"message":   message,
		"retryable": retryable,
	}
	if h.exposeProviderDiagnostics && err != nil {
		errorBody["details"] = err.Error()
	}
	response := map[string]any{"error": errorBody}
	if requestID := strings.TrimSpace(request.Header.Get("X-Request-ID")); requestID != "" {
		response["request_id"] = requestID
	}
	writeCallbackJSON(w, status, response)
}

func (h *KYCHandler) writeError(c *gin.Context, operation string, err error) {
	code, status := kycErrorMapping(err)
	errorBody := gin.H{"code": code, "message": kycPublicErrorMessage(code)}
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "INTERNAL_ERROR", http.StatusInternalServerError
		errorBody["code"] = code
		errorBody["message"] = kycPublicErrorMessage(code)
	} else if errors.Is(err, usecase.ErrKYCProviderUnavailable) {
		var providerErr *usecase.KYCProviderUnavailableError
		if errors.As(err, &providerErr) && providerErr.Err != nil {
			// The Persona adapter deliberately excludes response bodies and credentials
			// from its errors, so the wrapped cause is safe for internal diagnostics.
			h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", providerErr.Err))
			if h.exposeProviderDiagnostics {
				errorBody["details"] = providerErr.Err.Error()
			}
		}
	}
	c.JSON(status, gin.H{
		"error":      errorBody,
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
		"url":             view.URL,
		"environment_id":  view.EnvironmentID,
		"expires_at":      view.ExpiresAt,
	}
	if view.SessionToken != "" {
		inquiry["session_token"] = view.SessionToken
	}
	return inquiry
}
