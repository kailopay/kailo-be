package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type sep38PriceReaderFake struct {
	market MarketPrice
	err    error
}

func (f sep38PriceReaderFake) LatestXLMIDR(context.Context) (MarketPrice, error) {
	return f.market, f.err
}

func TestSep38PriceUsesSharedQuotePolicyForBothDirections(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	service, err := NewSep38Usecase(sep38PriceReaderFake{market: MarketPrice{
		IDRPerXLM: "2500", ObservedAt: now,
	}}, Sep38UsecaseConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 100},
		MinIDR:      10_000,
		MaxIDR:      10_000_000,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSep38Usecase() error = %v", err)
	}

	tests := []struct {
		name      string
		request   Sep38PriceRequest
		wantPrice string
		wantTotal string
		wantSell  string
		wantBuy   string
	}{
		{
			name:      "IDR to XLM",
			request:   Sep38PriceRequest{SellAsset: Sep38IDRAsset, BuyAsset: Sep38XLMAsset, SellAmount: "100000"},
			wantPrice: "2500", wantTotal: "2525", wantSell: "100000", wantBuy: "39.6039603",
		},
		{
			name:      "XLM to IDR",
			request:   Sep38PriceRequest{SellAsset: Sep38XLMAsset, BuyAsset: Sep38IDRAsset, SellAmount: "40"},
			wantPrice: "0.0004", wantTotal: "0.000396039603960396", wantSell: "40.0000000", wantBuy: "101000",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			price, err := service.Price(context.Background(), test.request)
			if err != nil {
				t.Fatalf("Price() error = %v", err)
			}
			if price.Price != test.wantPrice || price.TotalPrice != test.wantTotal ||
				price.SellAmount != test.wantSell || price.BuyAmount != test.wantBuy {
				t.Fatalf("price = %+v", price)
			}
		})
	}
}

func TestSep38RejectsInvalidPairAndAmountShape(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	service, err := NewSep38Usecase(sep38PriceReaderFake{market: MarketPrice{
		IDRPerXLM: "2500", ObservedAt: now,
	}}, Sep38UsecaseConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000,
		MaxIDR:      10_000_000,
		Now:         func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSep38Usecase() error = %v", err)
	}

	for _, test := range []struct {
		name    string
		request Sep38PriceRequest
	}{
		{name: "unsupported pair", request: Sep38PriceRequest{SellAsset: "iso4217:USD", BuyAsset: Sep38XLMAsset, SellAmount: "100"}},
		{name: "both amounts", request: Sep38PriceRequest{SellAsset: Sep38IDRAsset, BuyAsset: Sep38XLMAsset, SellAmount: "100000", BuyAmount: "40"}},
		{name: "fractional IDR", request: Sep38PriceRequest{SellAsset: Sep38IDRAsset, BuyAsset: Sep38XLMAsset, SellAmount: "100000.5"}},
		{name: "invalid XLM precision", request: Sep38PriceRequest{SellAsset: Sep38XLMAsset, BuyAsset: Sep38IDRAsset, SellAmount: "1.00000000"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.Price(context.Background(), test.request); !errors.Is(err, ErrInvalidSEP38Request) {
				t.Fatalf("Price() error = %v, want %v", err, ErrInvalidSEP38Request)
			}
		})
	}
}

func TestSep38RejectsXLMAmountOutsideIDRRange(t *testing.T) {
	service, err := NewSep38Usecase(sep38PriceReaderFake{market: MarketPrice{
		IDRPerXLM: "2500", ObservedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}}, Sep38UsecaseConfig{
		QuotePolicy: QuotePolicy{TTL: time.Minute, MaxAge: time.Hour, SpreadBPS: 100},
		MinIDR:      100_000, MaxIDR: 1_000_000, Now: func() time.Time {
			return time.Date(2026, 9, 16, 0, 1, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewSep38Usecase() error = %v", err)
	}

	_, err = service.Price(context.Background(), Sep38PriceRequest{
		SellAsset: Sep38XLMAsset, BuyAsset: Sep38IDRAsset, SellAmount: "1",
	})
	if !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("Price() error = %v, want ErrAmountOutOfRange", err)
	}
}
