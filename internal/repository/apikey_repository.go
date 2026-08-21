package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/febry3/kailopay-be/internal/entity"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
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
	var user entity.User
	if err := r.db.WithContext(ctx).Select("developer_enabled_at").
		Where("id = ? AND status = ?", userID, activeUserStatus).First(&user).Error; err != nil {
		return false, fmt.Errorf("finding developer account: %w", err)
	}
	return user.DeveloperEnabledAt != nil, nil
}

func (r *APIKeyRepository) CreateForOwner(
	ctx context.Context,
	ownerUserID, suggestedClientID, clientName string,
	key usecase.Key,
) (string, error) {
	var clientID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ? AND developer_enabled_at IS NOT NULL", ownerUserID, activeUserStatus).
			First(&user).Error; err != nil {
			return fmt.Errorf("locking developer account: %w", err)
		}

		var client entity.APIClient
		err := tx.Where("owner_user_id = ? AND environment = ?", ownerUserID, "test").First(&client).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			client = entity.APIClient{
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
			return usecase.ErrInvalidKey
		}
		clientID = client.ID
		row := entity.APIKey{
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

func (r *APIKeyRepository) FindActiveByPublicID(ctx context.Context, publicID string) (usecase.Key, usecase.Principal, error) {
	type result struct {
		entity.APIKey
		OwnerUserID string
	}
	var row result
	err := r.db.WithContext(ctx).Table("api_keys").
		Select("api_keys.*, api_clients.owner_user_id").
		Joins("JOIN api_clients ON api_clients.id = api_keys.client_id").
		Where("api_keys.public_id = ? AND api_keys.revoked_at IS NULL AND api_clients.status = ?", publicID, "active").
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.Key{}, usecase.Principal{}, usecase.ErrInvalidKey
	}
	if err != nil {
		return usecase.Key{}, usecase.Principal{}, fmt.Errorf("finding api key: %w", err)
	}
	return apiKey(row.APIKey), usecase.Principal{ClientID: row.ClientID, OwnerUserID: row.OwnerUserID}, nil
}

func (r *APIKeyRepository) TouchLastUsed(ctx context.Context, keyID string, usedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.APIKey{}).
		Where("id = ? AND revoked_at IS NULL", keyID).Update("last_used_at", usedAt.UTC())
	if result.Error != nil {
		return fmt.Errorf("updating api key usage: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return usecase.ErrInvalidKey
	}
	return nil
}

func (r *APIKeyRepository) ListForOwner(ctx context.Context, ownerUserID string) ([]usecase.Metadata, error) {
	type result struct {
		entity.APIKey
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
	keys := make([]usecase.Metadata, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, usecase.Metadata{
			ID: row.ID, ClientID: row.ClientID, Name: row.Name, Prefix: row.Prefix,
			CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt,
		})
	}
	return keys, nil
}

func (r *APIKeyRepository) RevokeForOwner(ctx context.Context, ownerUserID, keyID string, revokedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.APIKey{}).
		Where("id = ? AND revoked_at IS NULL AND client_id IN (?)", keyID,
			r.db.Model(&entity.APIClient{}).Select("id").Where("owner_user_id = ?", ownerUserID)).
		Update("revoked_at", revokedAt.UTC())
	if result.Error != nil {
		return fmt.Errorf("revoking api key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return usecase.ErrInvalidKey
	}
	return nil
}

func apiKey(row entity.APIKey) usecase.Key {
	return usecase.Key{
		ID: row.ID, ClientID: row.ClientID, PublicID: row.PublicID, Prefix: row.Prefix,
		SecretHash: append([]byte(nil), row.SecretHash...), CreatedAt: row.CreatedAt,
		LastUsedAt: row.LastUsedAt, RevokedAt: row.RevokedAt,
	}
}
