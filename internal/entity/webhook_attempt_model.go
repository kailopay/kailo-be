package entity

import "time"

type WebhookAttempt struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	EventID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_webhook_attempt,priority:1"`
	EndpointID       string    `gorm:"type:uuid;not null;uniqueIndex:idx_webhook_attempt,priority:2"`
	AttemptNumber    int       `gorm:"not null;uniqueIndex:idx_webhook_attempt,priority:3"`
	Status           string    `gorm:"type:text;not null"`
	ScheduledAt      time.Time `gorm:"not null;index"`
	StartedAt        *time.Time
	CompletedAt      *time.Time
	HTTPStatus       *int
	DurationMillis   *int64
	ResponseBodyHash *string    `gorm:"type:text"`
	SafeError        *string    `gorm:"type:text"`
	NextAttemptAt    *time.Time `gorm:"index"`
}

func (WebhookAttempt) TableName() string { return "webhook_attempts" }
