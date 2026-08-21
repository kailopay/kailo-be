package entity

import "time"

type User struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	Status             string  `gorm:"type:text;not null;index"`
	DisplayName        string  `gorm:"type:text;not null"`
	Email              *string `gorm:"type:text"`
	AvatarObjectKey    *string `gorm:"type:text"`
	EmailVerifiedAt    *time.Time
	DeveloperEnabledAt *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (User) TableName() string { return "users" }
