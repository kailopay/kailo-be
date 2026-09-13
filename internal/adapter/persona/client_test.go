package persona

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testPersonaSecret = "persona-webhook-secret-012345678901"

func TestCreateInquiryUsesPersonaJSONAPIAndIdempotency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/inquiries" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer persona-api-key" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Idempotency-Key"); got != "kyc_req_1" {
			t.Errorf("idempotency key = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		text := string(body)
		if !strings.Contains(text, `"inquiry-template-id":"itmpl_1"`) || !strings.Contains(text, `"reference-id":"user-1"`) {
			t.Errorf("request body = %s", text)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"type":"inquiry","id":"inq_1","attributes":{"status":"created","expires-at":"2026-09-14T00:00:00Z"}}}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	inquiry, err := client.CreateInquiry(context.Background(), "user-1", "kyc_req_1")
	if err != nil {
		t.Fatalf("CreateInquiry() error = %v", err)
	}
	if inquiry.ID != "inq_1" || inquiry.Status != "created" || inquiry.ExpiresAt == nil {
		t.Fatalf("inquiry = %+v", inquiry)
	}
}

func TestResumeInquiryReturnsSessionToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/inquiries/inq_1/resume" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{"session-token":"session_1"}}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	token, err := client.ResumeInquiry(context.Background(), "inq_1")
	if err != nil || token != "session_1" {
		t.Fatalf("ResumeInquiry() = %q, %v", token, err)
	}
}

func TestPersonaClientRejectsMalformedOversizedAndHTTPErrorResponsesWithoutSecrets(t *testing.T) {
	cases := []struct {
		name     string
		response string
		status   int
		maxBytes int64
	}{
		{name: "malformed json", response: "not-json", status: http.StatusOK, maxBytes: 1024},
		{name: "oversized body", response: strings.Repeat("x", 65), status: http.StatusOK, maxBytes: 64},
		{name: "provider error", response: "provider-secret-response", status: http.StatusBadRequest, maxBytes: 1024},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = io.WriteString(w, testCase.response)
			}))
			defer server.Close()
			client := newTestClientWithLimit(t, server.URL, testCase.maxBytes)
			_, err := client.CreateInquiry(context.Background(), "user-1", "req-1")
			if err == nil {
				t.Fatal("CreateInquiry() error = nil")
			}
			if strings.Contains(err.Error(), testCase.response) || strings.Contains(err.Error(), "persona-api-key") {
				t.Fatalf("error leaks provider data: %v", err)
			}
		})
	}
}

func TestPersonaClientHonorsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	client := newTestClientWithTimeout(t, server.URL, 20*time.Millisecond)

	_, err := client.CreateInquiry(context.Background(), "user-1", "req-1")
	if err == nil {
		t.Fatal("CreateInquiry() error = nil")
	}
}

func TestVerifyWebhookChecksRawBodySignatureAndParsesMinimalEvent(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	rawBody := []byte(`{"data":{"type":"event","id":"evt_1","attributes":{"name":"inquiry.approved","created-at":"2026-09-13T00:00:00Z","payload":{"data":{"type":"inquiry","id":"inq_1","attributes":{"status":"approved"}}}}}}`)
	signature := personaSignature(testPersonaSecret, now.Unix(), rawBody)
	client := newTestClient(t, "https://api.withpersona.com")

	event, err := client.VerifyWebhook(rawBody, signature, now)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.ID != "evt_1" || event.Type != "inquiry.approved" || event.InquiryID != "inq_1" || event.Status != "approved" || !event.CreatedAt.Equal(now) {
		t.Fatalf("event = %+v", event)
	}
}

func TestVerifyWebhookAcceptsRotatingSignaturesAndRejectsInvalidHeaders(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	rawBody := []byte(`{"data":{"id":"evt_1","type":"event","attributes":{"name":"inquiry.started","created-at":"2026-09-13T00:00:00Z","payload":{"data":{"type":"inquiry","id":"inq_1","attributes":{"status":"started"}}}}}}`)
	valid := personaSignature(testPersonaSecret, now.Unix(), rawBody)
	client := newTestClient(t, "https://api.withpersona.com")

	rotated := "t=" + strings.Split(strings.Split(valid, ",")[0], "=")[1] + ",v1=" + strings.Repeat("0", 64) + " " + valid
	if _, err := client.VerifyWebhook(rawBody, rotated, now); err != nil {
		t.Fatalf("VerifyWebhook(rotating) error = %v", err)
	}
	for _, invalid := range []string{
		"",
		"t=not-a-timestamp,v1=abc",
		"t=" + strconv.FormatInt(now.Add(-time.Hour).Unix(), 10) + ",v1=" + strings.Repeat("0", 64),
		"t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=" + strings.Repeat("0", 64),
	} {
		if _, err := client.VerifyWebhook(rawBody, invalid, now); err == nil {
			t.Errorf("VerifyWebhook(%q) error = nil", invalid)
		}
	}
	for _, extreme := range []int64{int64(-1 << 63), int64(1<<63 - 1)} {
		if _, err := client.VerifyWebhook(rawBody, personaSignature(testPersonaSecret, extreme, rawBody), now); err == nil {
			t.Errorf("VerifyWebhook(extreme timestamp %d) error = nil", extreme)
		}
	}
}

func TestVerifyWebhookRejectsEmptyBodyAndMalformedPayload(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	client := newTestClient(t, "https://api.withpersona.com")
	for _, rawBody := range [][]byte{nil, []byte("{}")} {
		signature := personaSignature(testPersonaSecret, now.Unix(), rawBody)
		if _, err := client.VerifyWebhook(rawBody, signature, now); err == nil {
			t.Errorf("VerifyWebhook(%q) error = nil", rawBody)
		}
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	return newTestClientWithConfig(t, Config{BaseURL: baseURL, APIKey: "persona-api-key", TemplateID: "itmpl_1", WebhookSecret: testPersonaSecret,
		Timeout: 2 * time.Second, SignatureTolerance: 5 * time.Minute, MaxResponseBytes: 1 << 20, HTTPClient: http.DefaultClient})
}

func newTestClientWithLimit(t *testing.T, baseURL string, maxBytes int64) *Client {
	return newTestClientWithConfig(t, Config{BaseURL: baseURL, APIKey: "persona-api-key", TemplateID: "itmpl_1", WebhookSecret: testPersonaSecret,
		Timeout: 2 * time.Second, SignatureTolerance: 5 * time.Minute, MaxResponseBytes: maxBytes, HTTPClient: http.DefaultClient})
}

func newTestClientWithTimeout(t *testing.T, baseURL string, timeout time.Duration) *Client {
	return newTestClientWithConfig(t, Config{BaseURL: baseURL, APIKey: "persona-api-key", TemplateID: "itmpl_1", WebhookSecret: testPersonaSecret,
		Timeout: timeout, SignatureTolerance: 5 * time.Minute, MaxResponseBytes: 1 << 20, HTTPClient: http.DefaultClient})
}

func newTestClientWithConfig(t *testing.T, config Config) *Client {
	t.Helper()
	client, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func personaSignature(secret string, timestamp int64, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join([]string{stringTimestamp(timestamp), string(rawBody)}, ".")))
	return "t=" + stringTimestamp(timestamp) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func stringTimestamp(value int64) string {
	return strconv.FormatInt(value, 10)
}
