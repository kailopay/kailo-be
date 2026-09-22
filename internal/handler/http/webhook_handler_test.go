package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type webhookServiceSpy struct{}

func (webhookServiceSpy) Register(context.Context, string, string, []string) (string, string, error) {
	return "endpoint-1", "whsec_test", nil
}

func (webhookServiceSpy) List(context.Context, string) ([]usecase.WebhookEndpointView, error) {
	return []usecase.WebhookEndpointView{}, nil
}

func (webhookServiceSpy) Disable(context.Context, string, string) error { return nil }

type webhookControlServiceSpy struct {
	ownerID      string
	endpointID   string
	deliveryID   string
	query        usecase.WebhookDeliveryQuery
	queued       usecase.WebhookQueuedEvent
	deliveries   usecase.WebhookDeliveryPage
	endpointView usecase.WebhookEndpointView
}

func (s *webhookControlServiceSpy) GetEndpoint(_ context.Context, ownerID, endpointID string) (usecase.WebhookEndpointView, error) {
	s.ownerID, s.endpointID = ownerID, endpointID
	return s.endpointView, nil
}

func (s *webhookControlServiceSpy) ListDeliveries(_ context.Context, ownerID string, query usecase.WebhookDeliveryQuery) (usecase.WebhookDeliveryPage, error) {
	s.ownerID, s.query = ownerID, query
	return s.deliveries, nil
}

func (s *webhookControlServiceSpy) EnqueueTest(_ context.Context, ownerID, endpointID string) (usecase.WebhookQueuedEvent, error) {
	s.ownerID, s.endpointID = ownerID, endpointID
	return s.queued, nil
}

func (s *webhookControlServiceSpy) ReplayDelivery(_ context.Context, ownerID, deliveryID string) error {
	s.ownerID, s.deliveryID = ownerID, deliveryID
	return nil
}

func TestWebhookHandlerDeliveryControlsUseSessionOwnerAndFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	control := &webhookControlServiceSpy{deliveries: usecase.WebhookDeliveryPage{Deliveries: []usecase.WebhookDeliveryView{}}}
	handler := NewWebhookHandler(webhookServiceSpy{}, nil)
	handler.ConfigureControl(control)
	router := gin.New()
	router.GET("/v1/webhook-deliveries", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Deliveries)

	request := httptest.NewRequest(http.MethodGet, "/v1/webhook-deliveries?endpoint_id=endpoint-1&event_id=event-1&status=exhausted&limit=5&from=2026-09-01T00:00:00Z&to=2026-09-22T00:00:00Z", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || control.ownerID != "user-1" {
		t.Fatalf("status/owner = %d/%q, body = %q", response.Code, control.ownerID, response.Body.String())
	}
	if control.query.EndpointID != "endpoint-1" || control.query.EventID != "event-1" || control.query.Status != usecase.WebhookDeliveryExhausted || control.query.Limit != 5 || control.query.From == nil || control.query.To == nil {
		t.Fatalf("delivery query = %#v", control.query)
	}
	if response.Body.String() == "" || response.Body.String() == `{"deliveries":null}` {
		t.Fatalf("delivery response must initialize empty arrays: %q", response.Body.String())
	}
}

func TestWebhookHandlerQueuesTestAndReplayForAuthenticatedOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	control := &webhookControlServiceSpy{queued: usecase.WebhookQueuedEvent{EventID: "event-1", OutboxID: "outbox-1"}}
	handler := NewWebhookHandler(webhookServiceSpy{}, nil)
	handler.ConfigureControl(control)
	authenticate := middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}})
	router := gin.New()
	router.POST("/v1/webhook-endpoints/:id/test", authenticate, handler.Test)
	router.POST("/v1/webhook-deliveries/:id/replay", authenticate, handler.Replay)

	testRequest := httptest.NewRequest(http.MethodPost, "/v1/webhook-endpoints/endpoint-1/test", nil)
	testRequest.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	testResponse := httptest.NewRecorder()
	router.ServeHTTP(testResponse, testRequest)
	if testResponse.Code != http.StatusAccepted || control.ownerID != "user-1" || control.endpointID != "endpoint-1" {
		t.Fatalf("test delivery status/owner/endpoint = %d/%q/%q, body = %q", testResponse.Code, control.ownerID, control.endpointID, testResponse.Body.String())
	}

	replayRequest := httptest.NewRequest(http.MethodPost, "/v1/webhook-deliveries/delivery-1/replay", nil)
	replayRequest.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusAccepted || control.ownerID != "user-1" || control.deliveryID != "delivery-1" {
		t.Fatalf("replay status/owner/delivery = %d/%q/%q, body = %q", replayResponse.Code, control.ownerID, control.deliveryID, replayResponse.Body.String())
	}
}

func TestWebhookHandlerControlResponseUsesUTCMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	control := &webhookControlServiceSpy{endpointView: usecase.WebhookEndpointView{ID: "endpoint-1", CreatedAt: createdAt}}
	handler := NewWebhookHandler(webhookServiceSpy{}, nil)
	handler.ConfigureControl(control)
	router := gin.New()
	router.GET("/v1/webhook-endpoints/:id", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Get)

	request := httptest.NewRequest(http.MethodGet, "/v1/webhook-endpoints/endpoint-1", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || control.endpointID != "endpoint-1" || control.ownerID != "user-1" {
		t.Fatalf("status/owner/endpoint = %d/%q/%q, body = %q", response.Code, control.ownerID, control.endpointID, response.Body.String())
	}
}
