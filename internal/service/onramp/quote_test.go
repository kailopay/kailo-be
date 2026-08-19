package onramp

import (
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

func TestQuotePolicyCreatesExactRoundedDownQuote(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	policy := QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 0}

	quote, err := policy.Create(now, entity.IDR(100_000), MarketPrice{IDRPerXLM: "2500", ObservedAt: now})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if quote.AssetAmount != entity.Stroops(400_000_000) {
		t.Fatalf("AssetAmount = %d, want 400000000", quote.AssetAmount)
	}
	if quote.Rate != "2500" || quote.AdjustedRate != "2500" || !quote.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("quote = %+v", quote)
	}
}

func TestQuotePolicyAppliesSpreadBeforeRoundingDown(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	policy := QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 100}

	quote, err := policy.Create(now, entity.IDR(100_000), MarketPrice{IDRPerXLM: "2500", ObservedAt: now})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if quote.AssetAmount != entity.Stroops(396_039_603) || quote.AdjustedRate != "2525" {
		t.Fatalf("quote = %+v", quote)
	}
}

func TestQuotePolicyRejectsStaleAndMalformedPrices(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	policy := QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute}
	tests := []struct {
		name   string
		market MarketPrice
		want   error
	}{
		{name: "stale", market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now.Add(-3 * time.Minute)}, want: ErrStalePrice},
		{name: "malformed", market: MarketPrice{IDRPerXLM: "NaN", ObservedAt: now}, want: ErrInvalidPrice},
		{name: "zero", market: MarketPrice{IDRPerXLM: "0", ObservedAt: now}, want: ErrInvalidPrice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := policy.Create(now, entity.IDR(100_000), tt.market); !errors.Is(err, tt.want) {
				t.Fatalf("Create() error = %v, want %v", err, tt.want)
			}
		})
	}
}
