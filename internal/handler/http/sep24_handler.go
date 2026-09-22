package httpapi

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
	WebAuthEndpoint       string
	SigningKey            string
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

	interactive       usecase.Sep24InteractiveService
	sessionAuth       middleware.SessionAuthenticator
	sep10Auth         middleware.SEP10Authenticator
	sessionCookieName string
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

// ConfigureInteractive adds the wallet-authenticated browser hand-off to the
// canonical SEP-24 routes. It does not alter the legacy order-backed service.
func (h *Sep24Handler) ConfigureInteractive(service usecase.Sep24InteractiveService, sessionAuth middleware.SessionAuthenticator, sep10Auth middleware.SEP10Authenticator, sessionCookieName string) {
	h.interactive = service
	h.sessionAuth = sessionAuth
	h.sep10Auth = sep10Auth
	h.sessionCookieName = strings.TrimSpace(sessionCookieName)
	if h.sessionCookieName == "" {
		h.sessionCookieName = middleware.DefaultSessionCookieName
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
WEB_AUTH_ENDPOINT = "%s"
SIGNING_KEY = "%s"
DOCUMENTATION = "https://github.com/febry3/kailopay-be"
[[CURRENCIES]]
code = "XLM"
issuer = ""
status = "test"
desc = "Native XLM on Stellar testnet (sandbox corridor)."
`, h.config.NetworkPassphrase, h.config.DepositAccount,
		h.config.TransferServerURL, h.config.QuoteServerURL, h.config.FederationURL,
		h.config.WebAuthEndpoint, h.config.SigningKey)
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

// InteractiveDeposit starts the wallet-owned session. The SEP-10 middleware
// has already authenticated the wallet account before this method runs.
func (h *Sep24Handler) InteractiveDeposit(c *gin.Context) {
	principal, ok := middleware.SEP10Principal(c.Request.Context())
	if !ok || h.interactive == nil {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	request, err := parseInteractiveRequest(c, usecase.Sep24KindDeposit)
	if err != nil {
		h.writeError(c, "starting SEP-24 interactive deposit", usecase.ErrSEP24InvalidRequest)
		return
	}
	view, err := h.interactive.Start(c.Request.Context(), principal, request)
	if err != nil {
		h.writeError(c, "starting SEP-24 interactive deposit", err)
		return
	}
	h.writeInteractiveSessionResponse(c, view)
}

// InteractiveWithdraw starts the wallet-owned withdrawal session without
// creating an off-ramp order. The order is created only after browser
// linking and KYC completion.
func (h *Sep24Handler) InteractiveWithdraw(c *gin.Context) {
	principal, ok := middleware.SEP10Principal(c.Request.Context())
	if !ok || h.interactive == nil {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	request, err := parseInteractiveRequest(c, usecase.Sep24KindWithdraw)
	if err != nil {
		h.writeError(c, "starting SEP-24 interactive withdrawal", usecase.ErrSEP24InvalidRequest)
		return
	}
	view, err := h.interactive.Start(c.Request.Context(), principal, request)
	if err != nil {
		h.writeError(c, "starting SEP-24 interactive withdrawal", err)
		return
	}
	h.writeInteractiveSessionResponse(c, view)
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

func (h *Sep24Handler) WalletTransaction(c *gin.Context) {
	principal, ok := middleware.SEP10Principal(c.Request.Context())
	service, serviceOK := h.service.(interface {
		GetWalletTransactionByIdentifier(context.Context, string, string) (usecase.Sep24TransactionView, error)
	})
	if !ok || !serviceOK {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	identifiers := []string{strings.TrimSpace(c.Query("id")), strings.TrimSpace(c.Query("stellar_transaction_id")), strings.TrimSpace(c.Query("external_transaction_id"))}
	identifier := ""
	for _, candidate := range identifiers {
		if candidate != "" {
			if identifier != "" {
				h.writeError(c, "getting wallet SEP-24 transaction", usecase.ErrSEP24InvalidRequest)
				return
			}
			identifier = candidate
		}
	}
	if identifier == "" {
		h.writeError(c, "getting wallet SEP-24 transaction", usecase.ErrSEP24InvalidRequest)
		return
	}
	view, err := service.GetWalletTransactionByIdentifier(c.Request.Context(), principal.Account, identifier)
	if err != nil {
		h.writeError(c, "getting wallet SEP-24 transaction", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"transaction": h.publicTransaction(view)})
}

func (h *Sep24Handler) WalletTransactions(c *gin.Context) {
	principal, ok := middleware.SEP10Principal(c.Request.Context())
	service, serviceOK := h.service.(interface {
		ListWalletTransactions(context.Context, string, int) ([]usecase.Sep24TransactionView, error)
	})
	if !ok || !serviceOK {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		h.writeError(c, "listing wallet SEP-24 transactions", usecase.ErrSEP24InvalidRequest)
		return
	}
	var views []usecase.Sep24TransactionView
	if filteredService, filteredOK := h.service.(interface {
		ListWalletTransactionsFiltered(context.Context, string, usecase.Sep24HistoryFilter) ([]usecase.Sep24TransactionView, error)
	}); filteredOK {
		var noOlderThan *time.Time
		if raw := strings.TrimSpace(c.Query("no_older_than")); raw != "" {
			parsed, parseErr := time.Parse(time.RFC3339, raw)
			if parseErr != nil {
				h.writeError(c, "listing wallet SEP-24 transactions", usecase.ErrSEP24InvalidRequest)
				return
			}
			noOlderThan = &parsed
		}
		views, err = filteredService.ListWalletTransactionsFiltered(c.Request.Context(), principal.Account, usecase.Sep24HistoryFilter{
			AssetCode: c.Query("asset_code"), Kind: c.Query("kind"), Limit: limit,
			NoOlderThan: noOlderThan, PagingID: c.Query("paging_id"),
		})
	} else {
		views, err = service.ListWalletTransactions(c.Request.Context(), principal.Account, limit)
	}
	if err != nil {
		h.writeError(c, "listing wallet SEP-24 transactions", err)
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
	if h.interactive != nil {
		h.interactivePage(c)
		return
	}
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

// InteractiveComplete handles the form posted by the browser page. The
// cookie is intentionally authenticated here because the initial interactive
// URL must remain usable before a retail session exists.
func (h *Sep24Handler) InteractiveComplete(c *gin.Context) {
	if h.interactive == nil {
		writeAuthError(c, http.StatusUnauthorized)
		return
	}
	user, ok := h.currentInteractiveUser(c)
	if !ok {
		if wantsJSON(c) {
			writeAuthError(c, http.StatusUnauthorized)
			return
		}
		h.writeInteractiveHTML(c, usecase.Sep24InteractiveView{ID: strings.TrimSpace(c.Param("id"))}, "Sign in to KailoPay to continue.", false)
		return
	}
	input := usecase.Sep24InteractiveCompletion{DestinationToken: sep24FormValue(c, "destination_token")}
	view, err := h.interactive.Complete(c.Request.Context(), c.Param("id"), user, input)
	if err != nil {
		h.writeError(c, "completing SEP-24 interactive transaction", err)
		return
	}
	if wantsJSON(c) {
		c.JSON(http.StatusOK, gin.H{
			"environment": "sandbox",
			"network":     usecase.StellarTestnetNetwork,
			"transaction": h.publicTransaction(view),
		})
		return
	}
	h.writeInteractiveHTML(c, usecase.Sep24InteractiveView{ID: view.ID, Kind: view.Kind, Status: view.Status, KYCStatus: string(usecase.KYCStatusApproved)}, "The transaction has been linked. This is a Stellar testnet sandbox; no real IDR moved.", false)
}

func (h *Sep24Handler) interactivePage(c *gin.Context) {
	transactionID := strings.TrimSpace(c.Param("id"))
	token := strings.TrimSpace(c.Query("token"))
	var (
		view usecase.Sep24InteractiveView
		err  error
	)
	if token != "" {
		view, err = h.interactive.LoadBrowser(c.Request.Context(), transactionID, token)
	} else if principal, authenticated := h.sep10Principal(c); authenticated {
		view, err = h.interactive.Load(c.Request.Context(), principal, transactionID)
	} else {
		h.writeError(c, "loading SEP-24 interactive transaction", usecase.ErrSEP24InteractiveNotFound)
		return
	}
	if err != nil {
		h.writeError(c, "loading SEP-24 interactive transaction", err)
		return
	}
	message := "Sign in to KailoPay to continue."
	if user, authenticated := h.currentInteractiveUser(c); authenticated {
		view, err = h.interactive.Link(c.Request.Context(), transactionID, user)
		if err != nil {
			h.writeError(c, "linking SEP-24 interactive session", err)
			return
		}
		message = "Complete identity verification to continue."
	}
	if wantsJSON(c) {
		response := gin.H{
			"environment":  "sandbox",
			"network":      usecase.StellarTestnetNetwork,
			"kyc_required": view.KYCRequired,
			"interactive":  view,
		}
		if view.Transaction != nil {
			response["transaction"] = h.publicTransaction(*view.Transaction)
		}
		c.JSON(http.StatusOK, response)
		return
	}
	h.writeInteractiveHTML(c, view, message, true)
}

func (h *Sep24Handler) sep10Principal(c *gin.Context) (usecase.SEP10Principal, bool) {
	if principal, ok := middleware.SEP10Principal(c.Request.Context()); ok {
		return principal, true
	}
	if h.sep10Auth == nil {
		return usecase.SEP10Principal{}, false
	}
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return usecase.SEP10Principal{}, false
	}
	principal, err := h.sep10Auth.Authenticate(c.Request.Context(), parts[1])
	if err != nil {
		return usecase.SEP10Principal{}, false
	}
	return principal, true
}

func (h *Sep24Handler) currentInteractiveUser(c *gin.Context) (usecase.AuthenticatedUser, bool) {
	if user, ok := middleware.AuthenticatedUser(c.Request.Context()); ok {
		return user, true
	}
	if h.sessionAuth == nil {
		return usecase.AuthenticatedUser{}, false
	}
	rawToken, err := c.Cookie(h.sessionCookieName)
	if err != nil || strings.TrimSpace(rawToken) == "" {
		return usecase.AuthenticatedUser{}, false
	}
	user, err := h.sessionAuth.Authenticate(c.Request.Context(), rawToken)
	if err != nil {
		return usecase.AuthenticatedUser{}, false
	}
	return user, true
}

func (h *Sep24Handler) writeInteractiveSessionResponse(c *gin.Context, view usecase.Sep24InteractiveView) {
	c.JSON(http.StatusOK, gin.H{
		"type":         usecase.Sep24InteractiveResponseType,
		"url":          view.URL,
		"id":           view.ID,
		"kyc_required": view.KYCRequired,
		"environment":  "sandbox",
		"network":      usecase.StellarTestnetNetwork,
	})
}

func (h *Sep24Handler) writeInteractiveHTML(c *gin.Context, view usecase.Sep24InteractiveView, message string, showForm bool) {
	type interactivePageData struct {
		ID             string
		Kind           string
		Status         string
		KYCStatus      string
		Message        string
		ShowForm       bool
		NeedsReference bool
	}
	data := interactivePageData{
		ID:             view.ID,
		Kind:           view.Kind,
		Status:         view.Status,
		KYCStatus:      view.KYCStatus,
		Message:        message,
		ShowForm:       showForm,
		NeedsReference: view.Kind == usecase.Sep24KindWithdraw,
	}
	const page = `<!doctype html><html><head><meta charset="utf-8"><title>KailoPay sandbox transfer</title></head><body><main><h1>KailoPay sandbox transfer</h1><p>{{.Message}}</p><p>Stellar testnet only. No real IDR moves. Off-ramp destinations are synthetic sandbox references.</p>{{if .ID}}<p>Transaction: <code>{{.ID}}</code></p>{{end}}{{if .KYCStatus}}<p>KYC status: <strong>{{.KYCStatus}}</strong></p>{{end}}{{if .ShowForm}}<form method="post"><input type="hidden" name="_csrf" value="interactive"><label>{{if .NeedsReference}}Sandbox payout reference <input name="destination_token" required>{{else}}Continue{{end}}</label><button type="submit">Continue</button></form>{{end}}</main></body></html>`
	parsed, err := template.New("sep24-interactive").Parse(page)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "rendering SEP-24 interactive page", slog.Any("error", err))
		c.Status(http.StatusInternalServerError)
		return
	}
	var body bytes.Buffer
	if err := parsed.Execute(&body, data); err != nil {
		h.logger.ErrorContext(c.Request.Context(), "writing SEP-24 interactive page", slog.Any("error", err))
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", body.Bytes())
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
	kind := view.Kind
	if kind == usecase.Sep24KindWithdraw {
		kind = "withdrawal"
	}
	transaction := gin.H{
		"id":         view.ID,
		"kind":       kind,
		"status":     view.Status,
		"started_at": view.Order.CreatedAt,
		"updated_at": view.Order.UpdatedAt,
	}
	if view.QuoteID != "" {
		transaction["quote_id"] = view.QuoteID
	}
	if view.StellarTransactionID != "" {
		transaction["stellar_transaction_id"] = view.StellarTransactionID
	}
	if view.ExternalTransactionID != "" {
		transaction["external_transaction_id"] = view.ExternalTransactionID
		transaction["sandbox_disclosure"] = "No real IDR moved; this is a simulated testnet payout."
	}
	if view.Kind == usecase.Sep24KindDeposit {
		transaction["amount_in"] = strconv.FormatInt(int64(view.Order.FiatAmountMinor), 10)
		transaction["amount_out"] = view.Order.AssetAmount.String()
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
		transaction["amount_out"] = strconv.FormatInt(int64(view.Order.FiatAmountMinor), 10)
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

type sep24InteractiveRequestBody struct {
	AssetCode        string `json:"asset_code"`
	AmountMinor      int64  `json:"amount_minor"`
	Amount           string `json:"amount"`
	Account          string `json:"account"`
	Memo             string `json:"memo"`
	PaymentMethod    string `json:"payment_method"`
	DestinationToken string `json:"destination_token"`
	QuoteID          string `json:"quote_id"`
}

func parseInteractiveRequest(c *gin.Context, kind string) (usecase.Sep24InteractiveRequest, error) {
	input := sep24InteractiveRequestBody{
		AssetCode:        sep24FormValue(c, "asset_code"),
		AmountMinor:      parseOptionalInt64(sep24FormValue(c, "amount_minor")),
		Amount:           sep24FormValue(c, "amount"),
		Account:          sep24FormValue(c, "account"),
		Memo:             sep24FormValue(c, "memo"),
		PaymentMethod:    sep24FormValue(c, "payment_method"),
		DestinationToken: sep24FormValue(c, "destination_token"),
		QuoteID:          sep24FormValue(c, "quote_id"),
	}
	if strings.Contains(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
		var body sep24InteractiveRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			return usecase.Sep24InteractiveRequest{}, err
		}
		input = body
	}
	amountMinor := input.AmountMinor
	if amountMinor == 0 && strings.TrimSpace(input.Amount) != "" && kind == usecase.Sep24KindDeposit {
		parsed, err := parseIDRMinor(input.Amount)
		if err != nil {
			return usecase.Sep24InteractiveRequest{}, err
		}
		amountMinor = parsed
	}
	return usecase.Sep24InteractiveRequest{
		Kind:             kind,
		AssetCode:        strings.TrimSpace(input.AssetCode),
		AmountMinor:      entity.IDR(amountMinor),
		AssetAmount:      strings.TrimSpace(input.Amount),
		Account:          strings.TrimSpace(input.Account),
		Memo:             strings.TrimSpace(input.Memo),
		PaymentMethod:    entity.PaymentMethod(strings.TrimSpace(input.PaymentMethod)),
		DestinationToken: strings.TrimSpace(input.DestinationToken),
		QuoteID:          strings.TrimSpace(input.QuoteID),
		IdempotencyKey:   strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	}, nil
}

func parseOptionalInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func wantsJSON(c *gin.Context) bool {
	if strings.EqualFold(strings.TrimSpace(c.Query("format")), "json") {
		return true
	}
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "application/json")
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
	case errors.Is(err, usecase.ErrSEP24TransactionNotFound), errors.Is(err, usecase.ErrOrderNotFound), errors.Is(err, usecase.ErrSEP24InteractiveNotFound):
		return "TRANSACTION_NOT_FOUND", http.StatusNotFound
	case errors.Is(err, usecase.ErrSEP24InteractiveExpired):
		return "TRANSACTION_EXPIRED", http.StatusGone
	case errors.Is(err, usecase.ErrSEP24InteractiveUserMismatch), errors.Is(err, usecase.ErrSEP24InteractiveWalletMismatch):
		return "AUTHENTICATION_REQUIRED", http.StatusForbidden
	case errors.Is(err, usecase.ErrKYCRequired):
		return "KYC_REQUIRED", http.StatusForbidden
	case errors.Is(err, usecase.ErrAmountOutOfRange):
		return "AMOUNT_OUT_OF_RANGE", http.StatusUnprocessableEntity
	case errors.Is(err, usecase.ErrIdempotencyConflict), errors.Is(err, usecase.ErrSEP24TransactionConflict), errors.Is(err, usecase.ErrSEP24InteractiveConflict), errors.Is(err, usecase.ErrSEP24InteractiveCompleted):
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
	case "TRANSACTION_EXPIRED":
		return "The interactive transaction has expired."
	case "AUTHENTICATION_REQUIRED":
		return "Authenticate the wallet and KailoPay account before continuing."
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
