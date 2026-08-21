package entity

import "time"

type OrderEvent struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	OrderID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_order_event_version,priority:1"`
	AggregateVersion int       `gorm:"not null;uniqueIndex:idx_order_event_version,priority:2"`
	EventType        string    `gorm:"type:text;not null"`
	PreviousStatus   *string   `gorm:"type:text"`
	NewStatus        *string   `gorm:"type:text"`
	Source           string    `gorm:"type:text;not null"`
	CorrelationID    string    `gorm:"type:text;not null;index"`
	Metadata         []byte    `gorm:"type:jsonb"`
	CreatedAt        time.Time `gorm:"not null"`
}

func (OrderEvent) TableName() string { return "order_events" }
