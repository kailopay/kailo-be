package entity

import "time"

type WebhookEvent struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	OrderID            string    `gorm:"type:uuid;not null;index"`
	EventType          string    `gorm:"type:text;not null"`
	APIVersion         string    `gorm:"type:text;not null"`
	CanonicalPayload   []byte    `gorm:"type:jsonb;not null"`
	SourceOrderEventID *string   `gorm:"type:uuid"`
	CreatedAt          time.Time `gorm:"not null"`
}

func (WebhookEvent) TableName() string { return "webhook_events" }
