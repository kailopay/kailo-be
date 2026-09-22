package entity

import "time"

// SEP10Challenge stores the minimum durable state needed to validate one
// wallet-signed challenge without retaining the raw challenge transaction.
type SEP10Challenge struct {
	ID            string    `gorm:"type:uuid;primaryKey"`
	ChallengeHash []byte    `gorm:"type:bytea;not null;unique"`
	Account       string    `gorm:"type:text;not null"`
	HomeDomain    string    `gorm:"type:text;not null"`
	Network       string    `gorm:"type:text;not null"`
	ExpiresAt     time.Time `gorm:"not null;index"`
	ConsumedAt    *time.Time
	CreatedAt     time.Time `gorm:"not null"`
}

func (SEP10Challenge) TableName() string { return "sep10_challenges" }
