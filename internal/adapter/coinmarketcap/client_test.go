package coinmarketcap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestClientReadsAndValidatesXLMIDRQuote(t *testing.T) {
	fixture, err := os.ReadFile("testdata/xlm-idr.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	tests := []struct {
		name           string
		statusCode     int
		body           string
		wantPrice      string
		wantObservedAt time.Time
		wantErr        bool
	}{
		{
			name:           "string error code 0 succeeds",
			body:           string(fixture),
			wantPrice:      "3113.974238579513",
			wantObservedAt: time.Date(2026, time.September, 1, 18, 59, 5, 0, time.UTC),
		},
		{
			name:           "numeric error code 0 succeeds",
			body:           `{"status":{"error_code":0},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantPrice:      "3113.974238579513",
			wantObservedAt: time.Date(2026, time.September, 1, 18, 59, 5, 0, time.UTC),
		},
		{
			name:       "non-2xx provider response",
			statusCode: http.StatusTooManyRequests,
			body:       `{"status":{"error_code":1008}}`,
			wantErr:    true,
		},
		{
			name:    "numeric non-zero error code fails",
			body:    `{"status":{"error_code":1008},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "string non-zero error code fails",
			body:    `{"status":{"error_code":"1008"},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "malformed error code fails",
			body:    `{"status":{"error_code":"not-an-integer"},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "missing error code fails",
			body:    `{"status":{},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "missing data",
			body:    `{"status":{"error_code":0}}`,
			wantErr: true,
		},
		{
			name:    "wrong asset ID",
			body:    `{"status":{"error_code":0},"data":[{"id":1,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "missing IDR quote",
			body:    `{"status":{"error_code":0},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2781,"symbol":"USD","price":0.123,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "empty price",
			body:    `{"status":{"error_code":0},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":null,"last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "invalid price",
			body:    `{"status":{"error_code":0},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":"not-a-number","last_updated":"2026-09-01T18:59:05Z"}]}]}`,
			wantErr: true,
		},
		{
			name:    "invalid last_updated timestamp",
			body:    `{"status":{"error_code":0},"data":[{"id":512,"symbol":"XLM","quote":[{"id":2794,"symbol":"IDR","price":3113.974238579513,"last_updated":"not-a-timestamp"}]}]}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			body:    `{"status":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCode := tt.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %s, want %s", r.Method, http.MethodGet)
				}
				if r.URL.Path != "/v3/cryptocurrency/quotes/latest" {
					t.Errorf("path = %s, want /v3/cryptocurrency/quotes/latest", r.URL.Path)
				}
				if got := r.URL.Query().Get("id"); got != "512" {
					t.Errorf("id query = %q, want 512", got)
				}
				if got := r.URL.Query().Get("convert"); got != "IDR" {
					t.Errorf("convert query = %q, want IDR", got)
				}
				if got := r.Header.Get("X-CMC_PRO_API_KEY"); got != "test-key" {
					t.Errorf("api key header = %q, want test-key", got)
				}

				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client, err := New(Config{
				BaseURL:    server.URL,
				APIKey:     "test-key",
				HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			got, err := client.LatestXLMIDR(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("LatestXLMIDR() error = nil, want error")
				}
				if !errors.Is(err, ErrUnavailable) {
					t.Fatalf("LatestXLMIDR() error = %v, want ErrUnavailable", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("LatestXLMIDR() error = %v", err)
			}
			if got.IDRPerXLM != tt.wantPrice {
				t.Errorf("IDRPerXLM = %q, want %q", got.IDRPerXLM, tt.wantPrice)
			}
			if !got.ObservedAt.Equal(tt.wantObservedAt) {
				t.Errorf("ObservedAt = %s, want %s", got.ObservedAt, tt.wantObservedAt)
			}
		})
	}
}
