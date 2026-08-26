package usecase

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/febry3/kailopay-be/internal/entity"
)

// Sep24Status maps internal order states onto the SEP-24 transaction status
// vocabulary (STELLAR-ANCHOR-INTEGRATION §8). ok is false for states with no
// SEP-24 counterpart.
func Sep24Status(status string) (string, bool) {
	switch entity.OrderStatus(status) {
	case entity.OrderStatusCreated:
		return "pending_user_transfer_start", true
	case entity.OrderStatusPaymentPending, entity.OrderStatusAssetPending:
		return "pending_user_transfer_start", true
	case entity.OrderStatusPaymentConfirmed, entity.OrderStatusAssetReceived:
		return "pending_anchor", true
	case entity.OrderStatusStellarProcessing, entity.OrderStatusRetirementProcessing, entity.OrderStatusWithdrawalProcessing:
		return "pending_external", true
	case entity.OrderStatusCompleted:
		return "completed", true
	case entity.OrderStatusExpired:
		return "expired", true
	case entity.OrderStatusPaymentFailed, entity.OrderStatusStellarFailed,
		entity.OrderStatusAssetInvalid, entity.OrderStatusRetirementFailed,
		entity.OrderStatusWithdrawalFailed, entity.OrderStatusCancelled:
		return "error", true
	default:
		return "", false
	}
}

// MapToSEP24Status exposes the mapping with the exported naming used by
// handlers; ok is false when no SEP-24 counterpart exists.
func MapToSEP24Status(status interface{ String() string }) (string, bool) {
	return Sep24Status(status.String())
}

// FederationResolver resolves synthetic demo federation names of the form
// <memo>*kailopay to the configured deposit account. Only names matching the
// documented sandbox pattern resolve; everything else 404s.
type FederationResolver struct {
	DepositAccount string
}

const federationDomain = "kailopay"

func (r *FederationResolver) Resolve(query string) (address, memo string, ok bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	name, domain, found := strings.Cut(query, "*")
	if !found || domain != federationDomain || name == "" || len(name) > 28 ||
		strings.ContainsAny(name, " \t\r\n") || !isFederationNameSafe(name) {
		return "", "", false
	}
	return r.DepositAccount, "fed-" + name, true
}

func isFederationNameSafe(name string) bool {
	for _, character := range name {
		alnum := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if !alnum && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

// Sep24TransactionID returns an opaque identifier for skeleton transactions.
func Sep24TransactionID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "tx-unknown"
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
