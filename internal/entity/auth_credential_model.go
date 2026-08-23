package entity

import "time"

type AuthCredential struct {
	ID                string `gorm:"type:uuid;primaryKey"`
	UserID            string `gorm:"type:uuid;not null;uniqueIndex:idx_auth_credentials_user"`
	PasswordHash      string `gorm:"type:text;not null"`
	FailedLoginCount  int    `gorm:"not null"`
	LockedUntil       *time.Time
	PasswordChangedAt time.Time `gorm:"not null"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (AuthCredential) TableName() string { return "auth_credentials" }
