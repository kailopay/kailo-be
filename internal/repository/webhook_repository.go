package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

// WebhookRepository implements developer endpoint management on the
// webhook_endpoints table created in migration 000001.
type WebhookRepository struct {
	db *gorm.DB
	tx txManager
}

func NewWebhookRepository(db *gorm.DB) *WebhookRepository {
	return &WebhookRepository{db: db, tx: newTxManager(db)}
}

func (r *WebhookRepository) CreateEndpoint(ctx context.Context, record usecase.WebhookEndpointRecord, secretHash []byte) (string, error) {
	var id string
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		generated, err := platform.NewID()
		if err != nil {
			return err
		}
		id = generated
		now := time.Now().UTC()
		eventTypes, err := json.Marshal(record.EventTypes)
		if err != nil {
			return fmt.Errorf("encoding event types: %w", err)
		}
		endpoint := entity.WebhookEndpoint{ID: id, ClientID: record.ClientID, URL: record.URL,
			Status: "active", SecretReference: base64.StdEncoding.EncodeToString(secretHash), EventTypes: eventTypes,
			CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&endpoint).Error; err != nil {
			return fmt.Errorf("creating webhook endpoint: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *WebhookRepository) ListEndpoints(ctx context.Context, clientID string) ([]usecase.WebhookEndpointView, error) {
	var rows []entity.WebhookEndpoint
	err := r.db.WithContext(ctx).Where("client_id = ?", clientID).Order("created_at DESC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing webhook endpoints: %w", err)
	}
	views := make([]usecase.WebhookEndpointView, 0, len(rows))
	for _, row := range rows {
		view := usecase.WebhookEndpointView{ID: row.ID, URL: row.URL, Status: row.Status,
			CreatedAt: row.CreatedAt, DisabledAt: row.DisabledAt}
		if json.Unmarshal(row.EventTypes, &view.EventTypes) != nil || view.EventTypes == nil {
			view.EventTypes = []string{}
		}
		views = append(views, view)
	}
	return views, nil
}

func (r *WebhookRepository) DisableEndpoint(ctx context.Context, clientID, endpointID string) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&entity.WebhookEndpoint{}).
		Where("id = ? AND client_id = ? AND disabled_at IS NULL", endpointID, clientID).
		Updates(map[string]any{"status": "disabled", "disabled_at": now, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("disabling webhook endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("webhook endpoint not found")
	}
	return nil
}

var _ usecase.WebhookEndpointRepository = (*WebhookRepository)(nil)
