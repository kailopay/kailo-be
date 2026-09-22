package entity

import "time"

type DeveloperWallet struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	UserID             string    `gorm:"type:uuid;not null;uniqueIndex:idx_developer_wallet_owner_network_account,priority:1;index"`
	ClientID           *string   `gorm:"type:uuid;index"`
	Network            string    `gorm:"type:text;not null;uniqueIndex:idx_developer_wallet_owner_network_account,priority:2"`
	WalletAccount      string    `gorm:"type:text;not null;uniqueIndex:idx_developer_wallet_owner_network_account,priority:3"`
	Label              string    `gorm:"type:text;not null"`
	IsPrimary          bool      `gorm:"not null"`
	VerificationMethod string    `gorm:"type:text;not null"`
	VerifiedAt         time.Time `gorm:"not null"`
	Status             string    `gorm:"type:text;not null"`
	RevokedAt          *time.Time
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (DeveloperWallet) TableName() string { return "developer_wallets" }
