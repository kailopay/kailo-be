package entity

import "time"

type WebhookEndpoint struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	ClientID        string    `gorm:"type:uuid;not null;index"`
	URL             string    `gorm:"type:text;not null"`
	Status          string    `gorm:"type:text;not null"`
	SecretReference string    `gorm:"type:text;not null"`
	EventTypes      []byte    `gorm:"type:jsonb;not null"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	DisabledAt      *time.Time
}

func (WebhookEndpoint) TableName() string { return "webhook_endpoints" }
