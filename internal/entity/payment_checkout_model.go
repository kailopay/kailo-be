package entity

import "time"

type PaymentCheckout struct {
	ID                    string  `gorm:"type:uuid;primaryKey"`
	OrderID               string  `gorm:"type:uuid;not null;index:idx_payment_order_created,priority:1"`
	Provider              string  `gorm:"type:text;not null;uniqueIndex:idx_payment_provider_checkout,priority:1"`
	ProviderCheckoutID    string  `gorm:"type:text;not null;uniqueIndex:idx_payment_provider_checkout,priority:2"`
	Method                string  `gorm:"type:text;not null"`
	Currency              string  `gorm:"type:text;not null"`
	AmountMinor           int64   `gorm:"not null"`
	Status                string  `gorm:"type:text;not null"`
	PresentationReference *string `gorm:"type:text"`
	ExpiresAt             *time.Time
	Metadata              []byte    `gorm:"type:jsonb"`
	CreatedAt             time.Time `gorm:"not null;index:idx_payment_order_created,priority:2"`
	UpdatedAt             time.Time `gorm:"not null"`
}

func (PaymentCheckout) TableName() string { return "payment_checkouts" }
