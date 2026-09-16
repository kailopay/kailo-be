package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type quotePreviewPriceFake struct {
	market MarketPrice
	err    error
}

func (f quotePreviewPriceFake) LatestXLMIDR(context.Context) (MarketPrice, error) {
	return f.market, f.err
}

func TestQuotePreviewUsecasePreviewsBothDirections(t *testing.T) {
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	service, err := NewQuotePreviewUsecase(quotePreviewPriceFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now}}, QuotePreviewServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 100},
		MinIDR:      10_000, MaxIDR: 10_000_000, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewQuotePreviewUsecase() error = %v", err)
	}

	tests := []struct {
		name       string
		command    QuotePreviewCommand
		wantFiat   entity.IDR
		wantAsset  entity.Stroops
		wantRate   string
		wantDirect QuoteDirection
	}{
		{
			name: "buy IDR for XLM", command: QuotePreviewCommand{Principal: apiOrderPrincipal("client-1", "owner-1"), Direction: QuoteDirectionBuy,
				FiatCurrency: IDRCurrency, FiatAmountMinor: 100_000, AssetNetwork: StellarTestnetNetwork, AssetCode: NativeXLMAssetCode},
			wantFiat: 100_000, wantAsset: 396_039_603, wantRate: "2525", wantDirect: QuoteDirectionBuy,
		},
		{
			name: "sell XLM for IDR", command: QuotePreviewCommand{Principal: apiOrderPrincipal("client-1", "owner-1"), Direction: QuoteDirectionSell,
				FiatCurrency: IDRCurrency, AssetNetwork: StellarTestnetNetwork, AssetCode: NativeXLMAssetCode, AssetAmount: "40.0000000"},
			wantFiat: 101_000, wantAsset: 400_000_000, wantRate: "2525", wantDirect: QuoteDirectionSell,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			preview, err := service.Preview(context.Background(), testCase.command)
			if err != nil {
				t.Fatalf("Preview() error = %v", err)
			}
			if preview.Direction != testCase.wantDirect || preview.Quote.FiatAmount != testCase.wantFiat ||
				preview.Quote.AssetAmount != testCase.wantAsset || preview.Quote.AdjustedRate != testCase.wantRate {
				t.Fatalf("Preview() = %+v, want direction=%q fiat=%d asset=%d adjusted_rate=%q", preview, testCase.wantDirect,
					testCase.wantFiat, testCase.wantAsset, testCase.wantRate)
			}
		})
	}
}

func TestQuotePreviewUsecaseRejectsInvalidRequestsAndUnavailablePrices(t *testing.T) {
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	config := QuotePreviewServiceConfig{QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute}, MinIDR: 10_000, MaxIDR: 10_000_000,
		Now: func() time.Time { return now }}
	base := QuotePreviewCommand{Principal: apiOrderPrincipal("client-1", "owner-1"), Direction: QuoteDirectionBuy, FiatCurrency: IDRCurrency,
		FiatAmountMinor: 100_000, AssetNetwork: StellarTestnetNetwork, AssetCode: NativeXLMAssetCode}
	providerErr := errors.New("provider unavailable")

	tests := []struct {
		name   string
		prices PriceReader
		mutate func(*QuotePreviewCommand)
		want   error
	}{
		{name: "unsupported direction", prices: quotePreviewPriceFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now}}, mutate: func(command *QuotePreviewCommand) {
			command.Direction = "convert"
		}, want: ErrInvalidQuoteRequest},
		{name: "unsupported asset", prices: quotePreviewPriceFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now}}, mutate: func(command *QuotePreviewCommand) {
			command.AssetCode = "USDC"
		}, want: ErrInvalidQuoteRequest},
		{name: "stale price", prices: quotePreviewPriceFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now.Add(-3 * time.Minute)}}, mutate: func(*QuotePreviewCommand) {
		}, want: ErrStalePrice},
		{name: "price unavailable", prices: quotePreviewPriceFake{err: providerErr}, mutate: func(*QuotePreviewCommand) {
		}, want: providerErr,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service, err := NewQuotePreviewUsecase(testCase.prices, config)
			if err != nil {
				t.Fatalf("NewQuotePreviewUsecase() error = %v", err)
			}
			command := base
			testCase.mutate(&command)
			_, err = service.Preview(context.Background(), command)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Preview() error = %v, want %v", err, testCase.want)
			}
		})
	}
}
