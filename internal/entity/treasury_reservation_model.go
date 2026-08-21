package entity

import "time"

type TreasuryReservation struct {
	ID            string    `gorm:"type:uuid;primaryKey"`
	TreasuryID    string    `gorm:"type:uuid;not null;index"`
	OrderID       string    `gorm:"type:uuid;not null;uniqueIndex"`
	AmountStroops int64     `gorm:"not null"`
	Status        string    `gorm:"type:text;not null;index"`
	ExpiresAt     time.Time `gorm:"not null;index"`
	ConsumedAt    *time.Time
	ReleasedAt    *time.Time
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (TreasuryReservation) TableName() string { return "treasury_reservations" }
