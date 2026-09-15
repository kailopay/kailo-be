package entity

import "time"

type KYCInquiryStatus string

const (
	KYCInquiryCreating      KYCInquiryStatus = "creating"
	KYCInquiryCreated       KYCInquiryStatus = "created"
	KYCInquiryPending       KYCInquiryStatus = "pending"
	KYCInquiryCompleted     KYCInquiryStatus = "completed"
	KYCInquiryPendingReview KYCInquiryStatus = "pending_review"
	KYCInquiryApproved      KYCInquiryStatus = "approved"
	KYCInquiryDeclined      KYCInquiryStatus = "declined"
	KYCInquiryFailed        KYCInquiryStatus = "failed"
	KYCInquiryExpired       KYCInquiryStatus = "expired"
)

type KYCInquiry struct {
	ID                  string           `gorm:"type:uuid;primaryKey"`
	UserID              string           `gorm:"type:uuid;not null;index"`
	Provider            string           `gorm:"type:text;not null"`
	ProviderInquiryID   *string          `gorm:"type:text;uniqueIndex:idx_kyc_inquiries_provider_inquiry"`
	ProviderRequestKey  string           `gorm:"type:text;not null;unique"`
	ProviderStatus      string           `gorm:"type:text"`
	Status              KYCInquiryStatus `gorm:"type:text;not null;index"`
	ProviderEventAt     *time.Time       `gorm:"index"`
	LastProviderEventID *string          `gorm:"type:text"`
	StartedAt           *time.Time
	CompletedAt         *time.Time
	ApprovedAt          *time.Time
	ExpiresAt           *time.Time
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

func (KYCInquiry) TableName() string { return "kyc_inquiries" }
