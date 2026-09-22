package entity

import "time"

type WebhookEvent struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	OrderID            *string   `gorm:"type:uuid;index"`
	ClientID           *string   `gorm:"type:uuid;index"`
	EventType          string    `gorm:"type:text;not null"`
	APIVersion         string    `gorm:"type:text;not null"`
	CanonicalPayload   []byte    `gorm:"type:jsonb;not null"`
	SourceOrderEventID *string   `gorm:"type:uuid"`
	IsTest             bool      `gorm:"not null;default:false;index"`
	CreatedAt          time.Time `gorm:"not null"`
}

func (WebhookEvent) TableName() string { return "webhook_events" }
