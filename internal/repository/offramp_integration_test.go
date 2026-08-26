package repository

import (
	"context"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

func newOfframpIntegration(t *testing.T) (*OfframpRepository, *gorm.DB) {
	t.Helper()
	store := newIntegrationStore(t)
	return NewOfframpRepository(store.db, testDestination(9), "stellar_testnet"), store.db
}

func offrampRecord(orderID string, stroops int64, now time.Time) usecase.OfframpCreateRecord {
	return usecase.OfframpCreateRecord{
		OrderID: orderID, ClientID: "00000000-0000-4000-8000-0000000000c1",
		IdempotencyKeyHash: "off-keyhash-" + orderID, RequestHash: "off-requesthash-" + orderID,
		AssetAmount: entity.Stroops(stroops),
		Quote: usecase.Quote{FiatAmount: 100_000, AssetAmount: entity.Stroops(stroops),
			Rate: "2500", AdjustedRate: "2500", SpreadBPS: 0, SourceAt: now, ExpiresAt: now.Add(5 * time.Minute)},
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer,
		DestinationToken: []byte(`{"token":"demo"}`),
		Memo:             usecase.OfframpDepositMemo(orderID),
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
	}
}

func TestCreateOfframpPersistsInstructionsAndReplays(t *testing.T) {
	repo, _ := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-000000000301"

	if err := repo.CreateOfframp(ctx, offrampRecord(orderID, 40_000_000, now)); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}

	view, err := repo.Get(ctx, "00000000-0000-4000-8000-0000000000c1", orderID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if view.Status != entity.OrderStatusAssetPending || view.StellarMemo != usecase.OfframpDepositMemo(orderID) {
		t.Fatalf("view status/memo = %s/%q", view.Status, view.StellarMemo)
	}
	if view.DepositTransactionHash != "" {
		t.Fatal("fresh order must not have a deposit hash yet")
	}

	// Replay with the same key returns the order; conflicting request hash errors.
	_, found, err := repo.FindOfframpReplay(ctx, "00000000-0000-4000-8000-0000000000c1",
		"off-keyhash-"+orderID, "off-requesthash-"+orderID)
	if err != nil || !found {
		t.Fatalf("FindOfframpReplay() = %v/%v, want found", found, err)
	}
	if _, _, err := repo.FindOfframpReplay(ctx, "00000000-0000-4000-8000-0000000000c1",
		"off-keyhash-"+orderID, "different"); err != usecase.ErrIdempotencyConflict {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestRecordAssetReceivedQueuesRetirementAtomically(t *testing.T) {
	repo, _ := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-000000000302"

	if err := repo.CreateOfframp(ctx, offrampRecord(orderID, 40_000_000, now)); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}
	candidates, err := repo.FindDepositCandidates(ctx, testDestination(9), 50)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("FindDepositCandidates() = %d/%v, want 1", len(candidates), err)
	}

	payment := usecase.ObservedPayment{TransactionHash: "dep-hash-" + orderID, From: testDestination(1),
		To: testDestination(9), Amount: 40_000_000, Memo: usecase.OfframpDepositMemo(orderID),
		LedgerAt: now.Add(time.Minute)}
	if err := repo.RecordAssetReceived(ctx, orderID, payment); err != nil {
		t.Fatalf("RecordAssetReceived() error = %v", err)
	}

	view, _ := repo.Get(ctx, "00000000-0000-4000-8000-0000000000c1", orderID)
	if view.Status != entity.OrderStatusRetirementProcessing {
		t.Fatalf("status = %s, want retirement_processing", view.Status)
	}
	if view.DepositTransactionHash != payment.TransactionHash {
		t.Fatalf("deposit hash = %q", view.DepositTransactionHash)
	}

	intent, err := repo.LoadRetirement(ctx, "stellar-retire-"+orderID)
	if err != nil {
		t.Fatalf("LoadRetirement() error = %v", err)
	}
	if intent.BurnTarget != usecase.BurnAddress || intent.Amount != 40_000_000 || intent.TransactionHash != "" {
		t.Fatalf("intent = %+v", intent)
	}

	// Re-delivery of the same scan must be a no-op.
	if err := repo.RecordAssetReceived(ctx, orderID, payment); err != nil {
		t.Fatalf("replayed RecordAssetReceived() error = %v", err)
	}
}

func TestRetirementConfirmAdvancesToWithdrawalAndPayoutCompletes(t *testing.T) {
	repo, db := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-000000000303"

	if err := repo.CreateOfframp(ctx, offrampRecord(orderID, 40_000_000, now)); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}
	payment := usecase.ObservedPayment{TransactionHash: "dep-hash-" + orderID, From: testDestination(1),
		To: testDestination(9), Amount: 40_000_000, Memo: usecase.OfframpDepositMemo(orderID), LedgerAt: now}
	if err := repo.RecordAssetReceived(ctx, orderID, payment); err != nil {
		t.Fatalf("RecordAssetReceived() error = %v", err)
	}

	retireAt := now.Add(2 * time.Minute)
	if err := repo.SaveRetirementHash(ctx, "stellar-retire-"+orderID, "burn-hash-1", retireAt); err != nil {
		t.Fatalf("SaveRetirementHash() error = %v", err)
	}
	// A hash mismatch must be rejected before any confirmation happens.
	if err := repo.ConfirmRetirement(ctx, "stellar-retire-"+orderID, "wrong-hash", retireAt); err == nil {
		t.Fatal("ConfirmRetirement() with wrong hash must fail")
	}
	if err := repo.ConfirmRetirement(ctx, "stellar-retire-"+orderID, "burn-hash-1", retireAt); err != nil {
		t.Fatalf("ConfirmRetirement() error = %v", err)
	}
	view, _ := repo.Get(ctx, "00000000-0000-4000-8000-0000000000c1", orderID)
	if view.Status != entity.OrderStatusWithdrawalProcessing {
		t.Fatalf("status = %s, want withdrawal_processing", view.Status)
	}

	reference, err := repo.CompleteSimulatedPayout(ctx, orderID, 100_000, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("CompleteSimulatedPayout() error = %v", err)
	}
	if reference != "payout_"+orderID {
		t.Fatalf("reference = %q", reference)
	}
	view, _ = repo.Get(ctx, "00000000-0000-4000-8000-0000000000c1", orderID)
	if view.Status != entity.OrderStatusCompleted || view.Payout == nil || !*view.Payout.Simulated {
		t.Fatalf("final view = %+v", view)
	}
	if countRows(t, db, &entity.OutboxMessage{}) < 2 {
		t.Fatal("expected retirement and payout outbox rows")
	}
}
