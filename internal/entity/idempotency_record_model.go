package entity

import "time"

type IdempotencyRecord struct {
	ID                string  `gorm:"type:uuid;primaryKey"`
	ClientID          *string `gorm:"type:uuid"`
	RetailUserID      *string `gorm:"type:uuid"`
	Operation         string  `gorm:"type:text;not null"`
	KeyHash           string  `gorm:"type:text;not null"`
	RequestHash       string  `gorm:"type:text;not null"`
	ResponseStatus    *int
	ResponseBody      []byte    `gorm:"type:jsonb"`
	CreatedResourceID *string   `gorm:"type:uuid"`
	State             string    `gorm:"type:text;not null"`
	ExpiresAt         time.Time `gorm:"not null;index"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_records" }
