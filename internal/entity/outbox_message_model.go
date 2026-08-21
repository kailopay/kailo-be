package entity

import "time"

type OutboxMessage struct {
	ID            string     `gorm:"type:uuid;primaryKey"`
	Topic         string     `gorm:"type:text;not null"`
	AggregateType string     `gorm:"type:text;not null"`
	AggregateID   string     `gorm:"type:uuid;not null;index"`
	Payload       []byte     `gorm:"type:jsonb;not null"`
	CreatedAt     time.Time  `gorm:"not null"`
	AvailableAt   time.Time  `gorm:"not null;index"`
	LeaseOwner    *string    `gorm:"type:text"`
	LeaseUntil    *time.Time `gorm:"index"`
	Attempts      int        `gorm:"not null"`
	ProcessedAt   *time.Time `gorm:"index"`
	LastError     *string    `gorm:"type:text"`
}

func (OutboxMessage) TableName() string { return "outbox_messages" }
