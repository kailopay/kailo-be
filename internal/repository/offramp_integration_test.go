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
		OrderID: orderID, Principal: integrationAPIPrincipal(),
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
	repo, db := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-000000000301"

	if err := repo.CreateOfframp(ctx, offrampRecord(orderID, 40_000_000, now)); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}

	view, err := repo.Get(ctx, integrationAPIPrincipal(), orderID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if view.Status != entity.OrderStatusAssetPending || view.StellarDestination != testDestination(9) || view.StellarMemo != usecase.OfframpDepositMemo(orderID) {
		t.Fatalf("view status/destination/memo = %s/%q/%q", view.Status, view.StellarDestination, view.StellarMemo)
	}
	if view.DepositTransactionHash != "" {
		t.Fatal("fresh order must not have a deposit hash yet")
	}
	order := loadOrder(t, db, orderID)
	if order.Version != 2 {
		t.Fatalf("order version = %d, want 2", order.Version)
	}
	var financial entity.OrderFinancial
	if err := db.Where("order_id = ?", orderID).First(&financial).Error; err != nil {
		t.Fatalf("loading financial snapshot: %v", err)
	}
	if financial.Direction != "offramp" || financial.GrossAmountMinor != 100_000 || financial.AssetAmountStroops != 40_000_000 || !financial.Simulated {
		t.Fatalf("financial snapshot = %#v", financial)
	}
	var events []entity.OrderEvent
	if err := db.Where("order_id = ?", orderID).Order("aggregate_version").Find(&events).Error; err != nil {
		t.Fatalf("loading order events: %v", err)
	}
	if len(events) != 2 || events[0].AggregateVersion != 1 || events[1].AggregateVersion != 2 {
		t.Fatalf("order event versions = %+v, want [1 2]", events)
	}
	var webhookEvents []entity.WebhookEvent
	if err := db.Where("order_id = ?", orderID).Find(&webhookEvents).Error; err != nil {
		t.Fatalf("loading webhook events: %v", err)
	}
	if len(webhookEvents) != 1 || webhookEvents[0].EventType != usecase.EventOrderCreated ||
		webhookEvents[0].APIVersion != usecase.WebhookAPIVersion || len(webhookEvents[0].CanonicalPayload) == 0 {
		t.Fatalf("webhook events = %+v, want one order.created event", webhookEvents)
	}
	var webhookOutbox entity.OutboxMessage
	if err := db.Where("topic = ? AND aggregate_id = ?", "webhook.deliver", webhookEvents[0].ID).First(&webhookOutbox).Error; err != nil {
		t.Fatalf("loading webhook outbox message: %v", err)
	}

	// Replay with the same key returns the order; conflicting request hash errors.
	_, found, err := repo.FindOfframpReplay(ctx, integrationAPIPrincipal(),
		"off-keyhash-"+orderID, "off-requesthash-"+orderID)
	if err != nil || !found {
		t.Fatalf("FindOfframpReplay() = %v/%v, want found", found, err)
	}
	if _, _, err := repo.FindOfframpReplay(ctx, integrationAPIPrincipal(),
		"off-keyhash-"+orderID, "different"); err != usecase.ErrIdempotencyConflict {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestExpiredOfframpIsExcludedAndCanBeExpired(t *testing.T) {
	repo, db := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-000000000305"
	record := offrampRecord(orderID, 40_000_000, now)
	record.ExpiresAt = now.Add(-time.Minute)

	if err := repo.CreateOfframp(ctx, record); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}
	candidates, err := repo.FindDepositCandidates(ctx, testDestination(9), 50)
	if err != nil {
		t.Fatalf("FindDepositCandidates() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("FindDepositCandidates() = %d, want 0 for expired order", len(candidates))
	}

	expired, err := repo.ExpirePendingDeposits(ctx, now, 50)
	if err != nil {
		t.Fatalf("ExpirePendingDeposits() error = %v", err)
	}
	if expired != 1 {
		t.Fatalf("ExpirePendingDeposits() = %d, want 1", expired)
	}
	view, err := repo.Get(ctx, integrationAPIPrincipal(), orderID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if view.Status != entity.OrderStatusExpired {
		t.Fatalf("status = %s, want expired", view.Status)
	}
	order := loadOrder(t, db, orderID)
	if order.Version != 3 {
		t.Fatalf("expired order version = %d, want 3", order.Version)
	}
	var events []entity.OrderEvent
	if err := db.Where("order_id = ?", orderID).Order("aggregate_version").Find(&events).Error; err != nil {
		t.Fatalf("loading expired order events: %v", err)
	}
	if len(events) != 3 || events[2].EventType != "order.expired" || events[2].AggregateVersion != 3 {
		t.Fatalf("expired order events = %+v, want order.expired at version 3", events)
	}
}

func TestOfframpOwnershipScopesHistoryAndReplayByRetailUser(t *testing.T) {
	repo, _ := newOfframpIntegration(t)
	ctx := context.Background()
	now := time.Now().UTC()
	first := offrampRecord("00000000-0000-4000-8000-000000000304", 40_000_000, now)
	first.Principal = integrationRetailPrincipal("00000000-0000-4000-8000-0000000000aa", "00000000-0000-4000-8000-0000000000d1")
	first.IdempotencyKeyHash = "shared-off-retail-key"
	first.RequestHash = "shared-off-retail-request"
	if err := repo.CreateOfframp(ctx, first); err != nil {
		t.Fatalf("CreateOfframp(retail A) error = %v", err)
	}

	if _, err := repo.Get(ctx, integrationRetailPrincipal("00000000-0000-4000-8000-0000000000aa", "00000000-0000-4000-8000-0000000000d2"), first.OrderID); err != nil {
		t.Fatalf("Get(retail A renewed session) error = %v", err)
	}
	if _, err := repo.Get(ctx, integrationRetailPrincipal("00000000-0000-4000-8000-0000000000ab", "00000000-0000-4000-8000-0000000000d3"), first.OrderID); err != usecase.ErrOrderNotFound {
		t.Fatalf("Get(retail B) error = %v, want ErrOrderNotFound", err)
	}
	if _, err := repo.Get(ctx, integrationAPIPrincipal(), first.OrderID); err != usecase.ErrOrderNotFound {
		t.Fatalf("Get(API client) error = %v, want ErrOrderNotFound", err)
	}

	if _, found, err := repo.FindOfframpReplay(ctx, integrationRetailPrincipal("00000000-0000-4000-8000-0000000000aa", "00000000-0000-4000-8000-0000000000d2"), first.IdempotencyKeyHash, first.RequestHash); err != nil || !found {
		t.Fatalf("FindOfframpReplay(retail A renewed session) = %v/%v, want found", found, err)
	}
	if _, found, err := repo.FindOfframpReplay(ctx, integrationRetailPrincipal("00000000-0000-4000-8000-0000000000ab", "00000000-0000-4000-8000-0000000000d3"), first.IdempotencyKeyHash, first.RequestHash); err != nil || found {
		t.Fatalf("FindOfframpReplay(retail B) = %v/%v, want not found", found, err)
	}

	second := first
	second.OrderID = "00000000-0000-4000-8000-000000000305"
	second.Principal = integrationRetailPrincipal("00000000-0000-4000-8000-0000000000ab", "00000000-0000-4000-8000-0000000000d3")
	if err := repo.CreateOfframp(ctx, second); err != nil {
		t.Fatalf("CreateOfframp(retail B same key) error = %v", err)
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

	view, _ := repo.Get(ctx, integrationAPIPrincipal(), orderID)
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

func TestRetirementConfirmQueuesAndCompletesSandboxPayoutIdempotently(t *testing.T) {
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
	view, _ := repo.Get(ctx, integrationAPIPrincipal(), orderID)
	if view.Status != entity.OrderStatusWithdrawalProcessing {
		t.Fatalf("status = %s, want withdrawal_processing", view.Status)
	}

	view, _ = repo.Get(ctx, integrationAPIPrincipal(), orderID)
	if view.Status != entity.OrderStatusWithdrawalProcessing || view.Payout != nil {
		t.Fatalf("post-retirement view = %+v, want withdrawal_processing without payout before worker", view)
	}
	var webhookEvents []entity.WebhookEvent
	if err := db.Joins("JOIN order_events ON order_events.id = webhook_events.source_order_event_id").
		Where("webhook_events.order_id = ?", orderID).Order("order_events.aggregate_version").Find(&webhookEvents).Error; err != nil {
		t.Fatalf("loading lifecycle webhook events: %v", err)
	}
	wantTypes := []string{usecase.EventOrderCreated, usecase.EventOrderAssetReceived, usecase.EventOrderProcessing}
	if len(webhookEvents) != len(wantTypes) {
		t.Fatalf("webhook event count = %d, want %d", len(webhookEvents), len(wantTypes))
	}
	for index, wantType := range wantTypes {
		if webhookEvents[index].EventType != wantType {
			t.Fatalf("webhook event %d type = %q, want %q", index, webhookEvents[index].EventType, wantType)
		}
	}
	if countRowsForTopic(t, db, "stellar.retire_offramp") != 1 {
		t.Fatal("expected one retirement outbox row")
	}
	if countRowsForTopic(t, db, usecase.SandboxPayoutTopic) != 1 {
		t.Fatal("expected one sandbox payout outbox row")
	}
	var payoutOutbox entity.OutboxMessage
	if err := db.Where("topic = ? AND aggregate_id = ?", usecase.SandboxPayoutTopic, orderID).First(&payoutOutbox).Error; err != nil {
		t.Fatalf("loading payout outbox message: %v", err)
	}
	worker, err := usecase.NewSandboxPayoutWorker(repo, usecase.SandboxPayoutSimulated, func() time.Time { return retireAt.Add(time.Minute) })
	if err != nil {
		t.Fatalf("NewSandboxPayoutWorker() error = %v", err)
	}
	job := usecase.Job{OutboxID: payoutOutbox.ID, IntentID: orderID}
	if err := worker.RunOnce(ctx, job); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if err := worker.RunOnce(ctx, job); err != nil {
		t.Fatalf("replayed RunOnce() error = %v", err)
	}
	view, err = repo.Get(ctx, integrationAPIPrincipal(), orderID)
	if err != nil {
		t.Fatalf("Get(completed) error = %v", err)
	}
	if view.Status != entity.OrderStatusCompleted || view.Payout == nil || view.Payout.Reference != usecase.SandboxPayoutReference(orderID) || view.Payout.Simulated == nil || !*view.Payout.Simulated {
		t.Fatalf("completed payout view = %+v", view)
	}
	if view.Payout.Disclosure != usecase.SandboxPayoutAfterConfirmedRetirementDisclosure {
		t.Fatalf("payout disclosure = %q", view.Payout.Disclosure)
	}
	var payouts []entity.OfframpPayout
	if err := db.Where("order_id = ?", orderID).Find(&payouts).Error; err != nil {
		t.Fatalf("loading payout rows: %v", err)
	}
	if len(payouts) != 1 {
		t.Fatalf("payout rows = %d, want 1", len(payouts))
	}
	var processed entity.OutboxMessage
	if err := db.First(&processed, "id = ?", payoutOutbox.ID).Error; err != nil || processed.ProcessedAt == nil {
		t.Fatalf("payout outbox processed = %v/%v, want timestamp", processed.ProcessedAt, err)
	}
}
