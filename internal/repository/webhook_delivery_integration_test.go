package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestWebhookDeliveryLeaseAndReplayAreBoundedAndIdempotent(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 5, 0, 0, 0, time.UTC)
	clientID := "00000000-0000-4000-8000-0000000000c1"
	ownerID := "00000000-0000-4000-8000-0000000000aa"
	endpointID := "00000000-0000-4000-8000-0000000000e1"
	eventID := "00000000-0000-4000-8000-0000000000f1"
	outboxID := "00000000-0000-4000-8000-0000000000f2"
	if err := store.db.Create(&entity.WebhookEndpoint{ID: endpointID, ClientID: clientID, URL: "https://callback.example/webhook", Status: "active",
		SecretReference: "cmVmZXJlbmNl", EventTypes: []byte(`["order.created"]`), CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("creating endpoint: %v", err)
	}
	clientPtr := clientID
	if err := store.db.Create(&entity.WebhookEvent{ID: eventID, ClientID: &clientPtr, EventType: usecase.EventOrderCreated,
		APIVersion: usecase.WebhookAPIVersion, CanonicalPayload: []byte(`{"id":"evt_test"}`), CreatedAt: now}).Error; err != nil {
		t.Fatalf("creating event: %v", err)
	}
	payload, err := json.Marshal(map[string]string{"event_id": eventID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.db.Create(&entity.OutboxMessage{ID: outboxID, Topic: "webhook.deliver", AggregateType: "webhook_event",
		AggregateID: eventID, Payload: payload, CreatedAt: now, AvailableAt: now}).Error; err != nil {
		t.Fatalf("creating outbox: %v", err)
	}

	repository := NewWebhookRepository(store.db)
	attempt, started, err := repository.StartAttempt(ctx, eventID, endpointID, "worker-1", now, time.Minute, 1)
	if err != nil || !started {
		t.Fatalf("StartAttempt() = %#v/%t/%v, want started", attempt, started, err)
	}
	if _, started, err := repository.StartAttempt(ctx, eventID, endpointID, "worker-2", now, time.Minute, 1); err != nil || started {
		t.Fatalf("second StartAttempt() = started %t, err %v, want no second lease", started, err)
	}
	status := usecase.WebhookDeliveryExhausted
	httpStatus := 503
	if err := repository.CompleteAttempt(ctx, attempt.ID, usecase.WebhookDeliveryResult{Status: status, HTTPStatus: &httpStatus}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}
	if err := repository.FinalizeEvent(ctx, outboxID, eventID, now); err != nil {
		t.Fatalf("FinalizeEvent() error = %v", err)
	}
	if err := repository.ReplayDelivery(ctx, ownerID, attempt.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("ReplayDelivery() error = %v", err)
	}
	if err := repository.ReplayDelivery(ctx, ownerID, attempt.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("idempotent ReplayDelivery() error = %v", err)
	}
	var replayCount int64
	if err := store.db.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "webhook.deliver", eventID).Count(&replayCount).Error; err != nil {
		t.Fatalf("counting replay outbox: %v", err)
	}
	if replayCount != 1 {
		t.Fatalf("unprocessed replay outboxes = %d, want 1", replayCount)
	}
}
