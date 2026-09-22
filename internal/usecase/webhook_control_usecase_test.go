package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeWebhookControlRepository struct {
	endpointOwner string
	endpointID    string
	page          WebhookDeliveryPage
	queued        WebhookQueuedEvent
	replayErr     error
	lastQuery     WebhookDeliveryQuery
	lastOwner     string
}

func (r *fakeWebhookControlRepository) GetEndpoint(_ context.Context, ownerID, endpointID string) (WebhookEndpointView, error) {
	r.lastOwner, r.endpointID = ownerID, endpointID
	if ownerID != r.endpointOwner || endpointID == "" {
		return WebhookEndpointView{}, ErrWebhookEndpointNotFound
	}
	return WebhookEndpointView{ID: endpointID, URL: "https://callback.example/webhook"}, nil
}

func (r *fakeWebhookControlRepository) ListDeliveries(_ context.Context, ownerID string, query WebhookDeliveryQuery) (WebhookDeliveryPage, error) {
	r.lastOwner, r.lastQuery = ownerID, query
	return r.page, nil
}

func (r *fakeWebhookControlRepository) EnqueueTest(_ context.Context, ownerID, endpointID string, _ time.Time) (WebhookQueuedEvent, error) {
	r.lastOwner, r.endpointID = ownerID, endpointID
	return r.queued, nil
}

func (r *fakeWebhookControlRepository) ReplayDelivery(_ context.Context, ownerID, deliveryID string, _ time.Time) error {
	r.lastOwner, r.endpointID = ownerID, deliveryID
	return r.replayErr
}

func TestWebhookControlUsecaseValidatesAndBoundsDeliveryQuery(t *testing.T) {
	repository := &fakeWebhookControlRepository{endpointOwner: "user-1", page: WebhookDeliveryPage{Deliveries: []WebhookDeliveryView{}}}
	service, err := NewWebhookControlUsecase(repository, func() time.Time { return time.Unix(1_700_000_000, 0) })
	if err != nil {
		t.Fatalf("NewWebhookControlUsecase() error = %v", err)
	}

	page, err := service.ListDeliveries(context.Background(), "user-1", WebhookDeliveryQuery{Status: WebhookDeliveryRetryScheduled})
	if err != nil {
		t.Fatalf("ListDeliveries() error = %v", err)
	}
	if page.Deliveries == nil || repository.lastQuery.Limit != 20 || repository.lastQuery.Status != WebhookDeliveryRetryScheduled {
		t.Fatalf("page/query = %#v/%#v, want initialized list and limit 20", page, repository.lastQuery)
	}
}

func TestWebhookControlUsecaseKeepsReplayOwnershipError(t *testing.T) {
	sentinel := errors.New("delivery is not owned")
	repository := &fakeWebhookControlRepository{replayErr: sentinel}
	service, err := NewWebhookControlUsecase(repository, time.Now)
	if err != nil {
		t.Fatalf("NewWebhookControlUsecase() error = %v", err)
	}
	if err := service.ReplayDelivery(context.Background(), "user-1", "attempt-1"); !errors.Is(err, sentinel) {
		t.Fatalf("ReplayDelivery() error = %v, want sentinel", err)
	}
}
