package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrWebhookEndpointNotFound = errors.New("webhook endpoint not found")
	ErrWebhookDeliveryNotFound = errors.New("webhook delivery not found")
	ErrWebhookReplayNotAllowed = errors.New("webhook delivery cannot be replayed")
)

type WebhookDeliveryQuery struct {
	EndpointID string
	EventID    string
	Status     string
	From       *time.Time
	To         *time.Time
	Limit      int
	Cursor     string
}

type WebhookDeliveryView struct {
	ID             string     `json:"id"`
	EventID        string     `json:"event_id"`
	EndpointID     string     `json:"endpoint_id"`
	EventType      string     `json:"event_type"`
	AttemptNumber  int        `json:"attempt_number"`
	Status         string     `json:"status"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	HTTPStatus     *int       `json:"http_status,omitempty"`
	DurationMillis *int64     `json:"duration_millis,omitempty"`
	SafeError      *string    `json:"safe_error,omitempty"`
	NextAttemptAt  *time.Time `json:"next_attempt_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type WebhookDeliveryPage struct {
	Deliveries   []WebhookDeliveryView `json:"deliveries"`
	NextPagingID string                `json:"paging_id,omitempty"`
}

type WebhookQueuedEvent struct {
	EventID  string
	OutboxID string
}

type WebhookControlRepository interface {
	GetEndpoint(ctx context.Context, ownerID, endpointID string) (WebhookEndpointView, error)
	ListDeliveries(ctx context.Context, ownerID string, query WebhookDeliveryQuery) (WebhookDeliveryPage, error)
	EnqueueTest(ctx context.Context, ownerID, endpointID string, now time.Time) (WebhookQueuedEvent, error)
	ReplayDelivery(ctx context.Context, ownerID, deliveryID string, now time.Time) error
}

type WebhookControlUsecase struct {
	repository WebhookControlRepository
	now        func() time.Time
}

func NewWebhookControlUsecase(repository WebhookControlRepository, now func() time.Time) (*WebhookControlUsecase, error) {
	if repository == nil || now == nil {
		return nil, errors.New("valid webhook control dependencies are required")
	}
	return &WebhookControlUsecase{repository: repository, now: now}, nil
}

func (s *WebhookControlUsecase) GetEndpoint(ctx context.Context, ownerID, endpointID string) (WebhookEndpointView, error) {
	if ownerID == "" || endpointID == "" {
		return WebhookEndpointView{}, ErrWebhookEndpointNotFound
	}
	return s.repository.GetEndpoint(ctx, ownerID, endpointID)
}

func (s *WebhookControlUsecase) ListDeliveries(ctx context.Context, ownerID string, query WebhookDeliveryQuery) (WebhookDeliveryPage, error) {
	if ownerID == "" {
		return WebhookDeliveryPage{}, errors.New("webhook owner is required")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 {
		return WebhookDeliveryPage{}, errors.New("webhook delivery limit is invalid")
	}
	if query.Status != "" && !isWebhookDeliveryStatus(query.Status) {
		return WebhookDeliveryPage{}, errors.New("webhook delivery status is invalid")
	}
	if query.From != nil && query.To != nil && query.From.After(*query.To) {
		return WebhookDeliveryPage{}, errors.New("webhook delivery date range is invalid")
	}
	page, err := s.repository.ListDeliveries(ctx, ownerID, query)
	if err != nil {
		return WebhookDeliveryPage{}, fmt.Errorf("listing webhook deliveries: %w", err)
	}
	if page.Deliveries == nil {
		page.Deliveries = []WebhookDeliveryView{}
	}
	return page, nil
}

func (s *WebhookControlUsecase) EnqueueTest(ctx context.Context, ownerID, endpointID string) (WebhookQueuedEvent, error) {
	if ownerID == "" || endpointID == "" {
		return WebhookQueuedEvent{}, ErrWebhookEndpointNotFound
	}
	return s.repository.EnqueueTest(ctx, ownerID, endpointID, s.now().UTC())
}

func (s *WebhookControlUsecase) ReplayDelivery(ctx context.Context, ownerID, deliveryID string) error {
	if ownerID == "" || deliveryID == "" {
		return ErrWebhookDeliveryNotFound
	}
	return s.repository.ReplayDelivery(ctx, ownerID, deliveryID, s.now().UTC())
}

func isWebhookDeliveryStatus(status string) bool {
	switch status {
	case WebhookDeliveryDelivered, WebhookDeliveryRetryScheduled, WebhookDeliveryFailed,
		WebhookDeliveryExhausted, "in_flight", "superseded":
		return true
	default:
		return false
	}
}
