package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// WebhookAPIVersion is stamped on every public event envelope.
const WebhookAPIVersion = "2026-08-01"

// Public webhook event catalog (WEBHOOK-DESIGN §2). Only order-lifecycle
// events exist in v0.1.0.
const (
	EventOrderCreated          = "order.created"
	EventOrderPaymentPending   = "order.payment_pending"
	EventOrderPaymentConfirmed = "order.payment_confirmed"
	EventOrderAssetReceived    = "order.asset_received"
	EventOrderProcessing       = "order.processing"
	EventOrderCompleted        = "order.completed"
	EventOrderFailed           = "order.failed"
	EventOrderExpired          = "order.expired"
)

// ErrInvalidWebhookURL rejects endpoints that fail the activation policy.
var ErrInvalidWebhookURL = errors.New("webhook URL must be https with a public host")

// WebhookEndpointRepository is the persistence port for developer endpoint
// management.
type WebhookEndpointRepository interface {
	CreateEndpoint(ctx context.Context, record WebhookEndpointRecord, secretHash []byte) (string, error)
	ListEndpoints(ctx context.Context, clientID string) ([]WebhookEndpointView, error)
	DisableEndpoint(ctx context.Context, clientID, endpointID string) error
}

// WebhookEndpointRecord is a new registration.
type WebhookEndpointRecord struct {
	ClientID   string
	URL        string
	EventTypes []string
}

// WebhookEndpointView is safe metadata; the signing secret never round-trips.
type WebhookEndpointView struct {
	ID         string     `json:"id"`
	URL        string     `json:"url"`
	Status     string     `json:"status"`
	EventTypes []string   `json:"event_types"`
	CreatedAt  time.Time  `json:"created_at"`
	DisabledAt *time.Time `json:"disabled_at,omitempty"`
}

// WebhookUsecase manages developer webhook endpoints.
type WebhookUsecase struct {
	repository WebhookEndpointRepository
	newID      func() (string, error)
	now        func() time.Time
}

func NewWebhookUsecase(repository WebhookEndpointRepository, newID func() (string, error), now func() time.Time) (*WebhookUsecase, error) {
	if repository == nil || newID == nil || now == nil {
		return nil, errors.New("valid webhook dependencies are required")
	}
	return &WebhookUsecase{repository: repository, newID: newID, now: now}, nil
}

// Register validates the URL against the activation policy, stores the
// endpoint, and returns its ID with the one-time plaintext signing secret.
func (s *WebhookUsecase) Register(ctx context.Context, clientID, rawURL string, eventTypes []string) (string, string, error) {
	if clientID == "" || len(rawURL) > 2048 || !isDeliverableWebhookURL(rawURL) {
		return "", "", ErrInvalidWebhookURL
	}
	if len(eventTypes) == 0 {
		eventTypes = []string{
			EventOrderCreated, EventOrderPaymentPending, EventOrderPaymentConfirmed,
			EventOrderAssetReceived, EventOrderProcessing, EventOrderCompleted,
			EventOrderFailed, EventOrderExpired,
		}
	}
	for _, eventType := range eventTypes {
		if !isKnownEventType(eventType) {
			return "", "", fmt.Errorf("unknown event type %q", eventType)
		}
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", fmt.Errorf("generating signing secret: %w", err)
	}
	secret := "whsec_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	id, err := s.newID()
	if err != nil {
		return "", "", fmt.Errorf("generating endpoint id: %w", err)
	}
	storedID, err := s.repository.CreateEndpoint(ctx, WebhookEndpointRecord{
		ClientID: clientID, URL: rawURL, EventTypes: eventTypes}, hashValue([]byte("kailopay-webhooks"), secret))
	if err != nil {
		return "", "", err
	}
	_ = id
	return storedID, secret, nil
}

func (s *WebhookUsecase) List(ctx context.Context, clientID string) ([]WebhookEndpointView, error) {
	return s.repository.ListEndpoints(ctx, clientID)
}

func (s *WebhookUsecase) Disable(ctx context.Context, clientID, endpointID string) error {
	return s.repository.DisableEndpoint(ctx, clientID, endpointID)
}

// isDeliverableWebhookURL enforces the Week 2 activation policy: HTTPS with
// host and no embedded credentials. Full SSRF re-resolution at delivery time
// is Week 3 (BE-061).
func isDeliverableWebhookURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		strings.ContainsAny(parsed.Host, " \t") {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" ||
		strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") || host == "metadata.google.internal" {
		return false
	}
	if strings.HasPrefix(host, "172.") {
		parts := splitHostOctets(host)
		if len(parts) >= 2 {
			second := atoiSafe(parts[1])
			if second >= 16 && second <= 31 {
				return false
			}
		}
	}
	return true
}

func splitHostOctets(host string) []string {
	return strings.Split(host, ".")
}

func atoiSafe(value string) int {
	result := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return -1
		}
		result = result*10 + int(character-'0')
	}
	return result
}

func isKnownEventType(value string) bool {
	switch value {
	case EventOrderCreated, EventOrderPaymentPending, EventOrderPaymentConfirmed,
		EventOrderAssetReceived, EventOrderProcessing, EventOrderCompleted,
		EventOrderFailed, EventOrderExpired:
		return true
	default:
		return false
	}
}

// BuildEventEnvelope renders the canonical public event payload that will be
// stored transactionally with an order transition and delivered by the
// Week 3 worker.
func BuildEventEnvelope(eventID, eventType, environment string, createdAt time.Time, order OrderView) ([]byte, error) {
	envelope := map[string]any{
		"id":          "evt_" + eventID,
		"object":      "event",
		"type":        eventType,
		"api_version": WebhookAPIVersion,
		"created_at":  createdAt.UTC().Format(time.RFC3339),
		"environment": environment,
		"data": map[string]any{
			"object": order,
		},
	}
	return json.Marshal(envelope)
}
