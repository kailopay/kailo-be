package repository

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These tests run the versioned SQL migrations and repository behavior against
// a real PostgreSQL instance. Set TEST_DATABASE_DSN to a disposable database;
// the schema is migrated once and every test truncates all tables first.
//
//	TEST_DATABASE_DSN="postgres://postgres:postgres@localhost:5432/kailopay_test?sslmode=disable&TimeZone=UTC"

var integrationTables = []string{
	"webhook_attempts", "webhook_events", "webhook_endpoints", "outbox_messages",
	"idempotency_records", "stellar_transactions", "gateway_events", "payment_checkouts",
	"order_events", "treasury_reservations", "treasury_accounts", "orders", "api_keys",
	"api_clients", "retail_sessions", "auth_transactions", "user_identities", "users",
}

type integrationStore struct {
	db         *gorm.DB
	onramp     *OnrampRepository
	settlement *SettlementRepository
	auth       *AuthRepository
}

func newIntegrationStore(t *testing.T) *integrationStore {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set; skipping PostgreSQL integration tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("accessing test database handle: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := platform.ApplyMigrations(ctx, sqlDB, os.DirFS("../../migrations")); err != nil {
		t.Fatalf("applying migrations: %v", err)
	}
	for _, table := range integrationTables {
		if err := db.Exec("TRUNCATE TABLE " + table + " RESTART IDENTITY CASCADE").Error; err != nil {
			t.Fatalf("truncating %s: %v", table, err)
		}
	}
	seed := []string{
		`INSERT INTO users (id, status, display_name, created_at, updated_at)
		 VALUES ('00000000-0000-4000-8000-0000000000aa', 'active', 'Integration Tester', now(), now())`,
		`INSERT INTO users (id, status, display_name, created_at, updated_at)
		 VALUES ('00000000-0000-4000-8000-0000000000ab', 'active', 'Second Integration Tester', now(), now())`,
		`INSERT INTO api_clients (id, owner_user_id, name, environment, status, created_at, updated_at)
		 VALUES ('00000000-0000-4000-8000-0000000000c1', '00000000-0000-4000-8000-0000000000aa', 'integration', 'test', 'active', now(), now())`,
		`INSERT INTO retail_sessions (id, user_id, token_hash, expires_at, created_at, updated_at)
		 VALUES
		 ('00000000-0000-4000-8000-0000000000d1', '00000000-0000-4000-8000-0000000000aa', decode('01', 'hex'), now() + interval '1 day', now(), now()),
		 ('00000000-0000-4000-8000-0000000000d2', '00000000-0000-4000-8000-0000000000aa', decode('02', 'hex'), now() + interval '1 day', now(), now()),
		 ('00000000-0000-4000-8000-0000000000d3', '00000000-0000-4000-8000-0000000000ab', decode('03', 'hex'), now() + interval '1 day', now(), now())`,
	}
	for _, statement := range seed {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seeding fixture: %v", err)
		}
	}
	return &integrationStore{
		db:         db,
		onramp:     NewOnrampRepository(db, "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF", "stellar_testnet", 0),
		settlement: NewSettlementRepository(db, 5),
		auth:       NewAuthRepository(db, 30*time.Minute),
	}
}

func testDestination(n int) string {
	return fmt.Sprintf("GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA%03d", n)
}

func testCreateRecord(orderID string, method entity.PaymentMethod, amount entity.IDR, stroops entity.Stroops, expiresAt time.Time) usecase.CreateRecord {
	now := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	return usecase.CreateRecord{
		OrderID: orderID, Principal: integrationAPIPrincipal(),
		IdempotencyKeyHash: "keyhash-" + orderID, RequestHash: "requesthash-" + orderID,
		PaymentMethod: method, Destination: testDestination(1), Quote: usecase.Quote{
			FiatAmount: amount, AssetAmount: stroops, Rate: "2500", AdjustedRate: "2500",
			SpreadBPS: 0, SourceAt: now, ExpiresAt: expiresAt,
		},
		CreatedAt: now,
	}
}

func integrationAPIPrincipal() usecase.OrderPrincipal {
	return usecase.OrderPrincipal{Kind: usecase.OrderPrincipalAPIClient,
		ClientID: "00000000-0000-4000-8000-0000000000c1", OwnerUserID: "00000000-0000-4000-8000-0000000000aa"}
}

func integrationRetailPrincipal(userID, sessionID string) usecase.OrderPrincipal {
	return usecase.OrderPrincipal{Kind: usecase.OrderPrincipalRetailSession, OwnerUserID: userID, SessionID: sessionID}
}

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var count int64
	if err := db.Model(model).Count(&count).Error; err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return count
}

func TestReserveAndCreatePersistsQRISAndBRIVAOrders(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(5 * time.Minute)

	orders := map[entity.PaymentMethod]string{
		entity.PaymentMethodQRIS:  "00000000-0000-4000-8000-0000000000a1",
		entity.PaymentMethodBRIVA: "00000000-0000-4000-8000-0000000000b1",
	}
	for method, orderID := range orders {
		if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, method, 100_000, 40_000_000, future), 1_000_000_000); err != nil {
			t.Fatalf("ReserveAndCreate(%s) error = %v", method, err)
		}
	}
	if count := countRows(t, store.db, &entity.OrderRecord{}); count != 2 {
		t.Fatalf("orders count = %d, want 2", count)
	}
	if count := countRows(t, store.db, &entity.TreasuryReservation{}); count != 2 {
		t.Fatalf("reservations count = %d, want 2", count)
	}
	if count := countRows(t, store.db, &entity.OrderEvent{}); count != 2 {
		t.Fatalf("order events count = %d, want 2", count)
	}
	var treasury entity.TreasuryAccount
	if err := store.db.First(&treasury).Error; err != nil {
		t.Fatalf("loading treasury: %v", err)
	}
	if treasury.ReservedStroops != 80_000_000 {
		t.Fatalf("reserved stroops = %d, want 80000000", treasury.ReservedStroops)
	}
}

func TestReserveAndCreateRejectsInsufficientLiquidity(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	record := testCreateRecord("00000000-0000-4000-8000-0000000000a2", entity.PaymentMethodQRIS, 100_000, 900_000_000_000, time.Now().UTC().Add(5*time.Minute))

	err := store.onramp.ReserveAndCreate(ctx, record, 1_000_000_000)
	if err != usecase.ErrInsufficientLiquidity {
		t.Fatalf("ReserveAndCreate() error = %v, want ErrInsufficientLiquidity", err)
	}
	if count := countRows(t, store.db, &entity.OrderRecord{}); count != 0 {
		t.Fatalf("orders count = %d, want 0", count)
	}
}

func TestFindReplayReturnsConflictForDifferentBody(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(5 * time.Minute)
	record := testCreateRecord("00000000-0000-4000-8000-0000000000b2", entity.PaymentMethodQRIS, 100_000, 40_000_000, future)
	if err := store.onramp.ReserveAndCreate(ctx, record, 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}

	_, found, err := store.onramp.FindReplay(ctx, record.Principal, record.IdempotencyKeyHash, "different-request-hash")
	if err != usecase.ErrIdempotencyConflict || found {
		t.Fatalf("FindReplay() = %v, %v; want ErrIdempotencyConflict, false", found, err)
	}
	_, found, err = store.onramp.FindReplay(ctx, record.Principal, record.IdempotencyKeyHash, record.RequestHash)
	if err != nil || !found {
		t.Fatalf("FindReplay() = %v, %v; want replay, nil", found, err)
	}
}

func TestConsumerOrderOwnershipScopesHistoryAndReplayByUser(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	future := time.Now().UTC().Add(5 * time.Minute)
	userA := "00000000-0000-4000-8000-0000000000aa"
	userB := "00000000-0000-4000-8000-0000000000ab"
	retailA := integrationRetailPrincipal(userA, "00000000-0000-4000-8000-0000000000d1")
	retailARenewed := integrationRetailPrincipal(userA, "00000000-0000-4000-8000-0000000000d2")
	retailB := integrationRetailPrincipal(userB, "00000000-0000-4000-8000-0000000000d3")

	recordA := testCreateRecord("00000000-0000-4000-8000-0000000000b3", entity.PaymentMethodQRIS, 100_000, 40_000_000, future)
	recordA.Principal = retailA
	recordA.IdempotencyKeyHash = "consumer-shared-key"
	recordA.RequestHash = "consumer-shared-request"
	if err := store.onramp.ReserveAndCreate(ctx, recordA, 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate(retail A) error = %v", err)
	}

	if _, err := store.onramp.Get(ctx, retailA, recordA.OrderID); err != nil {
		t.Fatalf("Get(retail A) error = %v", err)
	}
	if _, err := store.onramp.Get(ctx, retailARenewed, recordA.OrderID); err != nil {
		t.Fatalf("Get(retail A renewed session) error = %v", err)
	}
	if _, err := store.onramp.Get(ctx, retailB, recordA.OrderID); err != usecase.ErrOrderNotFound {
		t.Fatalf("Get(retail B) error = %v, want ErrOrderNotFound", err)
	}
	if _, err := store.onramp.Get(ctx, integrationAPIPrincipal(), recordA.OrderID); err != usecase.ErrOrderNotFound {
		t.Fatalf("Get(api client) error = %v, want ErrOrderNotFound", err)
	}

	orders, _, err := store.onramp.List(ctx, retailARenewed, 20, "")
	if err != nil || len(orders) != 1 || orders[0].ID != recordA.OrderID {
		t.Fatalf("List(retail A renewed session) = %d/%v, want one order", len(orders), err)
	}
	orders, _, err = store.onramp.List(ctx, retailB, 20, "")
	if err != nil || len(orders) != 0 {
		t.Fatalf("List(retail B) = %d/%v, want empty", len(orders), err)
	}

	if _, found, err := store.onramp.FindReplay(ctx, retailARenewed, recordA.IdempotencyKeyHash, recordA.RequestHash); err != nil || !found {
		t.Fatalf("FindReplay(retail A renewed session) = %v/%v, want found", found, err)
	}
	if _, found, err := store.onramp.FindReplay(ctx, retailB, recordA.IdempotencyKeyHash, recordA.RequestHash); err != nil || found {
		t.Fatalf("FindReplay(retail B) = %v/%v, want not found", found, err)
	}

	recordB := testCreateRecord("00000000-0000-4000-8000-0000000000b4", entity.PaymentMethodQRIS, 100_000, 40_000_000, future)
	recordB.Principal = retailB
	recordB.IdempotencyKeyHash = recordA.IdempotencyKeyHash
	recordB.RequestHash = recordA.RequestHash
	if err := store.onramp.ReserveAndCreate(ctx, recordB, 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate(retail B same key) error = %v", err)
	}
}

func TestSharedOrderViewIncludesOfframpSettlementDetails(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	orderID := "00000000-0000-4000-8000-0000000000b5"
	offramp := NewOfframpRepository(store.db, testDestination(9), "stellar_testnet")
	if err := offramp.CreateOfframp(ctx, offrampRecord(orderID, 40_000_000, now)); err != nil {
		t.Fatalf("CreateOfframp() error = %v", err)
	}

	depositID, err := platform.NewID()
	if err != nil {
		t.Fatalf("generating deposit id: %v", err)
	}
	depositHash := "deposit-hash-" + orderID
	if err := store.db.Create(&entity.StellarTransaction{
		ID: depositID, OrderID: orderID, IntentID: "stellar-offramp-deposit-" + orderID,
		Purpose: "deposit", Network: "stellar_testnet", AssetCode: "XLM", Amount: "400000000",
		TransactionHash: &depositHash, Status: "confirmed", AttemptCount: 1, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("creating deposit evidence: %v", err)
	}
	payoutID, err := platform.NewID()
	if err != nil {
		t.Fatalf("generating payout id: %v", err)
	}
	payout := entity.OfframpPayout{ID: payoutID, OrderID: orderID,
		Method: string(entity.WithdrawalMethodSandboxTransfer), AmountMinor: 100_000,
		ReferenceID: "payout_" + orderID, State: "completed", CompletedAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := store.db.Create(&payout).Error; err != nil {
		t.Fatalf("creating payout evidence: %v", err)
	}

	view, err := store.onramp.Get(ctx, integrationAPIPrincipal(), orderID)
	if err != nil {
		t.Fatalf("shared Get() error = %v", err)
	}
	if view.DepositTransactionHash != depositHash || view.Payout == nil || view.Payout.Reference != payout.ReferenceID ||
		view.Payout.AmountMinor != payout.AmountMinor || view.Payout.State != payout.State {
		t.Fatalf("shared order view = %+v, want deposit and payout details", view)
	}
}

func attachTestCheckout(t *testing.T, store *integrationStore, orderID string, method entity.PaymentMethod) {
	t.Helper()
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	checkout := usecase.Checkout{ProviderID: "ps-" + orderID, PaymentRequestID: "pr-" + orderID, Method: method, Status: "REQUIRES_ACTION",
		PresentationType: "QR_STRING", PresentationValue: "000201010211123", ExpiresAt: &expiresAt}
	if _, err := store.onramp.AttachCheckout(context.Background(), orderID, checkout); err != nil {
		t.Fatalf("AttachCheckout() error = %v", err)
	}
}

func TestExpectedPaymentResolvesPaymentRequestID(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000f1"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodBRIVA, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}
	attachTestCheckout(t, store, orderID, entity.PaymentMethodBRIVA)

	checks := []struct {
		name       string
		providerID string
	}{
		{name: "payment session", providerID: "ps-" + orderID},
		{name: "payment request", providerID: "pr-" + orderID},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			expected, err := store.onramp.ExpectedPayment(ctx, check.providerID)
			if err != nil {
				t.Fatalf("ExpectedPayment() error = %v", err)
			}
			if expected.OrderID != orderID || expected.ProviderID != check.providerID {
				t.Fatalf("ExpectedPayment() = %+v, want order %s and provider %s", expected, orderID, check.providerID)
			}
		})
	}
}

func TestRecordCallbackReceiptConcurrentReplay(t *testing.T) {
	store := newIntegrationStore(t)
	receipt := usecase.CallbackReceipt{EventID: "payment-concurrent-1", EventType: "payment.capture", CheckoutID: "pr-concurrent-1", PayloadHash: "payload-hash-1"}
	results := make(chan struct {
		processed bool
		err       error
	}, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for range 2 {
		go func() {
			defer waitGroup.Done()
			processed, err := store.onramp.RecordCallbackReceipt(context.Background(), receipt)
			results <- struct {
				processed bool
				err       error
			}{processed: processed, err: err}
		}()
	}
	waitGroup.Wait()
	close(results)

	processedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("RecordCallbackReceipt() error = %v", result.err)
		}
		if result.processed {
			processedCount++
		}
	}
	if processedCount > 1 {
		t.Fatalf("processed result count = %d, want at most 1", processedCount)
	}
	if count := countRows(t, store.db, &entity.GatewayEvent{}); count != 1 {
		t.Fatalf("gateway event count = %d, want 1", count)
	}
}

func loadOrder(t *testing.T, db *gorm.DB, orderID string) entity.OrderRecord {
	t.Helper()
	var order entity.OrderRecord
	if err := db.Where("id = ?", orderID).First(&order).Error; err != nil {
		t.Fatalf("loading order: %v", err)
	}
	return order
}

func TestFailCheckoutAppendsEventAndReleasesReservation(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000c2"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodQRIS, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}

	if err := store.onramp.FailCheckout(ctx, orderID, "provider rejected checkout"); err != nil {
		t.Fatalf("FailCheckout() error = %v", err)
	}
	order := loadOrder(t, store.db, orderID)
	if order.Status != string(entity.OrderStatusPaymentFailed) {
		t.Fatalf("status = %s, want payment_failed", order.Status)
	}
	var events []entity.OrderEvent
	if err := store.db.Where("order_id = ?", orderID).Find(&events).Error; err != nil {
		t.Fatalf("loading events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (created + failed)", len(events))
	}
	var reservation entity.TreasuryReservation
	if err := store.db.Where("order_id = ?", orderID).First(&reservation).Error; err != nil {
		t.Fatalf("loading reservation: %v", err)
	}
	if reservation.Status != "released" {
		t.Fatalf("reservation status = %s, want released", reservation.Status)
	}
}

func TestConfirmPaymentAndEnqueueIsIdempotent(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000d1"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodBRIVA, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}
	attachTestCheckout(t, store, orderID, entity.PaymentMethodBRIVA)

	confirmation := usecase.PaymentConfirmation{EventID: "event-1", EventType: "payment.capture", PayloadHash: "hash",
		Expected: usecase.ExpectedPayment{OrderID: orderID, ProviderID: "pr-" + orderID, Amount: 100_000, Currency: "IDR", Channel: "BRI_VIRTUAL_ACCOUNT"}}
	for i := 0; i < 2; i++ {
		if err := store.onramp.ConfirmPaymentAndEnqueue(ctx, confirmation); err != nil {
			t.Fatalf("ConfirmPaymentAndEnqueue() attempt %d error = %v", i+1, err)
		}
	}
	order := loadOrder(t, store.db, orderID)
	if order.Status != string(entity.OrderStatusStellarProcessing) {
		t.Fatalf("status = %s, want stellar_processing", order.Status)
	}
	if count := countRows(t, store.db, &entity.StellarTransaction{}); count != 1 {
		t.Fatalf("settlement intents = %d, want 1", count)
	}
	if count := countRows(t, store.db, &entity.OutboxMessage{}); count != 1 {
		t.Fatalf("outbox messages = %d, want 1", count)
	}
	if count := countRows(t, store.db, &entity.OrderEvent{}); count != 4 {
		t.Fatalf("order events = %d, want 4 (created, checkout, payment confirmed, transfer requested)", count)
	}
}

func TestConfirmPaymentAndEnqueueIsConcurrentIdempotent(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000f2"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodBRIVA, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}
	attachTestCheckout(t, store, orderID, entity.PaymentMethodBRIVA)

	confirmation := usecase.PaymentConfirmation{EventID: "event-concurrent-confirmation", EventType: "payment.capture", PayloadHash: "hash-concurrent",
		Expected: usecase.ExpectedPayment{OrderID: orderID, ProviderID: "pr-" + orderID, Amount: 100_000, Currency: "IDR", Channel: "BRI_VIRTUAL_ACCOUNT"}}
	errors := make(chan error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for range 2 {
		go func() {
			defer waitGroup.Done()
			errors <- store.onramp.ConfirmPaymentAndEnqueue(ctx, confirmation)
		}()
	}
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("ConfirmPaymentAndEnqueue() error = %v", err)
		}
	}

	if count := countRows(t, store.db, &entity.StellarTransaction{}); count != 1 {
		t.Fatalf("settlement intents = %d, want 1", count)
	}
	if count := countRows(t, store.db, &entity.OutboxMessage{}); count != 1 {
		t.Fatalf("outbox messages = %d, want 1", count)
	}
}

func TestReleaseExpiredReservationsExpiresUnpaidOrdersOnly(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-time.Minute)

	paidID := "00000000-0000-4000-8000-0000000000e1"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(paidID, entity.PaymentMethodQRIS, 100_000, 40_000_000, past), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate(paid) error = %v", err)
	}
	attachTestCheckout(t, store, paidID, entity.PaymentMethodQRIS)

	unknownID := "00000000-0000-4000-8000-0000000000e2"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(unknownID, entity.PaymentMethodQRIS, 100_000, 40_000_000, past), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate(unknown) error = %v", err)
	}
	if err := store.onramp.MarkCheckoutUnknown(ctx, unknownID, "provider outcome unknown"); err != nil {
		t.Fatalf("MarkCheckoutUnknown() error = %v", err)
	}

	futureID := "00000000-0000-4000-8000-0000000000e3"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(futureID, entity.PaymentMethodQRIS, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate(future) error = %v", err)
	}

	released, err := store.settlement.ReleaseExpiredReservations(ctx, time.Now().UTC(), 100)
	if err != nil || released != 1 {
		t.Fatalf("ReleaseExpiredReservations() = %d, %v; want 1, nil", released, err)
	}
	if order := loadOrder(t, store.db, paidID); order.Status != string(entity.OrderStatusExpired) {
		t.Fatalf("paid status = %s, want expired", order.Status)
	}
	if order := loadOrder(t, store.db, unknownID); order.Status != string(entity.OrderStatusCreated) {
		t.Fatalf("unknown status = %s, want created (held for operator)", order.Status)
	}
	if order := loadOrder(t, store.db, futureID); order.Status != string(entity.OrderStatusCreated) {
		t.Fatalf("future status = %s, want created (unexpired and untouched)", order.Status)
	}
	var events []entity.OrderEvent
	if err := store.db.Where("order_id = ? AND event_type = ?", paidID, "order.expired").Find(&events).Error; err != nil {
		t.Fatalf("loading expiry events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expiry events = %d, want 1", len(events))
	}
}

func TestSettlementConfirmConsumesReservationAndCompletesOrder(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-0000000000f1"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodQRIS, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}
	attachTestCheckout(t, store, orderID, entity.PaymentMethodQRIS)
	confirmation := usecase.PaymentConfirmation{EventID: "event-1", EventType: "payment.capture", PayloadHash: "hash",
		Expected: usecase.ExpectedPayment{OrderID: orderID, ProviderID: "pr-" + orderID, Amount: 100_000, Currency: "IDR", Channel: "QRIS"}}
	if err := store.onramp.ConfirmPaymentAndEnqueue(ctx, confirmation); err != nil {
		t.Fatalf("ConfirmPaymentAndEnqueue() error = %v", err)
	}

	intentID := "stellar-onramp-" + orderID
	if err := store.settlement.MarkSubmitted(ctx, intentID, "hash-1", time.Now().UTC()); err != nil {
		t.Fatalf("MarkSubmitted() error = %v", err)
	}
	if err := store.settlement.Confirm(ctx, intentID, "hash-1", time.Now().UTC()); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}

	order := loadOrder(t, store.db, orderID)
	if order.Status != string(entity.OrderStatusCompleted) {
		t.Fatalf("status = %s, want completed", order.Status)
	}
	var reservation entity.TreasuryReservation
	if err := store.db.Where("order_id = ?", orderID).First(&reservation).Error; err != nil {
		t.Fatalf("loading reservation: %v", err)
	}
	if reservation.Status != "consumed" {
		t.Fatalf("reservation status = %s, want consumed", reservation.Status)
	}
	var outbox entity.OutboxMessage
	if err := store.db.Where("aggregate_id = ?", orderID).First(&outbox).Error; err != nil {
		t.Fatalf("loading outbox: %v", err)
	}
	if outbox.ProcessedAt == nil {
		t.Fatal("outbox message was not finished")
	}
}

func TestFailPermanentAppendsEventAndHoldsReservation(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	orderID := "00000000-0000-4000-8000-000000000101"
	if err := store.onramp.ReserveAndCreate(ctx, testCreateRecord(orderID, entity.PaymentMethodQRIS, 100_000, 40_000_000, time.Now().UTC().Add(5*time.Minute)), 1_000_000_000); err != nil {
		t.Fatalf("ReserveAndCreate() error = %v", err)
	}
	attachTestCheckout(t, store, orderID, entity.PaymentMethodQRIS)
	confirmation := usecase.PaymentConfirmation{EventID: "event-1", EventType: "payment.capture", PayloadHash: "hash",
		Expected: usecase.ExpectedPayment{OrderID: orderID, ProviderID: "pr-" + orderID, Amount: 100_000, Currency: "IDR", Channel: "QRIS"}}
	if err := store.onramp.ConfirmPaymentAndEnqueue(ctx, confirmation); err != nil {
		t.Fatalf("ConfirmPaymentAndEnqueue() error = %v", err)
	}

	intentID := "stellar-onramp-" + orderID
	if err := store.settlement.MarkSubmitted(ctx, intentID, "hash-1", time.Now().UTC()); err != nil {
		t.Fatalf("MarkSubmitted() error = %v", err)
	}
	if err := store.settlement.FailPermanent(ctx, intentID, "Stellar transaction rejected"); err != nil {
		t.Fatalf("FailPermanent() error = %v", err)
	}

	order := loadOrder(t, store.db, orderID)
	if order.Status != string(entity.OrderStatusStellarFailed) {
		t.Fatalf("status = %s, want stellar_failed", order.Status)
	}
	var events []entity.OrderEvent
	if err := store.db.Where("order_id = ? AND event_type = ?", orderID, "stellar.failed").Find(&events).Error; err != nil {
		t.Fatalf("loading failure events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("failure events = %d, want 1", len(events))
	}
	var reservation entity.TreasuryReservation
	if err := store.db.Where("order_id = ?", orderID).First(&reservation).Error; err != nil {
		t.Fatalf("loading reservation: %v", err)
	}
	if reservation.Status != "reserved" {
		t.Fatalf("reservation status = %s, want reserved (held for manual recovery)", reservation.Status)
	}
}
