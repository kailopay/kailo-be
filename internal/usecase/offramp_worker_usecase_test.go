package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type depositExpiryFake struct {
	calls int
	now   time.Time
	limit int
	count int
}

func (f *depositExpiryFake) ExpirePendingDeposits(_ context.Context, now time.Time, limit int) (int, error) {
	f.calls++
	f.now = now
	f.limit = limit
	return f.count, nil
}

func TestDepositScannerExpireDepositsDelegatesToRepository(t *testing.T) {
	expiry := &depositExpiryFake{count: 3}
	scanner := DepositScanner{Expiry: expiry}
	now := time.Date(2026, 9, 13, 5, 0, 0, 0, time.FixedZone("test", 7*60*60))

	got, err := scanner.ExpireDeposits(context.Background(), now)
	if err != nil {
		t.Fatalf("ExpireDeposits() error = %v", err)
	}
	if got != 3 || expiry.calls != 1 {
		t.Fatalf("ExpireDeposits() = %d with %d calls, want 3 with 1 call", got, expiry.calls)
	}
	if !expiry.now.Equal(now.UTC()) {
		t.Fatalf("expiry timestamp = %v, want %v", expiry.now, now.UTC())
	}
	if expiry.limit != 100 {
		t.Fatalf("expiry limit = %d, want 100", expiry.limit)
	}
}

func TestDepositScannerExpireDepositsWithoutRepositoryIsNoOp(t *testing.T) {
	got, err := (DepositScanner{}).ExpireDeposits(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ExpireDeposits() error = %v", err)
	}
	if got != 0 {
		t.Fatalf("ExpireDeposits() = %d, want 0", got)
	}
}

type depositScanSourceFake struct {
	candidates []OrderView
}

func (f *depositScanSourceFake) FindDepositCandidates(context.Context, string, int) ([]OrderView, error) {
	return f.candidates, nil
}

type depositPaymentsFake struct {
	payments []ObservedPayment
}

func (f *depositPaymentsFake) RecentPayments(context.Context, string, int) ([]ObservedPayment, error) {
	return f.payments, nil
}

type depositSinkFake struct {
	received []string
	invalid  []struct {
		orderID string
		hash    string
		reason  string
	}
}

func (f *depositSinkFake) RecordAssetReceived(_ context.Context, orderID string, _ ObservedPayment) error {
	f.received = append(f.received, orderID)
	return nil
}

func (f *depositSinkFake) RecordAssetInvalid(_ context.Context, orderID, hash, reason string) error {
	f.invalid = append(f.invalid, struct {
		orderID string
		hash    string
		reason  string
	}{orderID: orderID, hash: hash, reason: reason})
	return nil
}

func TestDepositScannerRequiresTransactionHashAndKeepsMismatchCorrelation(t *testing.T) {
	account := "GDEPOSIT"
	candidates := &depositScanSourceFake{candidates: []OrderView{
		{ID: "order-valid", AssetAmount: entity.Stroops(40_000_000)},
		{ID: "order-empty-hash", AssetAmount: entity.Stroops(40_000_000)},
		{ID: "order-mismatch", AssetAmount: entity.Stroops(60_000_000)},
	}}
	payments := &depositPaymentsFake{payments: []ObservedPayment{
		{TransactionHash: "hash-valid", To: account, Amount: entity.Stroops(40_000_000), Memo: OfframpDepositMemo("order-valid"), LedgerAt: time.Now()},
		{To: account, Amount: entity.Stroops(40_000_000), Memo: OfframpDepositMemo("order-empty-hash"), LedgerAt: time.Now()},
		{TransactionHash: "hash-mismatch", To: account, Amount: entity.Stroops(40_000_000), Memo: OfframpDepositMemo("order-mismatch"), LedgerAt: time.Now()},
	}}
	sink := &depositSinkFake{}
	scanner := DepositScanner{Candidates: candidates, Payments: payments, Repository: sink}

	matched, err := scanner.ScanDeposits(context.Background(), account, time.Now())
	if err != nil {
		t.Fatalf("ScanDeposits() error = %v", err)
	}
	if matched != 1 || len(sink.received) != 1 || sink.received[0] != "order-valid" {
		t.Fatalf("matched=%d received=%v, want one valid deposit", matched, sink.received)
	}
	if len(sink.invalid) != 1 || sink.invalid[0].orderID != "order-mismatch" || sink.invalid[0].hash != "hash-mismatch" {
		t.Fatalf("invalid=%+v, want mismatch order and hash", sink.invalid)
	}
}

func TestDepositScannerUsesPersistedMemo(t *testing.T) {
	const (
		account = "GDEPOSIT"
		orderID = "15854fb0-d7ea-489d-a773-b01d46f948a9"
		memo    = "offlegacy-memo"
	)
	candidates := &depositScanSourceFake{candidates: []OrderView{
		{ID: orderID, StellarMemo: memo, AssetAmount: entity.Stroops(40_000_000)},
	}}
	payments := &depositPaymentsFake{payments: []ObservedPayment{
		{TransactionHash: "hash-persisted-memo", To: account, Amount: entity.Stroops(40_000_000), Memo: memo, LedgerAt: time.Now()},
	}}
	sink := &depositSinkFake{}
	scanner := DepositScanner{Candidates: candidates, Payments: payments, Repository: sink}

	matched, err := scanner.ScanDeposits(context.Background(), account, time.Now())
	if err != nil {
		t.Fatalf("ScanDeposits() error = %v", err)
	}
	if matched != 1 || len(sink.received) != 1 || sink.received[0] != orderID {
		t.Fatalf("matched=%d received=%v, want persisted memo to match", matched, sink.received)
	}
}
