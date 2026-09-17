package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

// Sep24Config carries the anchor-facing settings used by discovery and the
// interactive response URL.
type Sep24Config struct {
	DepositAccount        string
	NetworkPassphrase     string
	TransferServerURL     string
	QuoteServerURL        string
	FederationURL         string
	DepositMinAmountMinor int64
	DepositMaxAmountMinor int64
}

type Sep24Service interface {
	StartDeposit(ctx context.Context, command usecase.Sep24DepositCommand) (usecase.Sep24TransactionView, error)
	StartWithdraw(ctx context.Context, command usecase.Sep24WithdrawCommand) (usecase.Sep24TransactionView, error)
	GetTransaction(ctx context.Context, principal usecase.OrderPrincipal, transactionID string) (usecase.Sep24TransactionView, error)
	ListTransactions(ctx context.Context, principal usecase.OrderPrincipal, limit int) ([]usecase.Sep24TransactionView, error)
}

// Sep24Handler serves discovery and authenticated interactive transaction
// endpoints. Order creation and KYC decisions stay in the application service.
type Sep24Handler struct {
	config   Sep24Config
	logger   *slog.Logger
	service  Sep24Service
	resolver *usecase.FederationResolver
}

func NewSep24Handler(service Sep24Service, config Sep24Config, logger *slog.Logger) *Sep24Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sep24Handler{
		config:   config,
		logger:   logger,
		service:  service,
		resolver: &usecase.FederationResolver{DepositAccount: config.DepositAccount},
	}
}

// StellarToml serves /.well-known/stellar.toml advertising only implemented
// testnet services.
func (h *Sep24Handler) StellarToml(c *gin.Context) {
	c.Header("Content-Type", "application/x-stellar-toml; charset=utf-8")
	c.String(http.StatusOK, `# KailoPay sandbox/testnet anchor descriptor.
# Every advertised service is implemented on Stellar TESTNET only. This
# anchor handles sandbox funds; it never settles real value.
NETWORK_PASSPHRASE = "%s"
ACCOUNTS = ["%s"]
VERSION = "0.1.0"
TRANSFER_SERVER_SEP24 = "%s"
ANCHOR_QUOTE_SERVER = "%s"
FEDERATION_SERVER = "%s"
DOCUMENTATION = "https://github.com/febry3/kailopay-be"
[[CURRENCIES]]
code = "XLM"
issuer = ""
status = "test"
desc = "Native XLM on Stellar testnet (sandbox corridor)."
`, h.config.NetworkPassphrase, h.config.DepositAccount,
		h.config.TransferServerURL, h.config.QuoteServerURL, h.config.FederationURL)
}

// Federation resolves synthetic demo names to the deposit account with the
// name used as memo. Unknown or malformed queries return 404 per SEP style.
func (h *Sep24Handler) Federation(c *gin.Context) {
	q := c.Query("q")
	address, memo, ok := h.resolver.Resolve(q)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unrecognized federation name"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stellar_address": address, "memo": memo, "memo_type": "text"})
}

// Info returns the static SEP-24 info payload.
func (h *Sep24Handler) Info(c *gin.Context) {
	depositMin := h.config.DepositMinAmountMinor
	if depositMin <= 0 {
		depositMin = 1
	}
	depositMax := h.config.DepositMaxAmountMinor
	if depositMax <= 0 {
		depositMax = 100000
	}
	c.JSON(http.StatusOK, gin.H{
		"deposit": gin.H{"XLM": gin.H{
			"enabled": true, "min_amount": depositMin, "max_amount": depositMax,
			"amount_unit": "idr_minor", "fiat_currency": usecase.IDRCurrency,
		}},
		"withdraw": gin.H{"XLM": gin.H{
			"enabled": true, "min_amount": 1, "max_amount": 100000,
			"amount_unit": usecase.NativeXLMAssetCode, "fiat_currency": usecase.IDRCurrency,
		}},
		"fee": gin.H{"XLM": gin.H{"type": "none"}},
	})
}

// Deposit starts an authenticated SEP-24 deposit and maps it to an on-ramp
// order. The standard multipart form is accepted; amount_minor is the
// sandbox-specific IDR input required by the KailoPay quote flow.
func (h *Sep24Handler) Deposit(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	amount, err := parseIDRMinor(sep24FormValue(c, "amount_minor"))
	if err != nil {
		// Keep the legacy query-only asset_code compatibility without allowing
		// account or amount data to arrive through a URL.
		h.writeError(c, "starting SEP-24 deposit", usecase.ErrSEP24InvalidRequest)
		return
	}
	view, err := h.service.StartDeposit(c.Request.Context(), usecase.Sep24DepositCommand{
		Principal:      principal,
		IdempotencyKey: c.GetHeader("Idempotency-Key"),
		AssetCode:      sep24AssetCode(c),
		AmountMinor:    entity.IDR(amount),
		Destination:    sep24FormValue(c, "account"),
		Memo:           sep24FormValue(c, "memo"),
		PaymentMethod:  entity.PaymentMethod(sep24FormValue(c, "payment_method")),
	})
	if err != nil {
		h.writeError(c, "starting SEP-24 deposit", err)
		return
	}
	h.writeInteractiveResponse(c, view)
}

// Withdraw starts an authenticated SEP-24 withdrawal and maps it to an
// off-ramp order. The order's persisted deposit memo is returned by polling.
func (h *Sep24Handler) Withdraw(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	view, err := h.service.StartWithdraw(c.Request.Context(), usecase.Sep24WithdrawCommand{
		Principal:        principal,
		IdempotencyKey:   c.GetHeader("Idempotency-Key"),
		AssetCode:        sep24AssetCode(c),
		AssetAmount:      sep24FormValue(c, "amount"),
		DestinationToken: sep24FormValue(c, "destination_token"),
	})
	if err != nil {
		h.writeError(c, "starting SEP-24 withdrawal", err)
		return
	}
	h.writeInteractiveResponse(c, view)
}

// Transaction returns the current persisted order state for an owned SEP-24
// mapping. Client-supplied internal status values are intentionally ignored.
func (h *Sep24Handler) Transaction(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	transactionID := strings.TrimSpace(c.Query("id"))
	if transactionID == "" {
		h.writeError(c, "getting SEP-24 transaction", usecase.ErrSEP24InvalidRequest)
		return
	}
	view, err := h.service.GetTransaction(c.Request.Context(), principal, transactionID)
	if err != nil {
		h.writeError(c, "getting SEP-24 transaction", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"transaction": h.publicTransaction(view)})
}

// Transactions returns the authenticated owner's SEP-24 transaction history.
func (h *Sep24Handler) Transactions(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		h.writeError(c, "listing SEP-24 transactions", usecase.ErrSEP24InvalidRequest)
		return
	}
	views, err := h.service.ListTransactions(c.Request.Context(), principal, limit)
	if err != nil {
		h.writeError(c, "listing SEP-24 transactions", err)
		return
	}
	transactions := make([]gin.H, 0, len(views))
	for _, view := range views {
		transactions = append(transactions, h.publicTransaction(view))
	}
	c.JSON(http.StatusOK, gin.H{"transactions": transactions})
}

// Interactive gives the wallet's browser a small authenticated projection of
// the same transaction. A production wallet should use SEP-10/SEP-45 for the
// browser hand-off; the sandbox bridge uses the existing order principal.
func (h *Sep24Handler) Interactive(c *gin.Context) {
	principal, ok := middleware.OrderPrincipal(c.Request.Context())
	if !ok {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	transactionID := strings.TrimSpace(c.Param("id"))
	view, err := h.service.GetTransaction(c.Request.Context(), principal, transactionID)
	if err != nil {
		h.writeError(c, "getting SEP-24 interactive transaction", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"environment":  "sandbox",
		"network":      usecase.StellarTestnetNetwork,
		"kyc_required": false,
		"transaction":  h.publicTransaction(view),
	})
}

func (h *Sep24Handler) writeInteractiveResponse(c *gin.Context, view usecase.Sep24TransactionView) {
	response := gin.H{
		"type":         usecase.Sep24InteractiveResponseType,
		"url":          h.interactiveURL(view.ID),
		"id":           view.ID,
		"kyc_required": false,
		"environment":  "sandbox",
		"network":      usecase.StellarTestnetNetwork,
	}
	if paymentLinkURL := sep24PaymentLinkURL(view); paymentLinkURL != "" {
		response["payment_link_url"] = paymentLinkURL
	}
	c.JSON(http.StatusOK, response)
}

func (h *Sep24Handler) interactiveURL(transactionID string) string {
	base := strings.TrimRight(h.config.TransferServerURL, "/")
	return base + "/interactive/" + url.PathEscape(transactionID)
}

func (h *Sep24Handler) publicTransaction(view usecase.Sep24TransactionView) gin.H {
	transaction := gin.H{
		"id":         view.ID,
		"kind":       view.Kind,
		"status":     view.Status,
		"started_at": view.Order.CreatedAt,
		"updated_at": view.Order.UpdatedAt,
	}
	if paymentLinkURL := sep24PaymentLinkURL(view); paymentLinkURL != "" {
		transaction["payment_link_url"] = paymentLinkURL
	}
	if view.Order.StellarTransactionHash != "" {
		transaction["stellar_transaction_id"] = view.Order.StellarTransactionHash
	}
	if view.Order.DepositTransactionHash != "" {
		transaction["stellar_transaction_id"] = view.Order.DepositTransactionHash
	}
	if view.Kind == usecase.Sep24KindWithdraw {
		transaction["withdraw_anchor_account"] = h.config.DepositAccount
		transaction["withdraw_memo"] = view.Order.StellarMemo
		transaction["withdraw_memo_type"] = "text"
		transaction["amount_in"] = view.Order.AssetAmount.String()
		transaction["more_info_url"] = h.interactiveURL(view.ID)
	}
	return transaction
}

func sep24PaymentLinkURL(view usecase.Sep24TransactionView) string {
	if view.Kind != usecase.Sep24KindDeposit || view.Order.Checkout == nil {
		return ""
	}
	return strings.TrimSpace(view.Order.Checkout.PaymentLinkURL)
}

func parseIDRMinor(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("missing idr amount")
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount <= 0 {
		return 0, errors.New("invalid idr amount")
	}
	return amount, nil
}

func parseLimit(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 20, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		return 0, errors.New("invalid limit")
	}
	return limit, nil
}

func sep24FormValue(c *gin.Context, key string) string {
	return strings.TrimSpace(c.PostForm(key))
}

func sep24AssetCode(c *gin.Context) string {
	if assetCode := sep24FormValue(c, "asset_code"); assetCode != "" {
		return assetCode
	}
	return strings.TrimSpace(c.Query("asset_code"))
}

func (h *Sep24Handler) writeError(c *gin.Context, operation string, err error) {
	code, status := sep24ErrorMapping(err)
	if code == "" {
		h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
		code, status = "EXTERNAL_SERVICE_UNAVAILABLE", http.StatusServiceUnavailable
	}
	message := sep24PublicErrorMessage(code)
	c.JSON(status, gin.H{"error": message, "request_id": middleware.RequestIDFromContext(c)})
}

func sep24ErrorMapping(err error) (string, int) {
	switch {
	case errors.Is(err, usecase.ErrSEP24InvalidRequest), errors.Is(err, usecase.ErrInvalidCommand), errors.Is(err, usecase.ErrInvalidWithdrawal), errors.Is(err, usecase.ErrInvalidDestination):
		return "INVALID_REQUEST", http.StatusBadRequest
	case errors.Is(err, usecase.ErrSEP24TransactionNotFound), errors.Is(err, usecase.ErrOrderNotFound):
		return "TRANSACTION_NOT_FOUND", http.StatusNotFound
	case errors.Is(err, usecase.ErrKYCRequired):
		return "KYC_REQUIRED", http.StatusForbidden
	case errors.Is(err, usecase.ErrAmountOutOfRange):
		return "AMOUNT_OUT_OF_RANGE", http.StatusUnprocessableEntity
	case errors.Is(err, usecase.ErrIdempotencyConflict), errors.Is(err, usecase.ErrSEP24TransactionConflict):
		return "CONFLICT", http.StatusConflict
	case errors.Is(err, usecase.ErrCheckoutUnknown):
		return "TRANSACTION_PENDING_RECONCILIATION", http.StatusAccepted
	default:
		return "", 0
	}
}

func sep24PublicErrorMessage(code string) string {
	switch code {
	case "INVALID_REQUEST":
		return "The SEP-24 request is invalid."
	case "TRANSACTION_NOT_FOUND":
		return "The transaction does not exist or is not visible to this authenticated owner."
	case "KYC_REQUIRED":
		return "Complete identity verification before using this feature."
	case "AMOUNT_OUT_OF_RANGE":
		return "The requested amount is outside the supported range."
	case "CONFLICT":
		return "The transaction conflicts with an existing request."
	case "TRANSACTION_PENDING_RECONCILIATION":
		return "The transaction is pending external reconciliation."
	default:
		return "An external service is temporarily unavailable."
	}
}
