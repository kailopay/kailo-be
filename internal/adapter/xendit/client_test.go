package xendit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestVerifyCallbackRejectsWrongTokenBeforeAcceptingPayload(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment.capture","data":{"payment_id":"py-1","payment_request_id":"pr-1"}}`)
	if _, err := client.VerifyCallback(payload, "wrong"); !errors.Is(err, usecase.ErrInvalidCallback) {
		t.Fatalf("VerifyCallback(wrong) error = %v", err)
	}
	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil || callback.EventID != "py-1" {
		t.Fatalf("callback = %+v, error = %v", callback, err)
	}
}

func TestCreateCheckoutCreatesRestrictedHostedPaymentSession(t *testing.T) {
	fixture, err := os.ReadFile("testdata/qris-created.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "xnd_development_secret" || password != "" {
			t.Errorf("basic auth = %q/%q/%v", username, password, ok)
		}
		if r.URL.Path != "/sessions" || r.Header.Get("api-version") != "2024-11-11" {
			t.Errorf("request = %s, version %q", r.URL.Path, r.Header.Get("api-version"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["reference_id"] != "order-1" || body["session_type"] != "PAY" || body["mode"] != "PAYMENT_LINK" || body["amount"] != float64(100000) {
			t.Errorf("request body = %#v", body)
		}
		properties, ok := body["channel_properties"].(map[string]any)
		if !ok || properties["allowed_payment_channels"].([]any)[0] != "QRIS" {
			t.Errorf("channel properties = %#v", body["channel_properties"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, SecretKey: "xnd_development_secret", CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11", QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2026, 8, 20, 1, 5, 0, 0, time.UTC)
	checkout, err := client.CreateCheckout(context.Background(), usecase.CheckoutInput{OrderID: "order-1", Method: entity.PaymentMethodQRIS, Amount: 100_000, ExpiresAt: expires})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if checkout.ProviderID == "" || checkout.PresentationType != "PAYMENT_LINK" || checkout.PresentationValue == "" {
		t.Fatalf("checkout = %+v", checkout)
	}
}

func TestCreateCheckoutCreatesHostedPaymentSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" {
			t.Errorf("request path = %s, want /sessions", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["reference_id"] != "order-1" || body["session_type"] != "PAY" || body["mode"] != "PAYMENT_LINK" ||
			body["currency"] != "IDR" || body["country"] != "ID" || body["amount"] != float64(100000) {
			t.Errorf("request body = %#v", body)
		}
		if _, ok := body["allowed_payment_channels"]; ok {
			t.Errorf("hosted checkout should leave payment channels unrestricted: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"payment_session_id":"ps-1","payment_request_id":"pr-1","reference_id":"order-1","currency":"IDR","amount":100000,"status":"ACTIVE","expires_at":"2026-08-20T01:05:00Z","payment_link_url":"https://checkout-staging.xendit.co/sessions/ps-1"}`)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, SecretKey: "xnd_development_secret", CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11", QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	checkout, err := client.CreateCheckout(context.Background(), usecase.CheckoutInput{
		OrderID: "order-1", Method: entity.PaymentMethod("xendit"), Amount: 100_000,
		ExpiresAt: time.Date(2026, 8, 20, 1, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if checkout.ProviderID != "ps-1" || checkout.PaymentRequestID != "pr-1" || checkout.PresentationType != "PAYMENT_LINK" ||
		checkout.PresentationValue != "https://checkout-staging.xendit.co/sessions/ps-1" {
		t.Fatalf("checkout = %+v", checkout)
	}
}

func TestCreateCheckoutRejectsNonHTTPSPaymentLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"payment_session_id":"ps-1","reference_id":"order-1","currency":"IDR","amount":100000,"status":"ACTIVE","payment_link_url":"http://checkout.example/sessions/ps-1"}`)
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, SecretKey: "xnd_development_secret", CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11", QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.CreateCheckout(context.Background(), usecase.CheckoutInput{
		OrderID: "order-1", Method: entity.PaymentMethodXendit, Amount: 100_000,
		ExpiresAt: time.Date(2026, 8, 20, 1, 5, 0, 0, time.UTC),
	})
	var gatewayErr *usecase.GatewayError
	if !errors.As(err, &gatewayErr) || !gatewayErr.Unknown {
		t.Fatalf("CreateCheckout() error = %v, want unknown gateway error", err)
	}
}

func TestGetPaymentStateReconcilesHostedPaymentSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sessions/ps-1":
			_, _ = io.WriteString(w, `{"payment_session_id":"ps-1","reference_id":"order-1","currency":"IDR","amount":100000,"status":"COMPLETED","payment_request_id":"pr-1"}`)
		case "/v3/payment_requests/pr-1":
			_, _ = io.WriteString(w, `{"payment_request_id":"pr-1","reference_id":"order-1","currency":"IDR","request_amount":100000,"channel_code":"QRIS","status":"SUCCEEDED"}`)
		default:
			t.Errorf("unexpected request path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, SecretKey: "xnd_development_secret", CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11", QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	state, err := client.GetPaymentState(context.Background(), "ps-1")
	if err != nil {
		t.Fatalf("GetPaymentState() error = %v", err)
	}
	if state.ProviderID != "ps-1" || state.ReferenceID != "order-1" || state.Status != "COMPLETED" ||
		state.Currency != "IDR" || state.Amount != 100_000 || state.Channel != "QRIS" {
		t.Fatalf("state = %+v", state)
	}
}

func TestVerifyCallbackAcceptsPaymentSessionCompleted(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment_session.completed","data":{"payment_session_id":"ps-1","payment_request_id":"pr-1"}}`)
	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if callback.EventID != "payment_session.completed:ps-1" || callback.CheckoutID != "ps-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyCallbackAcceptsDashboardPaymentSessionID(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment_session.completed","data":{"id":"ps-dashboard-1","amount":100000}}`)

	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if callback.EventID != "payment_session.completed:ps-dashboard-1" || callback.CheckoutID != "ps-dashboard-1" {
		t.Fatalf("callback = %+v", callback)
	}
}

func TestVerifyCallbackUsesPaymentSessionWhenPresent(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment.capture","data":{"payment_id":"py-1","payment_session_id":"ps-1"}}`)
	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if callback.CheckoutID != "ps-1" {
		t.Fatalf("callback checkout ID = %q, want ps-1", callback.CheckoutID)
	}
}

func TestVerifyCallbackFallsBackToPaymentRequestID(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment.capture","data":{"payment_id":"py-1","payment_request_id":"pr-1"}}`)
	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if callback.CheckoutID != "pr-1" {
		t.Fatalf("callback checkout ID = %q, want pr-1", callback.CheckoutID)
	}
}
