package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestKYCReserveAllowsOnlyOneActiveInquiryPerUser(t *testing.T) {
	store := newIntegrationStore(t)
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	type result struct {
		record  usecase.KYCInquiryRecord
		created bool
		err     error
	}
	results := make(chan result, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for _, id := range []string{
		"00000000-0000-4000-8000-000000000401",
		"00000000-0000-4000-8000-000000000402",
	} {
		go func(internalID string) {
			defer waitGroup.Done()
			record, created, err := store.kyc.Reserve(context.Background(), integrationUserID(), internalID, "kyc_"+internalID, now)
			results <- result{record: record, created: created, err: err}
		}(id)
	}
	waitGroup.Wait()
	close(results)

	var records []result
	for item := range results {
		if item.err != nil {
			t.Fatalf("Reserve() error = %v", item.err)
		}
		records = append(records, item)
	}
	if len(records) != 2 || records[0].record.ID != records[1].record.ID {
		t.Fatalf("reservation results = %+v", records)
	}
	if (records[0].created && records[1].created) || (!records[0].created && !records[1].created) {
		t.Fatalf("created flags = %v/%v, want exactly one creator", records[0].created, records[1].created)
	}
	if count := countRows(t, store.db, &entity.KYCInquiry{}); count != 1 {
		t.Fatalf("kyc inquiry count = %d, want 1", count)
	}
}

func TestKYCAttachAndFindByProviderInquiryID(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	internalID := "00000000-0000-4000-8000-000000000403"
	if _, _, err := store.kyc.Reserve(ctx, integrationUserID(), internalID, "kyc_"+internalID, now); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	expiresAt := now.Add(24 * time.Hour)
	if err := store.kyc.AttachProviderInquiry(ctx, internalID, "inq_1", "created", &expiresAt, now); err != nil {
		t.Fatalf("AttachProviderInquiry() error = %v", err)
	}
	record, found, err := store.kyc.FindByProviderInquiryID(ctx, "persona", "inq_1")
	if err != nil || !found || record.ID != internalID || record.ProviderInquiryID != "inq_1" {
		t.Fatalf("FindByProviderInquiryID() = %+v/%v/%v", record, found, err)
	}
}

func TestKYCProviderEventsAreIdempotentAndApplyInCreationOrder(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	internalID := "00000000-0000-4000-8000-000000000404"
	if _, _, err := store.kyc.Reserve(ctx, integrationUserID(), internalID, "kyc_"+internalID, base); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}
	if err := store.kyc.AttachProviderInquiry(ctx, internalID, "inq_1", "pending", nil, base); err != nil {
		t.Fatalf("AttachProviderInquiry() error = %v", err)
	}
	approvedAt := base.Add(2 * time.Minute)
	approved := usecase.KYCProviderEventRecord{
		ID: "00000000-0000-4000-8000-000000000405", Provider: "persona", ProviderEventID: "evt-approved",
		InquiryID: internalID, EventType: "inquiry.approved", ProviderEventAt: approvedAt, PayloadHash: "hash-approved", ReceivedAt: approvedAt,
	}
	if err := store.kyc.RecordProviderEvent(ctx, approved, usecase.KYCStatusApproved, "approved", approvedAt); err != nil {
		t.Fatalf("RecordProviderEvent(approved) error = %v", err)
	}
	if err := store.kyc.RecordProviderEvent(ctx, approved, usecase.KYCStatusApproved, "approved", approvedAt.Add(time.Minute)); err != nil {
		t.Fatalf("duplicate RecordProviderEvent() error = %v", err)
	}
	conflict := approved
	conflict.PayloadHash = "different-hash"
	if err := store.kyc.RecordProviderEvent(ctx, conflict, usecase.KYCStatusApproved, "approved", approvedAt.Add(2*time.Minute)); !errors.Is(err, usecase.ErrKYCProviderEventConflict) {
		t.Fatalf("conflicting event error = %v", err)
	}
	older := usecase.KYCProviderEventRecord{
		ID: "00000000-0000-4000-8000-000000000406", Provider: "persona", ProviderEventID: "evt-pending",
		InquiryID: internalID, EventType: "inquiry.started", ProviderEventAt: base.Add(time.Minute), PayloadHash: "hash-pending", ReceivedAt: approvedAt.Add(3 * time.Minute),
	}
	if err := store.kyc.RecordProviderEvent(ctx, older, usecase.KYCStatusPending, "pending", older.ProviderEventAt); err != nil {
		t.Fatalf("older RecordProviderEvent() error = %v", err)
	}
	record, found, err := store.kyc.FindCurrent(ctx, integrationUserID())
	if err != nil || !found || record.Status != usecase.KYCStatusApproved {
		t.Fatalf("FindCurrent() = %+v/%v/%v, want approved", record, found, err)
	}
	if count := countRows(t, store.db, &entity.KYCProviderEvent{}); count != 2 {
		t.Fatalf("kyc provider event count = %d, want 2", count)
	}
}

func integrationUserID() string {
	return "00000000-0000-4000-8000-0000000000aa"
}
