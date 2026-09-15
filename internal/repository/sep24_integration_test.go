package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestSEP24RepositoryPersistsAndScopesOrderMapping(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000e1"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodQRIS, 100_000, 40_000_000, testFutureExpiry()), 1_000_000_000); err != nil {
		t.Fatalf("creating order fixture: %v", err)
	}

	repository := NewSEP24Repository(store.db)
	principal := integrationAPIPrincipal()
	record := usecase.Sep24TransactionRecord{
		Principal: principal, TransactionID: "deposit-" + orderID, OrderID: orderID, Kind: usecase.Sep24KindDeposit,
	}
	if err := repository.Create(ctx, record); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	found, err := repository.Find(ctx, principal, record.TransactionID)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found.OrderID != orderID || found.Kind != usecase.Sep24KindDeposit {
		t.Fatalf("mapping = %+v", found)
	}
	wrongPrincipal := usecase.OrderPrincipal{Kind: usecase.OrderPrincipalAPIClient, ClientID: "00000000-0000-4000-8000-0000000000c2", OwnerUserID: "00000000-0000-4000-8000-0000000000ab"}
	if _, err := repository.Find(ctx, wrongPrincipal, record.TransactionID); !errors.Is(err, usecase.ErrSEP24TransactionNotFound) {
		t.Fatalf("wrong-owner Find() error = %v, want %v", err, usecase.ErrSEP24TransactionNotFound)
	}
}

func TestSEP24RepositoryRejectsDirectionMismatch(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000e2"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodQRIS, 100_000, 40_000_000, testFutureExpiry()), 1_000_000_000); err != nil {
		t.Fatalf("creating order fixture: %v", err)
	}

	repository := NewSEP24Repository(store.db)
	err := repository.Create(ctx, usecase.Sep24TransactionRecord{
		Principal: integrationAPIPrincipal(), TransactionID: "withdraw-" + orderID, OrderID: orderID, Kind: usecase.Sep24KindWithdraw,
	})
	if !errors.Is(err, usecase.ErrSEP24TransactionConflict) {
		t.Fatalf("Create() error = %v, want %v", err, usecase.ErrSEP24TransactionConflict)
	}
}

func testFutureExpiry() time.Time {
	return time.Now().UTC().Add(5 * time.Minute)
}
