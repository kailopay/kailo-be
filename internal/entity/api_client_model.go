package entity

import "time"

type APIClient struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	OwnerUserID string    `gorm:"type:uuid;not null;index;uniqueIndex:idx_api_client_owner_environment,priority:1"`
	Name        string    `gorm:"type:text;not null"`
	Environment string    `gorm:"type:text;not null;uniqueIndex:idx_api_client_owner_environment,priority:2"`
	Status      string    `gorm:"type:text;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (APIClient) TableName() string { return "api_clients" }
