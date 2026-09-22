package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *WebhookRepository) GetEndpoint(ctx context.Context, ownerID, endpointID string) (usecase.WebhookEndpointView, error) {
	var row entity.WebhookEndpoint
	if err := ownedWebhookEndpointQuery(r.db.WithContext(ctx), ownerID, endpointID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.WebhookEndpointView{}, usecase.ErrWebhookEndpointNotFound
		}
		return usecase.WebhookEndpointView{}, fmt.Errorf("finding webhook endpoint: %w", err)
	}
	return webhookEndpointView(row), nil
}

func (r *WebhookRepository) ListDeliveries(ctx context.Context, ownerID string, query usecase.WebhookDeliveryQuery) (usecase.WebhookDeliveryPage, error) {
	type deliveryRow struct {
		ID             string
		EventID        string
		EndpointID     string
		EventType      string
		AttemptNumber  int
		Status         string
		ScheduledAt    time.Time
		StartedAt      *time.Time
		CompletedAt    *time.Time
		HTTPStatus     *int
		DurationMillis *int64
		ResponseHash   *string
		SafeError      *string
		NextAttemptAt  *time.Time
	}
	queryDB := r.db.WithContext(ctx).Table("webhook_attempts AS wa").
		Select("wa.id, wa.event_id, wa.endpoint_id, we.event_type, wa.attempt_number, wa.status, wa.scheduled_at, wa.started_at, wa.completed_at, wa.http_status, wa.duration_millis, wa.response_body_hash, wa.safe_error, wa.next_attempt_at").
		Joins("JOIN webhook_events AS we ON we.id = wa.event_id").
		Joins("JOIN webhook_endpoints AS endpoint ON endpoint.id = wa.endpoint_id").
		Joins("JOIN api_clients AS client ON client.id = endpoint.client_id").
		Where("client.owner_user_id = ?", ownerID)
	if query.EndpointID != "" {
		queryDB = queryDB.Where("wa.endpoint_id = ?", query.EndpointID)
	}
	if query.EventID != "" {
		queryDB = queryDB.Where("wa.event_id = ?", query.EventID)
	}
	if query.Status != "" {
		queryDB = queryDB.Where("wa.status = ?", query.Status)
	}
	if query.From != nil {
		queryDB = queryDB.Where("wa.scheduled_at >= ?", query.From.UTC())
	}
	if query.To != nil {
		queryDB = queryDB.Where("wa.scheduled_at < ?", query.To.UTC())
	}
	if query.Cursor != "" {
		cursor, err := decodeDeveloperCursor(query.Cursor)
		if err != nil {
			return usecase.WebhookDeliveryPage{}, err
		}
		queryDB = queryDB.Where("(wa.scheduled_at < ? OR (wa.scheduled_at = ? AND wa.id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	rows := make([]deliveryRow, 0, limit+1)
	if err := queryDB.Order("wa.scheduled_at DESC, wa.id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return usecase.WebhookDeliveryPage{}, fmt.Errorf("listing webhook deliveries: %w", err)
	}
	page := usecase.WebhookDeliveryPage{Deliveries: make([]usecase.WebhookDeliveryView, 0, len(rows))}
	if len(rows) > limit {
		last := rows[limit-1]
		cursor, err := encodeDeveloperCursor(developerCursor{CreatedAt: last.ScheduledAt, ID: last.ID})
		if err != nil {
			return usecase.WebhookDeliveryPage{}, err
		}
		page.NextPagingID = cursor
		rows = rows[:limit]
	}
	for _, row := range rows {
		page.Deliveries = append(page.Deliveries, usecase.WebhookDeliveryView{ID: row.ID, EventID: row.EventID,
			EndpointID: row.EndpointID, EventType: row.EventType, AttemptNumber: row.AttemptNumber, Status: row.Status,
			ScheduledAt: row.ScheduledAt, StartedAt: row.StartedAt, CompletedAt: row.CompletedAt, HTTPStatus: row.HTTPStatus,
			DurationMillis: row.DurationMillis, SafeError: row.SafeError, NextAttemptAt: row.NextAttemptAt,
			CreatedAt: row.ScheduledAt})
	}
	return page, nil
}

func (r *WebhookRepository) EnqueueTest(ctx context.Context, ownerID, endpointID string, now time.Time) (usecase.WebhookQueuedEvent, error) {
	var queued usecase.WebhookQueuedEvent
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var endpoint entity.WebhookEndpoint
		if err := ownedWebhookEndpointQuery(tx, ownerID, endpointID).First(&endpoint).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrWebhookEndpointNotFound
			}
			return fmt.Errorf("finding webhook endpoint: %w", err)
		}
		if endpoint.Status != "active" || endpoint.DisabledAt != nil {
			return usecase.ErrWebhookEndpointNotFound
		}
		var eventTypes []string
		if err := json.Unmarshal(endpoint.EventTypes, &eventTypes); err != nil || len(eventTypes) == 0 {
			return errors.New("webhook endpoint subscriptions are invalid")
		}
		eventID, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generating webhook test event id: %w", err)
		}
		payload, err := usecase.BuildTestWebhookEnvelope(eventID, eventTypes[0], "sandbox", endpoint.ID, now)
		if err != nil {
			return fmt.Errorf("building webhook test payload: %w", err)
		}
		clientID := endpoint.ClientID
		event := entity.WebhookEvent{ID: eventID, ClientID: &clientID, EventType: eventTypes[0],
			APIVersion: usecase.WebhookAPIVersion, CanonicalPayload: payload, IsTest: true, CreatedAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("creating webhook test event: %w", err)
		}
		outboxID, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generating webhook test outbox id: %w", err)
		}
		outboxPayload, err := json.Marshal(map[string]string{"event_id": eventID})
		if err != nil {
			return fmt.Errorf("encoding webhook test outbox: %w", err)
		}
		outbox := entity.OutboxMessage{ID: outboxID, Topic: "webhook.deliver", AggregateType: "webhook_event",
			AggregateID: eventID, Payload: outboxPayload, CreatedAt: now, AvailableAt: now}
		if err := tx.Create(&outbox).Error; err != nil {
			return fmt.Errorf("creating webhook test outbox: %w", err)
		}
		queued = usecase.WebhookQueuedEvent{EventID: eventID, OutboxID: outboxID}
		return nil
	})
	return queued, err
}

func (r *WebhookRepository) ReplayDelivery(ctx context.Context, ownerID, deliveryID string, now time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var attempt entity.WebhookAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", deliveryID).First(&attempt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrWebhookDeliveryNotFound
			}
			return fmt.Errorf("finding webhook delivery: %w", err)
		}
		var endpoint entity.WebhookEndpoint
		if err := ownedWebhookEndpointQuery(tx, ownerID, attempt.EndpointID).First(&endpoint).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrWebhookDeliveryNotFound
			}
			return fmt.Errorf("checking webhook delivery ownership: %w", err)
		}
		var existing entity.OutboxMessage
		existingErr := tx.Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "webhook.deliver", attempt.EventID).First(&existing).Error
		if attempt.Status == "replay_pending" && existingErr == nil {
			return nil
		}
		if attempt.Status != usecase.WebhookDeliveryExhausted {
			return usecase.ErrWebhookReplayNotAllowed
		}
		if existingErr == nil {
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("checking webhook replay outbox: %w", existingErr)
		}
		if err := tx.Model(&entity.WebhookAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{
			"status": "replay_pending", "next_attempt_at": now.UTC(), "safe_error": "manual webhook replay requested",
			"lease_owner": nil, "lease_until": nil,
		}).Error; err != nil {
			return fmt.Errorf("marking webhook replay: %w", err)
		}
		outboxID, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generating webhook replay outbox id: %w", err)
		}
		payload, err := json.Marshal(map[string]string{"event_id": attempt.EventID})
		if err != nil {
			return fmt.Errorf("encoding webhook replay outbox: %w", err)
		}
		if err := tx.Create(&entity.OutboxMessage{ID: outboxID, Topic: "webhook.deliver", AggregateType: "webhook_event",
			AggregateID: attempt.EventID, Payload: payload, CreatedAt: now.UTC(), AvailableAt: now.UTC()}).Error; err != nil {
			return fmt.Errorf("creating webhook replay outbox: %w", err)
		}
		return nil
	})
}

func ownedWebhookEndpointQuery(db *gorm.DB, ownerID, endpointID string) *gorm.DB {
	return db.Table("webhook_endpoints AS webhook_endpoint").Select("webhook_endpoint.*").
		Joins("JOIN api_clients AS api_client ON api_client.id = webhook_endpoint.client_id").
		Where("webhook_endpoint.id = ? AND (webhook_endpoint.client_id = ? OR api_client.owner_user_id = ?)", endpointID, ownerID, ownerID)
}

func webhookEndpointView(row entity.WebhookEndpoint) usecase.WebhookEndpointView {
	view := usecase.WebhookEndpointView{ID: row.ID, URL: row.URL, Status: row.Status, CreatedAt: row.CreatedAt, DisabledAt: row.DisabledAt, EventTypes: []string{}}
	if err := json.Unmarshal(row.EventTypes, &view.EventTypes); err != nil || view.EventTypes == nil {
		view.EventTypes = []string{}
	}
	return view
}

var _ usecase.WebhookControlRepository = (*WebhookRepository)(nil)
