package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type DeveloperWalletService interface {
	Register(ctx context.Context, userID string, input usecase.DeveloperWalletInput) (usecase.DeveloperWalletView, error)
	List(ctx context.Context, userID string) ([]usecase.DeveloperWalletView, error)
	Update(ctx context.Context, userID, walletID string, update usecase.DeveloperWalletUpdate) (usecase.DeveloperWalletView, error)
	Revoke(ctx context.Context, userID, walletID string) error
}

type DeveloperWalletHandler struct {
	service DeveloperWalletService
	logger  *slog.Logger
}

func NewDeveloperWalletHandler(service DeveloperWalletService, logger *slog.Logger) *DeveloperWalletHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeveloperWalletHandler{service: service, logger: logger}
}

func (h *DeveloperWalletHandler) Create(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok || user.User.ID == "" {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		ClientID      string `json:"client_id"`
		Network       string `json:"network"`
		WalletAccount string `json:"wallet_account"`
		Label         string `json:"label"`
		IsPrimary     bool   `json:"is_primary"`
		SEP10Token    string `json:"sep10_token"`
	}
	if err := decodeJSON(c, &request, 16<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	view, err := h.service.Register(c.Request.Context(), user.User.ID, usecase.DeveloperWalletInput{
		ClientID: request.ClientID, Network: request.Network, WalletAccount: request.WalletAccount,
		Label: request.Label, IsPrimary: request.IsPrimary, SEP10Token: request.SEP10Token,
	})
	if err != nil {
		h.writeError(c, "registering developer wallet", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"wallet": developerWalletResponseFrom(view)})
}

func (h *DeveloperWalletHandler) List(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok || user.User.ID == "" {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	views, err := h.service.List(c.Request.Context(), user.User.ID)
	if err != nil {
		h.writeError(c, "listing developer wallets", err)
		return
	}
	wallets := make([]developerWalletResponse, 0, len(views))
	for _, view := range views {
		wallets = append(wallets, developerWalletResponseFrom(view))
	}
	c.JSON(http.StatusOK, gin.H{"wallets": wallets})
}

func (h *DeveloperWalletHandler) Update(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok || user.User.ID == "" {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	var request struct {
		Label     *string `json:"label"`
		IsPrimary *bool   `json:"is_primary"`
	}
	if err := decodeJSON(c, &request, 8<<10); err != nil {
		writeRequestError(c, http.StatusBadRequest)
		return
	}
	view, err := h.service.Update(c.Request.Context(), user.User.ID, c.Param("id"), usecase.DeveloperWalletUpdate{Label: request.Label, IsPrimary: request.IsPrimary})
	if err != nil {
		h.writeError(c, "updating developer wallet", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"wallet": developerWalletResponseFrom(view)})
}

func (h *DeveloperWalletHandler) Delete(c *gin.Context) {
	user, ok := middleware.AuthenticatedUser(c.Request.Context())
	if !ok || user.User.ID == "" {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	if err := h.service.Revoke(c.Request.Context(), user.User.ID, c.Param("id")); err != nil {
		h.writeError(c, "revoking developer wallet", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DeveloperWalletHandler) writeError(c *gin.Context, operation string, err error) {
	switch {
	case errors.Is(err, usecase.ErrInvalidDeveloperWallet), errors.Is(err, usecase.ErrDeveloperWalletAccountMismatch):
		message := "invalid developer wallet"
		if errors.Is(err, usecase.ErrDeveloperWalletAccountMismatch) {
			message = "wallet account proof does not match"
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
	case errors.Is(err, usecase.ErrDeveloperWalletProofRequired):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "valid SEP-10 wallet proof is required"})
	case errors.Is(err, usecase.ErrDeveloperWalletNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
	default:
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		writeRequestError(c, http.StatusInternalServerError)
	}
}

type developerWalletResponse struct {
	ID                 string     `json:"id"`
	UserID             string     `json:"user_id"`
	ClientID           string     `json:"client_id,omitempty"`
	Network            string     `json:"network"`
	WalletAccount      string     `json:"wallet_account"`
	Label              string     `json:"label"`
	IsPrimary          bool       `json:"is_primary"`
	VerificationMethod string     `json:"verification_method"`
	VerifiedAt         time.Time  `json:"verified_at"`
	Status             string     `json:"status"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func developerWalletResponseFrom(value usecase.DeveloperWalletView) developerWalletResponse {
	return developerWalletResponse{
		ID: value.ID, UserID: value.UserID, ClientID: value.ClientID, Network: value.Network,
		WalletAccount: value.WalletAccount, Label: value.Label, IsPrimary: value.IsPrimary,
		VerificationMethod: value.VerificationMethod, VerifiedAt: value.VerifiedAt,
		Status: value.Status, RevokedAt: value.RevokedAt, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}
