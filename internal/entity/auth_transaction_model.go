package entity

import "time"

type AuthTransaction struct {
	ID                     string     `gorm:"type:uuid;primaryKey"`
	StateHash              []byte     `gorm:"type:bytea;not null;uniqueIndex"`
	NonceHash              []byte     `gorm:"type:bytea;not null"`
	CodeVerifierCiphertext []byte     `gorm:"type:bytea;not null"`
	ExpiresAt              time.Time  `gorm:"not null;index"`
	ConsumedAt             *time.Time `gorm:"index"`
	CreatedAt              time.Time  `gorm:"not null"`
}

func (AuthTransaction) TableName() string { return "auth_transactions" }
