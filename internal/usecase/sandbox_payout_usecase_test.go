package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSandboxPayoutWorkerCompletesOnlyInSimulatedMode(t *testing.T) {
	store := &sandboxPayoutRepositoryFake{}
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	worker, err := NewSandboxPayoutWorker(store, SandboxPayoutSimulated, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewSandboxPayoutWorker() error = %v", err)
	}
	if err := worker.RunOnce(context.Background(), Job{OutboxID: "outbox-1", IntentID: "order-1"}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if store.calls != 1 || store.reference != "sandbox-payout-order-1" {
		t.Fatalf("payout call = %+v, want one deterministic completion", store)
	}

	disabled, err := NewSandboxPayoutWorker(store, SandboxPayoutDisabled, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewSandboxPayoutWorker(disabled) error = %v", err)
	}
	if err := disabled.RunOnce(context.Background(), Job{OutboxID: "outbox-2", IntentID: "order-2"}); !errors.Is(err, ErrSandboxPayoutDisabled) {
		t.Fatalf("disabled RunOnce() error = %v, want disabled", err)
	}
	if store.calls != 1 {
		t.Fatal("disabled payout worker called the repository")
	}
}

func TestSandboxPayoutWorkerRejectsIncompleteJob(t *testing.T) {
	worker, err := NewSandboxPayoutWorker(&sandboxPayoutRepositoryFake{}, SandboxPayoutSimulated, time.Now)
	if err != nil {
		t.Fatalf("NewSandboxPayoutWorker() error = %v", err)
	}
	if err := worker.RunOnce(context.Background(), Job{OutboxID: "", IntentID: "order-1"}); !errors.Is(err, ErrSandboxPayoutInvalid) {
		t.Fatalf("RunOnce() error = %v, want invalid job", err)
	}
}

type sandboxPayoutRepositoryFake struct {
	calls     int
	orderID   string
	outboxID  string
	reference string
	when      time.Time
}

func (f *sandboxPayoutRepositoryFake) CompleteSandboxPayout(_ context.Context, orderID, outboxID, reference string, now time.Time) error {
	f.calls++
	f.orderID, f.outboxID, f.reference, f.when = orderID, outboxID, reference, now
	return nil
}
