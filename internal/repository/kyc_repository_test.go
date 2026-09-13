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

func fmtKYCProviderEventConflict() error {
	return usecase.ErrKYCProviderEventConflict
}
