package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeWebhookDNSResolver struct {
	ips []net.IP
	err error
}

func (r fakeWebhookDNSResolver) LookupIP(context.Context, string) ([]net.IP, error) {
	return r.ips, r.err
}

type fakeWebhookHTTPClient struct {
	request  *http.Request
	response *http.Response
	err      error
}

func (c *fakeWebhookHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.request = request
	return c.response, c.err
}

type fakeWebhookSecretProtector struct {
	secret string
	err    error
}

func (p fakeWebhookSecretProtector) Protect(string) ([]byte, error) { return nil, nil }

func (p fakeWebhookSecretProtector) Unprotect([]byte) (string, error) {
	return p.secret, p.err
}

type fakeWebhookDeliveryRepository struct {
	event       WebhookEventRecord
	endpoints   []WebhookDeliveryEndpoint
	attempt     WebhookDeliveryAttempt
	start       bool
	result      WebhookDeliveryResult
	completed   bool
	finalized   bool
	finalOutbox string
	finalEvent  string
}

func (r *fakeWebhookDeliveryRepository) LoadEvent(context.Context, string) (WebhookEventRecord, error) {
	return r.event, nil
}

func (r *fakeWebhookDeliveryRepository) ListDeliveryEndpoints(context.Context, string, string) ([]WebhookDeliveryEndpoint, error) {
	return r.endpoints, nil
}

func (r *fakeWebhookDeliveryRepository) StartAttempt(context.Context, string, string, string, time.Time, time.Duration, int) (WebhookDeliveryAttempt, bool, error) {
	return r.attempt, r.start, nil
}

func (r *fakeWebhookDeliveryRepository) CompleteAttempt(_ context.Context, _ string, result WebhookDeliveryResult) error {
	r.result = result
	r.completed = true
	return nil
}

func (r *fakeWebhookDeliveryRepository) FinalizeEvent(_ context.Context, outboxID, eventID string, _ time.Time) error {
	r.finalized = true
	r.finalOutbox = outboxID
	r.finalEvent = eventID
	return nil
}

func TestBuildWebhookSignatureUsesTimestampAndRawPayload(t *testing.T) {
	timestamp := time.Unix(1_700_000_000, 0).UTC()
	payload := []byte(`{"id":"evt_123","type":"order.created"}`)
	secret := []byte("webhook-secret")

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("1700000000."))
	_, _ = mac.Write(payload)
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))

	if got := BuildWebhookSignature("evt_123", timestamp, payload, secret); got != want {
		t.Fatalf("BuildWebhookSignature() = %q, want %q", got, want)
	}
}

func TestValidateWebhookDestinationResolvesAndRejectsPrivateAddresses(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		resolver WebhookDNSResolver
		wantErr  bool
	}{
		{name: "requires https", rawURL: "http://public.example/callback", resolver: fakeWebhookDNSResolver{ips: []net.IP{net.ParseIP("8.8.8.8")}}, wantErr: true},
		{name: "rejects private literal", rawURL: "https://127.0.0.1/callback", wantErr: true},
		{name: "rejects private DNS result", rawURL: "https://callback.example/callback", resolver: fakeWebhookDNSResolver{ips: []net.IP{net.ParseIP("10.0.0.8")}}, wantErr: true},
		{name: "rejects mixed DNS result", rawURL: "https://callback.example/callback", resolver: fakeWebhookDNSResolver{ips: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.4")}}, wantErr: true},
		{name: "accepts public DNS result", rawURL: "https://callback.example/callback", resolver: fakeWebhookDNSResolver{ips: []net.IP{net.ParseIP("8.8.8.8")}}},
		{name: "rejects resolver failure", rawURL: "https://callback.example/callback", resolver: fakeWebhookDNSResolver{err: errors.New("lookup failed")}, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateWebhookDestination(context.Background(), testCase.rawURL, testCase.resolver)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ValidateWebhookDestination() error = %v, wantErr %t", err, testCase.wantErr)
			}
		})
	}
}

func TestClassifyWebhookDeliveryResponse(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		networkErr error
		attempt    int
		max        int
		want       string
	}{
		{name: "success", status: http.StatusNoContent, attempt: 1, max: 5, want: WebhookDeliveryDelivered},
		{name: "bad request is terminal", status: http.StatusBadRequest, attempt: 1, max: 5, want: WebhookDeliveryFailed},
		{name: "conflict retries", status: http.StatusConflict, attempt: 1, max: 5, want: WebhookDeliveryRetryScheduled},
		{name: "too early retries", status: http.StatusTooEarly, attempt: 1, max: 5, want: WebhookDeliveryRetryScheduled},
		{name: "server error retries", status: http.StatusBadGateway, attempt: 1, max: 5, want: WebhookDeliveryRetryScheduled},
		{name: "last server error exhausts", status: http.StatusBadGateway, attempt: 5, max: 5, want: WebhookDeliveryExhausted},
		{name: "network error retries", networkErr: errors.New("timeout"), attempt: 1, max: 5, want: WebhookDeliveryRetryScheduled},
		{name: "last network error exhausts", networkErr: errors.New("timeout"), attempt: 5, max: 5, want: WebhookDeliveryExhausted},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			result := ClassifyWebhookDelivery(testCase.status, testCase.networkErr, testCase.attempt, testCase.max, time.Unix(1_700_000_000, 0).UTC(), time.Second)
			if result.Status != testCase.want {
				t.Fatalf("status = %q, want %q", result.Status, testCase.want)
			}
		})
	}
}

func TestWebhookDeliveryUsecaseSignsAndRecordsSuccessfulDelivery(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	repository := &fakeWebhookDeliveryRepository{
		event: WebhookEventRecord{ID: "event-1", EventType: EventOrderCreated, APIVersion: WebhookAPIVersion,
			Payload: []byte(`{"id":"evt_event-1","type":"order.created"}`)},
		endpoints: []WebhookDeliveryEndpoint{{ID: "endpoint-1", URL: "https://callback.example/webhook", SecretReference: []byte("reference")}},
		attempt:   WebhookDeliveryAttempt{ID: "attempt-1", EventID: "event-1", EndpointID: "endpoint-1", AttemptNumber: 1},
		start:     true,
	}
	client := &fakeWebhookHTTPClient{response: &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}}
	service, err := NewWebhookDeliveryUsecase(repository, fakeWebhookSecretProtector{secret: "whsec_test"}, WebhookDeliveryConfig{
		Timeout: 5 * time.Second, LeaseDuration: 30 * time.Second, RetryDelay: time.Second, MaxAttempts: 5,
		Now: func() time.Time { return now }, Resolver: fakeWebhookDNSResolver{ips: []net.IP{net.ParseIP("8.8.8.8")}}, Client: client,
	})
	if err != nil {
		t.Fatalf("NewWebhookDeliveryUsecase() error = %v", err)
	}

	if err := service.RunOnce(context.Background(), WebhookDeliveryJob{OutboxID: "outbox-1", EventID: "event-1", WorkerID: "worker-1"}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !repository.completed || repository.result.Status != WebhookDeliveryDelivered {
		t.Fatalf("completed = %t, result = %#v, want delivered", repository.completed, repository.result)
	}
	if !repository.finalized || repository.finalOutbox != "outbox-1" || repository.finalEvent != "event-1" {
		t.Fatalf("finalization = %#v/%#v/%t, want outbox-1/event-1/true", repository.finalOutbox, repository.finalEvent, repository.finalized)
	}
	if client.request == nil {
		t.Fatal("webhook request was not sent")
	}
	if got := client.request.Header.Get("KailoPay-Event-Id"); got != "evt_event-1" {
		t.Fatalf("KailoPay-Event-Id = %q, want evt_event-1", got)
	}
	if got := client.request.Header.Get("KailoPay-Signature"); !strings.HasPrefix(got, "v1=") {
		t.Fatalf("KailoPay-Signature = %q, want v1 signature", got)
	}
	if got := client.request.Header.Get("KailoPay-Event-Type"); got != EventOrderCreated {
		t.Fatalf("KailoPay-Event-Type = %q, want %q", got, EventOrderCreated)
	}
}

func TestWebhookSecretBoxDoesNotPersistPlaintext(t *testing.T) {
	box, err := NewWebhookSecretBox([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("NewWebhookSecretBox() error = %v", err)
	}
	protected, err := box.Protect("whsec_plaintext")
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if strings.Contains(string(protected), "whsec_plaintext") {
		t.Fatal("protected secret contains plaintext")
	}
	if got, err := box.Unprotect(protected); err != nil || got != "whsec_plaintext" {
		t.Fatalf("Unprotect() = %q, %v; want original secret", got, err)
	}
}
