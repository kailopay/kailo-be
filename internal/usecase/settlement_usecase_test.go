package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type settlementStoreFake struct {
	job         Job
	intent      Intent
	confirmed   int
	resetCalls  int
	retryErrors []string
	released    int
}

func (s *settlementStoreFake) Lease(context.Context, string, time.Time, time.Duration) (Job, error) {
	return s.job, nil
}
func (s *settlementStoreFake) LoadIntent(context.Context, string) (Intent, error) {
	return s.intent, nil
}
func (s *settlementStoreFake) MarkSubmitted(_ context.Context, _ string, hash string, _ time.Time) error {
	s.intent.TransactionHash = hash
	return nil
}
func (s *settlementStoreFake) Confirm(context.Context, string, string, time.Time) error {
	s.confirmed++
	return nil
}
func (s *settlementStoreFake) MarkUnknown(context.Context, string, string) error   { return nil }
func (s *settlementStoreFake) FailPermanent(context.Context, string, string) error { return nil }
func (s *settlementStoreFake) RetryLater(_ context.Context, _ string, _ time.Time, safeError string) error {
	s.retryErrors = append(s.retryErrors, safeError)
	return nil
}
func (s *settlementStoreFake) ResetSubmitted(_ context.Context, _ string, safeError string) error {
	s.resetCalls++
	s.intent.TransactionHash = ""
	s.retryErrors = append(s.retryErrors, safeError)
	return nil
}
func (s *settlementStoreFake) ReleaseExpiredReservations(context.Context, time.Time, int) (int, error) {
	s.released++
	return s.released, nil
}

type settlementNetworkFake struct{ buildCalls, submitCalls, findCalls int }

func (n *settlementNetworkFake) Build(context.Context, Transfer) (BuiltTransaction, error) {
	n.buildCalls++
	return BuiltTransaction{Hash: "hash-1", Envelope: "envelope"}, nil
}
func (n *settlementNetworkFake) Submit(context.Context, BuiltTransaction) (Submission, error) {
	n.submitCalls++
	return Submission{Result: SubmissionUnknown}, nil
}
func (n *settlementNetworkFake) FindByHash(context.Context, string) (Submission, error) {
	n.findCalls++
	return Submission{Result: SubmissionConfirmed, LedgerAt: time.Now()}, nil
}
func (n *settlementNetworkFake) SpendableBalance(context.Context, string) (entity.Stroops, error) {
	return 0, nil
}

func TestUnknownSubmissionReconcilesHashBeforeRetry(t *testing.T) {
	store := &settlementStoreFake{job: Job{OutboxID: "outbox-1", IntentID: "intent-1"}, intent: Intent{IntentID: "intent-1", Transfer: Transfer{Amount: 100}}}
	network := &settlementNetworkFake{}
	service, err := NewSettlementUsecase(store, network, SettlementConfig{LeaseDuration: time.Minute, RetryDelay: time.Second, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunOnce(context.Background(), "worker-1")
	if err != nil || !processed {
		t.Fatalf("RunOnce() = %v, %v", processed, err)
	}
	if network.buildCalls != 1 || network.submitCalls != 1 || network.findCalls != 1 || store.confirmed != 1 {
		t.Fatalf("build=%d submit=%d find=%d confirmed=%d", network.buildCalls, network.submitCalls, network.findCalls, store.confirmed)
	}
}

func TestRetryableSubmissionResetsHashAndSchedulesRebuild(t *testing.T) {
	store := &settlementStoreFake{job: Job{OutboxID: "outbox-1", IntentID: "intent-1"}, intent: Intent{IntentID: "intent-1", Transfer: Transfer{Amount: 100}}}
	network := &retryableNetworkFake{}
	service, err := NewSettlementUsecase(store, network, SettlementConfig{LeaseDuration: time.Minute, RetryDelay: time.Second, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.RunOnce(context.Background(), "worker-1")
	if err != nil || !processed {
		t.Fatalf("RunOnce() = %v, %v", processed, err)
	}
	if store.resetCalls != 1 {
		t.Fatalf("ResetSubmitted calls = %d, want 1", store.resetCalls)
	}
	if store.intent.TransactionHash != "" {
		t.Fatalf("intent hash = %q, want cleared", store.intent.TransactionHash)
	}
	if network.findCalls != 0 {
		t.Fatalf("FindByHash calls = %d, want 0 for a rejected transaction", network.findCalls)
	}
}

func TestReleaseExpiredDelegatesToRepository(t *testing.T) {
	store := &settlementStoreFake{}
	service, err := NewSettlementUsecase(store, &settlementNetworkFake{}, SettlementConfig{LeaseDuration: time.Minute, RetryDelay: time.Second, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	released, err := service.ReleaseExpired(context.Background())
	if err != nil || released != 1 {
		t.Fatalf("ReleaseExpired() = %d, %v", released, err)
	}
}

type retryableNetworkFake struct{ settlementNetworkFake }

func (n *retryableNetworkFake) Submit(context.Context, BuiltTransaction) (Submission, error) {
	n.submitCalls++
	return Submission{Result: SubmissionRetryable, SafeError: "Stellar sequence conflict"}, nil
}
