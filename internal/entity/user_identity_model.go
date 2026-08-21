package entity

import "time"

type UserIdentity struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	UserID      string    `gorm:"type:uuid;not null;index"`
	Provider    string    `gorm:"type:text;not null;uniqueIndex:idx_user_identity_provider_subject,priority:1"`
	Subject     string    `gorm:"type:text;not null;uniqueIndex:idx_user_identity_provider_subject,priority:2"`
	CreatedAt   time.Time `gorm:"not null"`
	LastLoginAt *time.Time
}

func (UserIdentity) TableName() string { return "user_identities" }
