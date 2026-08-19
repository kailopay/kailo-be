package coinmarketcap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestClientReadsXLMIDRQuote(t *testing.T) {
	fixture, err := os.ReadFile("testdata/xlm-idr.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/cryptocurrency/quotes/latest" || r.URL.Query().Get("id") != "512" || r.URL.Query().Get("convert") != "IDR" {
			t.Errorf("request URL = %s", r.URL.String())
		}
		if r.Header.Get("X-CMC_PRO_API_KEY") != "test-key" {
			t.Errorf("API key header = %q", r.Header.Get("X-CMC_PRO_API_KEY"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	price, err := client.LatestXLMIDR(context.Background())
	if err != nil {
		t.Fatalf("LatestXLMIDR() error = %v", err)
	}
	if price.IDRPerXLM != "2500.125" || !price.ObservedAt.Equal(time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)) {
		t.Fatalf("price = %+v", price)
	}
}

func TestClientRejectsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":{"error_code":1008}}`))
	}))
	defer server.Close()
	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.LatestXLMIDR(context.Background()); err == nil {
		t.Fatal("LatestXLMIDR() error = nil")
	}
}
