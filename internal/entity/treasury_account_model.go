package entity

import "time"

type TreasuryAccount struct {
	ID                     string    `gorm:"type:uuid;primaryKey"`
	Network                string    `gorm:"type:text;not null;uniqueIndex:idx_treasury_network_account,priority:1"`
	PublicAccount          string    `gorm:"type:text;not null;uniqueIndex:idx_treasury_network_account,priority:2"`
	ObservedBalanceStroops int64     `gorm:"not null"`
	ReservedStroops        int64     `gorm:"not null"`
	OperatingBufferStroops int64     `gorm:"not null"`
	LastReconciledAt       time.Time `gorm:"not null"`
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
}

func (TreasuryAccount) TableName() string { return "treasury_accounts" }
