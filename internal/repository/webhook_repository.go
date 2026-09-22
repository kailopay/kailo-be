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

func (r *WebhookRepository) CreateEndpoint(ctx context.Context, record usecase.WebhookEndpointRecord, secretReference []byte) (string, error) {
	var id string
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var client entity.APIClient
		clientErr := tx.Where("id = ? AND status = ?", record.ClientID, "active").First(&client).Error
		if errors.Is(clientErr, gorm.ErrRecordNotFound) {
			clientErr = tx.Where("owner_user_id = ? AND environment = ? AND status = ?", record.ClientID, "test", "active").
				Order("created_at").First(&client).Error
		}
		if clientErr != nil {
			return errors.New("developer api client is required before registering a webhook")
		}
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
		endpoint := entity.WebhookEndpoint{ID: id, ClientID: client.ID, URL: record.URL,
			Status: "active", SecretReference: base64.StdEncoding.EncodeToString(secretReference), EventTypes: eventTypes,
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
	err := r.db.WithContext(ctx).Table("webhook_endpoints AS webhook_endpoint").Select("webhook_endpoint.*").
		Joins("JOIN api_clients AS api_client ON api_client.id = webhook_endpoint.client_id").
		Where("webhook_endpoint.client_id = ? OR api_client.owner_user_id = ?", clientID, clientID).
		Order("webhook_endpoint.created_at DESC").Find(&rows).Error
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
	result := r.db.WithContext(ctx).Table("webhook_endpoints AS webhook_endpoint").
		Where("webhook_endpoint.id = ? AND webhook_endpoint.disabled_at IS NULL AND (webhook_endpoint.client_id = ? OR EXISTS (SELECT 1 FROM api_clients WHERE api_clients.id = webhook_endpoint.client_id AND api_clients.owner_user_id = ?))", endpointID, clientID, clientID).
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
