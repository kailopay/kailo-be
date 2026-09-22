package entity

import "time"

// OrderFinancial is the immutable financial snapshot captured when an order
// is created. Historical reporting must use this record instead of mutable
// quote or order fields.
type OrderFinancial struct {
	ID                    string    `gorm:"type:uuid;primaryKey"`
	OrderID               string    `gorm:"type:uuid;not null;unique"`
	ClientID              *string   `gorm:"type:uuid;index"`
	Direction             string    `gorm:"type:text;not null"`
	Environment           string    `gorm:"type:text;not null"`
	Network               string    `gorm:"type:text;not null"`
	Currency              string    `gorm:"type:text;not null"`
	AssetCode             string    `gorm:"type:text;not null"`
	AssetIssuer           string    `gorm:"type:text;not null"`
	AssetAmount           string    `gorm:"type:numeric(30,18);not null"`
	AssetAmountStroops    int64     `gorm:"not null"`
	GrossAmountMinor      int64     `gorm:"not null"`
	FeeAmountMinor        int64     `gorm:"not null"`
	PlatformRevenueMinor  int64     `gorm:"not null"`
	DeveloperRevenueMinor int64     `gorm:"not null"`
	NetAmountMinor        int64     `gorm:"not null"`
	FeeCurrency           string    `gorm:"type:text;not null"`
	FeePolicyVersion      string    `gorm:"type:text;not null"`
	Source                string    `gorm:"type:text;not null"`
	QuoteSpreadBPS        int       `gorm:"not null"`
	Simulated             bool      `gorm:"not null"`
	CreatedAt             time.Time `gorm:"not null;index"`
}

func (OrderFinancial) TableName() string { return "order_financials" }
