package repository

import (
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestNormalizeOutboxJobUsesPersistedRowID(t *testing.T) {
	job := normalizeOutboxJob("outbox-1", usecase.Job{IntentID: "intent-1"})
	if job.OutboxID != "outbox-1" || job.IntentID != "intent-1" {
		t.Fatalf("job = %#v, want persisted outbox id and decoded intent", job)
	}
}

func TestNormalizeOutboxJobPreservesDecoderID(t *testing.T) {
	job := normalizeOutboxJob("outbox-1", usecase.Job{OutboxID: "decoded-id", IntentID: "intent-1"})
	if job.OutboxID != "decoded-id" {
		t.Fatalf("job.OutboxID = %q, want decoder value", job.OutboxID)
	}
}
