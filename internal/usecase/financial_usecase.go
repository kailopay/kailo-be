package usecase

import (
	"errors"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	SandboxFeePolicyVersion = "sandbox-zero-fee-v1"
	SandboxFinancialSource  = "sandbox"
)

var ErrInvalidFinancialSnapshot = errors.New("invalid financial snapshot")

type FinancialSnapshotInput struct {
	SnapshotID         string
	OrderID            string
	ClientID           *string
	Direction          string
	Environment        string
	Network            string
	Currency           string
	AssetCode          string
	AssetIssuer        string
	AssetAmount        string
	AssetAmountStroops int64
	GrossAmountMinor   int64
	QuoteSpreadBPS     int
	CreatedAt          time.Time
}

// BuildSandboxFinancialSnapshot records the deterministic sandbox fee policy.
// The zero-fee policy is explicit so later reports do not mistake simulated
// testnet activity for real revenue.
func BuildSandboxFinancialSnapshot(input FinancialSnapshotInput) (entity.OrderFinancial, error) {
	if strings.TrimSpace(input.SnapshotID) == "" || strings.TrimSpace(input.OrderID) == "" ||
		(input.Direction != "onramp" && input.Direction != "offramp") ||
		input.Environment != "test" || input.Network != StellarTestnetNetwork ||
		input.Currency != IDRCurrency || input.AssetCode != NativeXLMAssetCode ||
		strings.TrimSpace(input.AssetAmount) == "" || input.AssetAmountStroops <= 0 ||
		input.GrossAmountMinor <= 0 || input.QuoteSpreadBPS < 0 || input.QuoteSpreadBPS > 10000 ||
		input.CreatedAt.IsZero() {
		return entity.OrderFinancial{}, ErrInvalidFinancialSnapshot
	}

	createdAt := input.CreatedAt.UTC()
	return entity.OrderFinancial{
		ID:                    input.SnapshotID,
		OrderID:               input.OrderID,
		ClientID:              input.ClientID,
		Direction:             input.Direction,
		Environment:           input.Environment,
		Network:               input.Network,
		Currency:              input.Currency,
		AssetCode:             input.AssetCode,
		AssetIssuer:           input.AssetIssuer,
		AssetAmount:           input.AssetAmount,
		AssetAmountStroops:    input.AssetAmountStroops,
		GrossAmountMinor:      input.GrossAmountMinor,
		FeeAmountMinor:        0,
		PlatformRevenueMinor:  0,
		DeveloperRevenueMinor: 0,
		NetAmountMinor:        input.GrossAmountMinor,
		FeeCurrency:           input.Currency,
		FeePolicyVersion:      SandboxFeePolicyVersion,
		Source:                SandboxFinancialSource,
		QuoteSpreadBPS:        input.QuoteSpreadBPS,
		Simulated:             true,
		CreatedAt:             createdAt,
	}, nil
}
