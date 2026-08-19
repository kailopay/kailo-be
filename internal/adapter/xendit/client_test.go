package xendit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/service/onramp"
)

func TestVerifyCallbackRejectsWrongTokenBeforeAcceptingPayload(t *testing.T) {
	client, err := New(Config{BaseURL: "https://api.xendit.co", SecretKey: "xnd_development_secret",
		CallbackToken: "01234567890123456789012345678901", APIVersion: "2024-11-11",
		QRISChannel: "QRIS", VAChannel: "BRI_VIRTUAL_ACCOUNT", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"event":"payment.capture","data":{"payment_id":"py-1","payment_request_id":"pr-1"}}`)
	if _, err := client.VerifyCallback(payload, "wrong"); !errors.Is(err, onramp.ErrInvalidCallback) {
		t.Fatalf("VerifyCallback(wrong) error = %v", err)
	}
	callback, err := client.VerifyCallback(payload, "01234567890123456789012345678901")
	if err != nil || callback.EventID != "py-1" {
		t.Fatalf("callback = %+v, error = %v", callback, err)
	}
}

func TestCreateCheckoutMapsQRISPaymentRequest(t *testing.T) {
	fixture, err := os.ReadFile("testdata/qris-created.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "xnd_development_secret" || password != "" {
			t.Errorf("basic auth = %q/%q/%v", username, password, ok)
		}
		if r.URL.Path != "/v3/payment_requests" || r.Header.Get("api-version") != "2024-11-11" {
			t.Errorf("request = %s, version %q", r.URL.Path, r.Header.Get("api-version"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["reference_id"] != "order-1" || body["channel_code"] != "QRIS" || body["request_amount"] != float64(100000) {
			t.Errorf("request body = %#v", body)
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
	checkout, err := client.CreateCheckout(context.Background(), onramp.CheckoutInput{OrderID: "order-1", Method: entity.PaymentMethodQRIS, Amount: 100_000, ExpiresAt: expires})
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if checkout.ProviderID == "" || checkout.PresentationType != "QR_STRING" || checkout.PresentationValue == "" {
		t.Fatalf("checkout = %+v", checkout)
	}
}
