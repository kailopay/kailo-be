package onramp

import (
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrInvalidPrice = errors.New("invalid market price")
	ErrStalePrice   = errors.New("stale market price")
	ErrInvalidQuote = errors.New("invalid quote")
)

type MarketPrice struct {
	IDRPerXLM  string
	ObservedAt time.Time
}

type Quote struct {
	FiatAmount   entity.IDR
	AssetAmount  entity.Stroops
	Rate         string
	AdjustedRate string
	SpreadBPS    int
	SourceAt     time.Time
	ExpiresAt    time.Time
}

type QuotePolicy struct {
	TTL       time.Duration
	MaxAge    time.Duration
	SpreadBPS int
}

func (p QuotePolicy) Create(now time.Time, amount entity.IDR, market MarketPrice) (Quote, error) {
	if p.TTL <= 0 || p.MaxAge <= 0 || p.SpreadBPS < 0 || p.SpreadBPS > 10_000 || amount.Validate() != nil {
		return Quote{}, ErrInvalidQuote
	}
	now = now.UTC()
	observedAt := market.ObservedAt.UTC()
	if observedAt.IsZero() || now.Sub(observedAt) > p.MaxAge || observedAt.After(now.Add(time.Minute)) {
		return Quote{}, ErrStalePrice
	}
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(market.IDRPerXLM))
	if !ok || rate.Sign() <= 0 {
		return Quote{}, ErrInvalidPrice
	}
	spreadMultiplier := new(big.Rat).SetFrac64(int64(10_000+p.SpreadBPS), 10_000)
	adjustedRate := new(big.Rat).Mul(rate, spreadMultiplier)

	stroopsNumerator := new(big.Int).Mul(big.NewInt(int64(amount)), big.NewInt(int64(entity.StroopsPerXLM)))
	stroopsNumerator.Mul(stroopsNumerator, adjustedRate.Denom())
	stroops := new(big.Int).Quo(stroopsNumerator, adjustedRate.Num())
	if !stroops.IsInt64() || stroops.Sign() <= 0 {
		return Quote{}, ErrInvalidQuote
	}
	return Quote{
		FiatAmount: amount, AssetAmount: entity.Stroops(stroops.Int64()),
		Rate: decimal(rate), AdjustedRate: decimal(adjustedRate), SpreadBPS: p.SpreadBPS,
		SourceAt: observedAt, ExpiresAt: now.Add(p.TTL),
	}, nil
}

func decimal(value *big.Rat) string {
	formatted := value.FloatString(18)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	return formatted
}
