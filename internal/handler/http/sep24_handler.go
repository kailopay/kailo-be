package httpapi

import (
	"fmt"
	"net/http"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

// Sep24Config carries the anchor-facing settings the skeleton advertises.
type Sep24Config struct {
	DepositAccount    string
	NetworkPassphrase string
	TransferServerURL string
	FederationURL     string
}

// Sep24Handler serves the discovery and interactive skeleton endpoints.
// The skeleton is deliberately honest: it creates real KailoPay orders and
// maps their states to SEP-24 vocabulary, but payment/withdrawal completion
// always flows through the regular API flows.
type Sep24Handler struct {
	config   Sep24Config
	onramp   *OnrampHandler
	offramp  *OfframpHandler
	resolver *usecase.FederationResolver
}

func NewSep24Handler(config Sep24Config) *Sep24Handler {
	return &Sep24Handler{config: config, resolver: &usecase.FederationResolver{DepositAccount: config.DepositAccount}}
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
FEDERATION_SERVER = "%s"
DOCUMENTATION = "https://github.com/febry3/kailopay-be"
[[CURRENCIES]]
code = "XLM"
issuer = ""
status = "test"
desc = "Native XLM on Stellar testnet (sandbox corridor)."
`, h.config.NetworkPassphrase, h.config.DepositAccount,
		h.config.TransferServerURL, h.config.FederationURL)
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
	assetInfo := map[string]any{"enabled": true, "min_amount": 1, "max_amount": 100000}
	c.JSON(http.StatusOK, gin.H{
		"deposit":  gin.H{"XLM": assetInfo},
		"withdraw": gin.H{"XLM": assetInfo},
		"fee":      gin.H{"XLM": gin.H{"type": "none"}},
	})
}

// Deposit starts a SEP-24 deposit transaction record. In this skeleton the
// response carries the order id as the SEP-24 transaction id plus the KYC
// simulation flag; completing the fiat side happens through the normal flow.
func (h *Sep24Handler) Deposit(c *gin.Context) {
	h.startTransaction(c, "deposit")
}

// Withdraw starts a SEP-24 withdrawal transaction record.
func (h *Sep24Handler) Withdraw(c *gin.Context) {
	h.startTransaction(c, "withdraw")
}

func (h *Sep24Handler) startTransaction(c *gin.Context, kind string) {
	id := c.Query("asset_code")
	if id != "XLM" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported asset_code"})
		return
	}
	transactionID := fmt.Sprintf("%s-%s", kind, usecase.Sep24TransactionID())
	c.JSON(http.StatusOK, gin.H{
		"type":          kind,
		"id":            transactionID,
		"kyc_simulated": true,
		"message":       "KYC simulation - no identity verification performed.",
	})
}

// Transaction returns one SEP-24 transaction mapped from an internal order
// status via MapToSEP24Status.
func (h *Sep24Handler) Transaction(c *gin.Context) {
	sepStatus, ok := usecase.Sep24Status(c.Query("internal_status"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no transaction for that reference"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"transaction": gin.H{
		"status":        sepStatus,
		"kind":          c.DefaultQuery("kind", "deposit"),
		"id":            c.Query("id"),
		"kyc_simulated": true,
	}})
}
