package entity

import "time"

type StellarTransaction struct {
	ID              string  `gorm:"type:uuid;primaryKey"`
	OrderID         string  `gorm:"type:uuid;not null;uniqueIndex:idx_stellar_order_purpose,priority:1"`
	IntentID        string  `gorm:"type:text;not null;uniqueIndex"`
	Purpose         string  `gorm:"type:text;not null;uniqueIndex:idx_stellar_order_purpose,priority:2"`
	Network         string  `gorm:"type:text;not null"`
	AssetCode       string  `gorm:"type:text;not null"`
	Amount          string  `gorm:"type:numeric(30,18);not null"`
	Source          *string `gorm:"type:text"`
	Destination     *string `gorm:"type:text"`
	Memo            *string `gorm:"type:text"`
	TransactionHash *string `gorm:"type:text;uniqueIndex"`
	Status          string  `gorm:"type:text;not null"`
	AttemptCount    int     `gorm:"not null"`
	LedgerAt        *time.Time
	LastError       *string   `gorm:"type:text"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

func (StellarTransaction) TableName() string { return "stellar_transactions" }
