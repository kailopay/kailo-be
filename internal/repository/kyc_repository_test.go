package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestKYCInquiryRowMappingPreservesOnlySafeFields(t *testing.T) {
	createdAt := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	expiresAt := createdAt.Add(24 * time.Hour)
	approvedAt := updatedAt.Add(time.Minute)
	providerInquiryID := "inq_1"
	providerEventID := "evt_1"
	row := entity.KYCInquiry{
		ID: "internal-1", UserID: "user-1", Provider: "persona", ProviderInquiryID: &providerInquiryID,
		ProviderRequestKey: "kyc_internal-1", ProviderStatus: "approved", Status: entity.KYCInquiryApproved,
		ProviderEventAt: &updatedAt, LastProviderEventID: &providerEventID, ApprovedAt: &approvedAt,
		ExpiresAt: &expiresAt, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}

	got := kycInquiryRecord(row)
	if got.ID != row.ID || got.UserID != row.UserID || got.ProviderInquiryID != providerInquiryID ||
		got.Status != usecase.KYCStatusApproved || got.ApprovedAt == nil || !got.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("kycInquiryRecord() = %+v", got)
	}
}

func TestKYCProviderEventConflictUsesStableError(t *testing.T) {
	if !errors.Is(fmtKYCProviderEventConflict(), usecase.ErrKYCProviderEventConflict) {
		t.Fatal("provider event conflict is not classified as ErrKYCProviderEventConflict")
	}
}

func TestIsProviderEventNewerUsesTimestampThenProviderEventID(t *testing.T) {
	currentAt := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	currentID := "evt-2"
	newerAt := currentAt.Add(time.Second)
	olderAt := currentAt.Add(-time.Second)

	for _, test := range []struct {
		name    string
		eventAt time.Time
		eventID string
		want    bool
	}{
		{name: "later timestamp", eventAt: newerAt, eventID: "evt-1", want: true},
		{name: "older timestamp", eventAt: olderAt, eventID: "evt-9", want: false},
		{name: "equal timestamp with greater id", eventAt: currentAt, eventID: "evt-3", want: true},
		{name: "equal timestamp with lower id", eventAt: currentAt, eventID: "evt-1", want: false},
		{name: "equal timestamp without stored id", eventAt: currentAt, eventID: "evt-1", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			storedID := &currentID
			if test.name == "equal timestamp without stored id" {
				storedID = nil
			}
			if got := isProviderEventNewer(&currentAt, storedID, test.eventAt, test.eventID); got != test.want {
				t.Fatalf("isProviderEventNewer() = %v, want %v", got, test.want)
			}
		})
	}

	if !isProviderEventNewer(nil, nil, currentAt, "evt-1") {
		t.Fatal("an inquiry without a provider event must accept its first event")
	}
}

func fmtKYCProviderEventConflict() error {
	return usecase.ErrKYCProviderEventConflict
}
