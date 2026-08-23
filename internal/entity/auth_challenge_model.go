package entity

import "time"

type AuthChallenge struct {
	ID         string     `gorm:"type:uuid;primaryKey"`
	UserID     string     `gorm:"type:uuid;not null;index:idx_auth_challenges_user_purpose,priority:1"`
	TokenHash  []byte     `gorm:"not null;uniqueIndex:idx_auth_challenges_token"`
	Purpose    string     `gorm:"type:text;not null;index:idx_auth_challenges_user_purpose,priority:2"`
	ExpiresAt  time.Time  `gorm:"not null"`
	ConsumedAt *time.Time `gorm:"index"`
	CreatedAt  time.Time  `gorm:"not null"`
}

func (AuthChallenge) TableName() string { return "auth_challenges" }
