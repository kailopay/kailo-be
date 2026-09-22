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
	"gorm.io/gorm/clause"
)

// LoadEvent returns the immutable event payload stored with the order
// transition. The payload is copied so callers cannot mutate the repository
// row representation.
func (r *WebhookRepository) LoadEvent(ctx context.Context, eventID string) (usecase.WebhookEventRecord, error) {
	var row entity.WebhookEvent
	if err := r.db.WithContext(ctx).Where("id = ?", eventID).First(&row).Error; err != nil {
		return usecase.WebhookEventRecord{}, fmt.Errorf("finding webhook event: %w", err)
	}
	return usecase.WebhookEventRecord{ID: row.ID, EventType: row.EventType, APIVersion: row.APIVersion,
		Payload: append([]byte(nil), row.CanonicalPayload...)}, nil
}

// ListDeliveryEndpoints resolves the order's API client and filters active
// endpoint subscriptions. Retail-session orders have no client and therefore
// intentionally produce no developer deliveries.
func (r *WebhookRepository) ListDeliveryEndpoints(ctx context.Context, eventID, eventType string) ([]usecase.WebhookDeliveryEndpoint, error) {
	var order struct {
		ClientID *string
	}
	if err := r.db.WithContext(ctx).Table("webhook_events AS webhook_event").
		Select("COALESCE(webhook_event.client_id, orders.client_id) AS client_id").
		Joins("LEFT JOIN orders ON orders.id = webhook_event.order_id").
		Where("webhook_event.id = ?", eventID).Scan(&order).Error; err != nil {
		return nil, fmt.Errorf("finding webhook event client: %w", err)
	}
	if order.ClientID == nil || *order.ClientID == "" {
		return []usecase.WebhookDeliveryEndpoint{}, nil
	}
	var rows []entity.WebhookEndpoint
	if err := r.db.WithContext(ctx).Where("client_id = ? AND status = ? AND disabled_at IS NULL", *order.ClientID, "active").
		Order("created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing active webhook endpoints: %w", err)
	}
	endpoints := make([]usecase.WebhookDeliveryEndpoint, 0, len(rows))
	for _, row := range rows {
		var eventTypes []string
		if err := unmarshalWebhookEventTypes(row.EventTypes, &eventTypes); err != nil {
			return nil, err
		}
		if !containsString(eventTypes, eventType) {
			continue
		}
		secretReference, err := base64.StdEncoding.DecodeString(row.SecretReference)
		if err != nil || len(secretReference) == 0 {
			return nil, errors.New("webhook secret reference is invalid")
		}
		endpoints = append(endpoints, usecase.WebhookDeliveryEndpoint{ID: row.ID, URL: row.URL, SecretReference: secretReference})
	}
	return endpoints, nil
}

func unmarshalWebhookEventTypes(raw []byte, eventTypes *[]string) error {
	if err := json.Unmarshal(raw, eventTypes); err != nil || eventTypes == nil || *eventTypes == nil {
		return errors.New("webhook event subscriptions are invalid")
	}
	return nil
}

// StartAttempt obtains an attempt-level lease. An expired in-flight attempt
// is converted into a new attempt number so a late response cannot overwrite
// the result owned by a newer worker.
func (r *WebhookRepository) StartAttempt(ctx context.Context, eventID, endpointID, workerID string, now time.Time, leaseDuration time.Duration, maxAttempts int) (usecase.WebhookDeliveryAttempt, bool, error) {
	var attempt usecase.WebhookDeliveryAttempt
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var latest entity.WebhookAttempt
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("event_id = ? AND endpoint_id = ?", eventID, endpointID).
			Order("attempt_number DESC").First(&latest).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			latest.AttemptNumber = 0
		} else if err != nil {
			return fmt.Errorf("finding latest webhook attempt: %w", err)
		}
		manualReplay := false
		if latest.AttemptNumber > 0 {
			switch latest.Status {
			case usecase.WebhookDeliveryDelivered, usecase.WebhookDeliveryFailed, usecase.WebhookDeliveryExhausted:
				return nil
			case "in_flight":
				if latest.LeaseUntil != nil && latest.LeaseUntil.After(now) {
					return nil
				}
				if latest.AttemptNumber >= maxAttempts {
					return tx.Model(&entity.WebhookAttempt{}).Where("id = ?", latest.ID).
						Updates(map[string]any{"status": usecase.WebhookDeliveryExhausted, "completed_at": now,
							"safe_error": "webhook delivery lease expired", "lease_owner": nil, "lease_until": nil}).Error
				}
				if err := tx.Model(&entity.WebhookAttempt{}).Where("id = ?", latest.ID).
					Updates(map[string]any{"status": "superseded", "completed_at": now, "safe_error": "webhook delivery lease expired",
						"lease_owner": nil, "lease_until": nil}).Error; err != nil {
					return fmt.Errorf("superseding expired webhook attempt: %w", err)
				}
			case "retry_scheduled":
				if latest.NextAttemptAt != nil && latest.NextAttemptAt.After(now) {
					return nil
				}
				if err := tx.Model(&entity.WebhookAttempt{}).Where("id = ?", latest.ID).
					Update("status", "superseded").Error; err != nil {
					return fmt.Errorf("superseding webhook retry: %w", err)
				}
			case "replay_pending":
				manualReplay = true
				if err := tx.Model(&entity.WebhookAttempt{}).Where("id = ?", latest.ID).
					Updates(map[string]any{"status": "superseded", "completed_at": now, "lease_owner": nil, "lease_until": nil}).Error; err != nil {
					return fmt.Errorf("starting webhook replay: %w", err)
				}
			}
		}
		nextNumber := latest.AttemptNumber + 1
		if nextNumber > maxAttempts && !manualReplay {
			return nil
		}
		id, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generating webhook attempt id: %w", err)
		}
		startedAt := now.UTC()
		leaseUntil := startedAt.Add(leaseDuration)
		row := entity.WebhookAttempt{ID: id, EventID: eventID, EndpointID: endpointID, AttemptNumber: nextNumber,
			Status: "in_flight", ScheduledAt: startedAt, StartedAt: &startedAt, LeaseOwner: &workerID, LeaseUntil: &leaseUntil}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("creating webhook attempt: %w", err)
		}
		attempt = usecase.WebhookDeliveryAttempt{ID: row.ID, EventID: row.EventID, EndpointID: row.EndpointID, AttemptNumber: row.AttemptNumber}
		return nil
	})
	if err != nil {
		return usecase.WebhookDeliveryAttempt{}, false, err
	}
	return attempt, attempt.ID != "", nil
}

func (r *WebhookRepository) CompleteAttempt(ctx context.Context, attemptID string, result usecase.WebhookDeliveryResult) error {
	updates := map[string]any{
		"status": result.Status, "completed_at": time.Now().UTC(), "duration_millis": result.DurationMillis,
		"lease_owner": nil, "lease_until": nil, "next_attempt_at": result.NextAttemptAt,
		"safe_error": result.SafeError, "response_body_hash": result.ResponseBodyHash,
	}
	if result.HTTPStatus != nil {
		updates["http_status"] = *result.HTTPStatus
	}
	query := r.db.WithContext(ctx).Model(&entity.WebhookAttempt{}).Where("id = ? AND status = ?", attemptID, "in_flight").Updates(updates)
	if query.Error != nil {
		return fmt.Errorf("updating webhook attempt: %w", query.Error)
	}
	if query.RowsAffected == 0 {
		return errors.New("webhook attempt is no longer leased")
	}
	return nil
}

// FinalizeEvent either requeues the event at its earliest retry time or marks
// the delivery outbox processed after every endpoint reached a terminal state.
func (r *WebhookRepository) FinalizeEvent(ctx context.Context, outboxID, eventID string, now time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var retry entity.WebhookAttempt
		err := tx.Where("event_id = ? AND status = ? AND next_attempt_at IS NOT NULL", eventID, usecase.WebhookDeliveryRetryScheduled).
			Order("next_attempt_at").First(&retry).Error
		if err == nil {
			return tx.Model(&entity.OutboxMessage{}).Where("id = ? AND processed_at IS NULL", outboxID).
				Updates(map[string]any{"available_at": retry.NextAttemptAt, "lease_owner": nil, "lease_until": nil,
					"last_error": retry.SafeError}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("finding webhook retry: %w", err)
		}
		var active entity.WebhookAttempt
		err = tx.Where("event_id = ? AND status = ?", eventID, "in_flight").Order("lease_until").First(&active).Error
		if err == nil {
			availableAt := now
			if active.LeaseUntil != nil && active.LeaseUntil.After(availableAt) {
				availableAt = *active.LeaseUntil
			}
			return tx.Model(&entity.OutboxMessage{}).Where("id = ? AND processed_at IS NULL", outboxID).
				Updates(map[string]any{"available_at": availableAt, "lease_owner": nil, "lease_until": nil}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("finding active webhook attempt: %w", err)
		}
		return tx.Model(&entity.OutboxMessage{}).Where("id = ? AND processed_at IS NULL", outboxID).
			Updates(map[string]any{"processed_at": now.UTC(), "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var _ usecase.WebhookDeliveryRepository = (*WebhookRepository)(nil)
