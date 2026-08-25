package entity

import "time"

// OfframpPayout records the sandbox-simulated IDR payout for one off-ramp
// order (ADR-003). The reference is deterministic and the wording everywhere
// must disclose that no real IDR moves in the sandbox release.
type OfframpPayout struct {
	ID          string `gorm:"type:uuid;primaryKey"`
	OrderID     string `gorm:"type:uuid;not null;uniqueIndex:idx_offramp_payouts_order"`
	Method      string `gorm:"type:text;not null"`
	AmountMinor int64  `gorm:"not null"`
	ReferenceID string `gorm:"type:text;not null;uniqueIndex:idx_offramp_payouts_reference"`
	State       string `gorm:"type:text;not null"`
	CompletedAt *time.Time
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (OfframpPayout) TableName() string { return "offramp_payouts" }
