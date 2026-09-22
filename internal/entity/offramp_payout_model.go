package entity

import "time"

// OfframpPayout records sandbox payout evidence for one off-ramp order. The
// simulator creates at most one row per order and never moves real fiat.
type OfframpPayout struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	OrderID     string `gorm:"type:uuid;not null;unique"`
	Method      string `gorm:"type:text;not null"`
	AmountMinor int64  `gorm:"not null"`
	ReferenceID string `gorm:"type:text;not null;unique"`
	State       string `gorm:"type:text;not null"`
	CompletedAt *time.Time
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (OfframpPayout) TableName() string { return "offramp_payouts" }
