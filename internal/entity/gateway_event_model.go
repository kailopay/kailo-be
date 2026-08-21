package entity

import "time"

type GatewayEvent struct {
	ID                string    `gorm:"type:uuid;primaryKey"`
	Provider          string    `gorm:"type:text;not null;uniqueIndex:idx_gateway_event,priority:1"`
	ProviderEventID   string    `gorm:"type:text;not null;uniqueIndex:idx_gateway_event,priority:2"`
	EventType         string    `gorm:"type:text;not null"`
	CheckoutReference *string   `gorm:"type:text"`
	OrderReference    *string   `gorm:"type:text"`
	PayloadHash       string    `gorm:"type:text;not null"`
	SignatureVerified bool      `gorm:"not null"`
	MatchingResult    string    `gorm:"type:text"`
	ReceivedAt        time.Time `gorm:"not null"`
	ProcessedAt       *time.Time
	ProcessingStatus  string  `gorm:"type:text;not null"`
	ProcessingError   *string `gorm:"type:text"`
}

func (GatewayEvent) TableName() string { return "gateway_events" }
