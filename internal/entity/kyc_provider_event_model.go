package entity

import "time"

type KYCProviderEvent struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	Provider        string    `gorm:"type:text;not null;index:idx_kyc_provider_event_identity,unique"`
	ProviderEventID string    `gorm:"type:text;not null;index:idx_kyc_provider_event_identity,unique"`
	InquiryID       string    `gorm:"type:uuid;not null;index"`
	EventType       string    `gorm:"type:text;not null"`
	ProviderEventAt time.Time `gorm:"not null"`
	PayloadHash     string    `gorm:"type:text;not null"`
	ReceivedAt      time.Time `gorm:"not null"`
}

func (KYCProviderEvent) TableName() string { return "kyc_provider_events" }
