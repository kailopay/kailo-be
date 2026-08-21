package entity

import "time"

type RetailSession struct {
	ID         string     `gorm:"type:uuid;primaryKey"`
	UserID     string     `gorm:"type:uuid;not null;index"`
	TokenHash  []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	ExpiresAt  time.Time  `gorm:"not null;index"`
	LastUsedAt *time.Time `gorm:"index"`
	RevokedAt  *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
	UpdatedAt  time.Time  `gorm:"not null"`
}

func (RetailSession) TableName() string { return "retail_sessions" }
