package entity

import "time"

// SEP24InteractiveSession keeps the browser hand-off separate from order
// creation. It is bound to the authenticated wallet and becomes linked to a
// retail user only after the interactive flow completes.
type SEP24InteractiveSession struct {
	ID               string  `gorm:"type:uuid;primaryKey"`
	TransactionID    string  `gorm:"type:text;not null;unique"`
	Kind             string  `gorm:"type:text;not null"`
	WalletAccount    string  `gorm:"type:text;not null;index"`
	BrowserTokenHash []byte  `gorm:"type:bytea;not null;unique"`
	RequestHash      []byte  `gorm:"type:bytea;not null"`
	RequestPayload   []byte  `gorm:"type:jsonb;not null"`
	AssetCode        string  `gorm:"type:text;not null"`
	AssetAmount      *string `gorm:"type:text"`
	FiatAmountMinor  *int64
	PaymentMethod    *string   `gorm:"type:text"`
	DestinationToken []byte    `gorm:"type:bytea"`
	QuoteID          *string   `gorm:"type:text;index"`
	LinkedUserID     *string   `gorm:"type:uuid;index"`
	LinkedSessionID  *string   `gorm:"type:uuid;index"`
	KYCStatus        string    `gorm:"type:text;not null"`
	OrderID          *string   `gorm:"type:uuid;unique"`
	ExpiresAt        time.Time `gorm:"not null;index"`
	CompletedAt      *time.Time
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (SEP24InteractiveSession) TableName() string { return "sep24_interactive_sessions" }
