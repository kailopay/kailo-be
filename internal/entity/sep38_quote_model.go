package entity

import "time"

// SEP38Quote is a firm quote owned by one authenticated Stellar account.
// Amounts remain strings so the exact decimal representation is preserved.
type SEP38Quote struct {
	ID             string    `gorm:"type:uuid;primaryKey"`
	QuoteID        string    `gorm:"type:text;not null;unique"`
	WalletAccount  string    `gorm:"type:text;not null;index"`
	SellAsset      string    `gorm:"type:text;not null"`
	BuyAsset       string    `gorm:"type:text;not null"`
	SellAmount     string    `gorm:"type:text;not null"`
	BuyAmount      string    `gorm:"type:text;not null"`
	Price          string    `gorm:"type:text;not null"`
	SpreadBPS      int       `gorm:"not null"`
	DeliveryMethod string    `gorm:"type:text;not null"`
	ExpiresAt      time.Time `gorm:"not null;index"`
	ConsumedAt     *time.Time
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (SEP38Quote) TableName() string { return "sep38_quotes" }
