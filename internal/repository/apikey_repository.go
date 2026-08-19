package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/service/apikey"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type APIKeyRepository struct {
	db *gorm.DB
}

func NewAPIKeyRepository(db *gorm.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) DeveloperModeEnabled(ctx context.Context, userID string) (bool, error) {
	var user User
	if err := r.db.WithContext(ctx).Select("developer_enabled_at").
		Where("id = ? AND status = ?", userID, activeUserStatus).First(&user).Error; err != nil {
		return false, fmt.Errorf("finding developer account: %w", err)
	}
	return user.DeveloperEnabledAt != nil, nil
}

func (r *APIKeyRepository) CreateForOwner(
	ctx context.Context,
	ownerUserID, suggestedClientID, clientName string,
	key apikey.Key,
) (string, error) {
	var clientID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND developer_enabled_at IS NOT NULL", ownerUserID, activeUserStatus).
			First(&user).Error; err != nil {
			return fmt.Errorf("locking developer account: %w", err)
		}

		var client APIClient
		err := tx.Where("owner_user_id = ? AND environment = ?", ownerUserID, "test").First(&client).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			client = APIClient{
				ID: suggestedClientID, OwnerUserID: ownerUserID, Name: clientName,
				Environment: "test", Status: "active", CreatedAt: key.CreatedAt, UpdatedAt: key.CreatedAt,
			}
			if err := tx.Create(&client).Error; err != nil {
				return fmt.Errorf("creating api client: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("finding api client: %w", err)
		}
		if client.Status != "active" {
			return apikey.ErrInvalidKey
		}
		clientID = client.ID
		row := APIKey{
			ID: key.ID, ClientID: client.ID, PublicID: key.PublicID, Prefix: key.Prefix,
			SecretHash: append([]byte(nil), key.SecretHash...), CreatedAt: key.CreatedAt,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("creating api key: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return clientID, nil
}

func (r *APIKeyRepository) FindActiveByPublicID(ctx context.Context, publicID string) (apikey.Key, apikey.Principal, error) {
	type result struct {
		APIKey
		OwnerUserID string
	}
	var row result
	err := r.db.WithContext(ctx).Table("api_keys").
		Select("api_keys.*, api_clients.owner_user_id").
		Joins("JOIN api_clients ON api_clients.id = api_keys.client_id").
		Where("api_keys.public_id = ? AND api_keys.revoked_at IS NULL AND api_clients.status = ?", publicID, "active").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apikey.Key{}, apikey.Principal{}, apikey.ErrInvalidKey
	}
	if err != nil {
		return apikey.Key{}, apikey.Principal{}, fmt.Errorf("finding api key: %w", err)
	}
	return apiKey(row.APIKey), apikey.Principal{ClientID: row.ClientID, OwnerUserID: row.OwnerUserID}, nil
}

func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, keyID string, usedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&APIKey{}).
		Where("id = ? AND revoked_at IS NULL", keyID).Update("last_used_at", usedAt.UTC())
	if result.Error != nil {
		return fmt.Errorf("updating api key usage: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apikey.ErrInvalidKey
	}
	return nil
}

func (r *APIKeyRepository) ListForOwner(ctx context.Context, ownerUserID string) ([]apikey.Metadata, error) {
	type result struct {
		APIKey
		Name string
	}
	rows := make([]result, 0)
	if err := r.db.WithContext(ctx).Table("api_keys").
		Select("api_keys.*, api_clients.name").
		Joins("JOIN api_clients ON api_clients.id = api_keys.client_id").
		Where("api_clients.owner_user_id = ?", ownerUserID).
		Order("api_keys.created_at DESC").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}
	keys := make([]apikey.Metadata, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, apikey.Metadata{
			ID: row.ID, ClientID: row.ClientID, Name: row.Name, Prefix: row.Prefix,
			CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt,
		})
	}
	return keys, nil
}

func (r *APIKeyRepository) RevokeForOwner(ctx context.Context, ownerUserID, keyID string, revokedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&APIKey{}).
		Where("id = ? AND revoked_at IS NULL AND client_id IN (?)", keyID,
			r.db.Model(&APIClient{}).Select("id").Where("owner_user_id = ?", ownerUserID)).
		Update("revoked_at", revokedAt.UTC())
	if result.Error != nil {
		return fmt.Errorf("revoking api key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apikey.ErrInvalidKey
	}
	return nil
}

func apiKey(row APIKey) apikey.Key {
	return apikey.Key{
		ID: row.ID, ClientID: row.ClientID, PublicID: row.PublicID, Prefix: row.Prefix,
		SecretHash: append([]byte(nil), row.SecretHash...), CreatedAt: row.CreatedAt,
		LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt,
	}
}
