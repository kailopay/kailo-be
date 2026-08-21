package entity

import "time"

type APIKey struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	ClientID   string    `gorm:"type:uuid;not null;index"`
	PublicID   string    `gorm:"type:text;not null;uniqueIndex"`
	Prefix     string    `gorm:"type:text;not null"`
	SecretHash []byte    `gorm:"type:bytea;not null"`
	CreatedAt  time.Time `gorm:"not null"`
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

func (APIKey) TableName() string { return "api_keys" }
