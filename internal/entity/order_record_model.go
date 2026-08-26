package entity

import "time"

type OrderRecord struct {
	ID                    string    `gorm:"type:uuid;primaryKey"`
	ClientID              *string   `gorm:"type:uuid;index"`
	CreatedByUserID       *string   `gorm:"type:uuid;index"`
	RetailSessionID       *string   `gorm:"type:uuid;index"`
	Direction             string    `gorm:"type:text;not null"`
	Status                string    `gorm:"type:text;not null;index"`
	Version               int       `gorm:"not null"`
	Currency              string    `gorm:"type:text;not null"`
	FiatAmountMinor       int64     `gorm:"not null"`
	AssetCode             string    `gorm:"type:text;not null"`
	AssetIssuer           string    `gorm:"type:text;not null"`
	Network               string    `gorm:"type:text;not null"`
	AssetAmount           string    `gorm:"type:numeric(30,18);not null"`
	AssetAmountStroops    int64     `gorm:"not null"`
	QuoteProvider         string    `gorm:"type:text;not null"`
	QuoteSourceAt         time.Time `gorm:"not null"`
	QuoteRate             string    `gorm:"type:numeric(30,18);not null"`
	QuoteAdjustedRate     string    `gorm:"type:numeric(30,18);not null"`
	QuoteSpreadBPS        int       `gorm:"not null"`
	QuoteExpiresAt        time.Time `gorm:"not null;index"`
	PaymentMethod         *string   `gorm:"type:text"`
	GatewayProvider       *string   `gorm:"type:text"`
	StellarSource         *string   `gorm:"type:text"`
	StellarDestination    *string   `gorm:"type:text"`
	StellarMemo           *string   `gorm:"type:text"`
	WithdrawalMethod      *string   `gorm:"type:text"`
	WithdrawalDestination []byte    `gorm:"type:jsonb"`
	ExpiresAt             *time.Time
	FailureCode           *string `gorm:"type:text"`
	FailureStage          *string `gorm:"type:text"`
	FailureRetryable      *bool
	CreatedAt             time.Time `gorm:"not null;index"`
	UpdatedAt             time.Time `gorm:"not null;index"`
	CompletedAt           *time.Time
}

func (OrderRecord) TableName() string { return "orders" }
