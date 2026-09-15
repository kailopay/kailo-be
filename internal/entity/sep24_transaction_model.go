package entity

import "time"

// SEP24Transaction stores the durable protocol-to-order correlation. Order
// ownership is intentionally resolved through OrderID rather than copied here.
type SEP24Transaction struct {
	ID            string    `gorm:"type:uuid;primaryKey"`
	TransactionID string    `gorm:"type:text;not null;unique"`
	OrderID       string    `gorm:"type:uuid;not null;unique"`
	Kind          string    `gorm:"type:text;not null"`
	CreatedAt     time.Time `gorm:"not null"`
}

func (SEP24Transaction) TableName() string { return "sep24_transactions" }
