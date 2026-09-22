package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

func TestSEP38QuoteLifecycleIsWalletOwnedAndSingleUse(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	repository := &sep38QuoteRepositoryFake{}
	service, err := NewSep38QuoteUsecase(Sep38QuoteDependencies{
		Prices: sep38PriceReaderFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now}},
		Quotes: repository,
	}, Sep38QuoteConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute, SpreadBPS: 100},
		MinIDR:      10_000, MaxIDR: 10_000_000,
		Now:   func() time.Time { return now },
		NewID: func() (string, error) { return "00000000-0000-4000-8000-000000000401", nil },
	})
	if err != nil {
		t.Fatalf("NewSep38QuoteUsecase() error = %v", err)
	}

	principal := SEP10Principal{Account: "GACCOUNT"}
	created, err := service.Create(context.Background(), principal, Sep38QuoteRequest{
		SellAsset: Sep38IDRAsset, BuyAsset: Sep38XLMAsset, SellAmount: "100000",
		SellDeliveryMethod: "xendit",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.QuoteID == "" || created.SellAmount != "100000" || created.BuyAmount == "" ||
		created.SpreadBPS != 100 || created.SellDeliveryMethod != "xendit" {
		t.Fatalf("created quote = %+v", created)
	}

	loaded, err := service.Get(context.Background(), principal, created.QuoteID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.QuoteID != created.QuoteID || loaded.BuyAmount != created.BuyAmount {
		t.Fatalf("loaded quote = %+v, want %+v", loaded, created)
	}

	if err := service.Consume(context.Background(), principal, created.QuoteID); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if _, err := service.Get(context.Background(), principal, created.QuoteID); !errors.Is(err, ErrSEP38QuoteConsumed) {
		t.Fatalf("Get(consumed) error = %v, want %v", err, ErrSEP38QuoteConsumed)
	}
	if _, err := service.Get(context.Background(), SEP10Principal{Account: "GOTHER"}, created.QuoteID); !errors.Is(err, ErrSEP38QuoteNotFound) {
		t.Fatalf("Get(other wallet) error = %v, want %v", err, ErrSEP38QuoteNotFound)
	}
}

func TestSEP38QuoteRejectsExpiredQuote(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	repository := &sep38QuoteRepositoryFake{}
	service, err := NewSep38QuoteUsecase(Sep38QuoteDependencies{
		Prices: sep38PriceReaderFake{market: MarketPrice{IDRPerXLM: "2500", ObservedAt: now}}, Quotes: repository,
	}, Sep38QuoteConfig{
		QuotePolicy: QuotePolicy{TTL: time.Minute, MaxAge: 2 * time.Minute}, MinIDR: 10_000, MaxIDR: 10_000_000,
		Now: func() time.Time { return now }, NewID: func() (string, error) { return "00000000-0000-4000-8000-000000000402", nil },
	})
	if err != nil {
		t.Fatalf("NewSep38QuoteUsecase() error = %v", err)
	}
	created, err := service.Create(context.Background(), SEP10Principal{Account: "GACCOUNT"}, Sep38QuoteRequest{
		SellAsset: Sep38IDRAsset, BuyAsset: Sep38XLMAsset, SellAmount: "100000",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	service.config.Now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := service.Get(context.Background(), SEP10Principal{Account: "GACCOUNT"}, created.QuoteID); !errors.Is(err, ErrSEP38QuoteExpired) {
		t.Fatalf("Get(expired) error = %v, want %v", err, ErrSEP38QuoteExpired)
	}
}

type sep38QuoteRepositoryFake struct {
	quote entity.SEP38Quote
}

func (r *sep38QuoteRepositoryFake) Create(_ context.Context, quote entity.SEP38Quote) error {
	r.quote = quote
	return nil
}

func (r *sep38QuoteRepositoryFake) Find(_ context.Context, walletAccount, quoteID string) (entity.SEP38Quote, error) {
	if r.quote.QuoteID == "" || r.quote.QuoteID != quoteID || r.quote.WalletAccount != walletAccount {
		return entity.SEP38Quote{}, ErrSEP38QuoteNotFound
	}
	return r.quote, nil
}

func (r *sep38QuoteRepositoryFake) Consume(_ context.Context, walletAccount, quoteID string, now time.Time) error {
	quote, err := r.Find(context.Background(), walletAccount, quoteID)
	if err != nil {
		return err
	}
	if quote.ConsumedAt != nil {
		return ErrSEP38QuoteConsumed
	}
	if !now.Before(quote.ExpiresAt) {
		return ErrSEP38QuoteExpired
	}
	consumedAt := now
	r.quote.ConsumedAt = &consumedAt
	return nil
}
