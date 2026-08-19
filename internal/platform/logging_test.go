package platform

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewJSONLoggerWritesStructuredFields(t *testing.T) {
	var output bytes.Buffer

	logger, err := NewLogger("info", "json", &output)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("order created", slog.String("order_id", "ord_test"))

	text := output.String()
	if !strings.Contains(text, `"msg":"order created"`) {
		t.Fatalf("log output = %q, missing message", text)
	}
	if !strings.Contains(text, `"order_id":"ord_test"`) {
		t.Fatalf("log output = %q, missing order_id", text)
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	if _, err := NewLogger("verbose", "json", &bytes.Buffer{}); err == nil {
		t.Fatal("New() error = nil, want unknown level error")
	}
}

func TestNewLoggerRedactsSensitiveAttributes(t *testing.T) {
	var output bytes.Buffer
	logger, err := NewLogger("info", "json", &output)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	logger.Info("credential event",
		slog.String("password", "do-not-log"),
		slog.String("access_token", "token-do-not-log"),
		slog.String("order_id", "ord_test"),
	)

	text := output.String()
	if strings.Contains(text, "do-not-log") || strings.Contains(text, "token-do-not-log") {
		t.Fatalf("log output contains sensitive value: %q", text)
	}
	if !strings.Contains(text, `"password":"[REDACTED]"`) ||
		!strings.Contains(text, `"access_token":"[REDACTED]"`) {
		t.Fatalf("log output does not contain redacted fields: %q", text)
	}
	if !strings.Contains(text, `"order_id":"ord_test"`) {
		t.Fatalf("log output lost non-sensitive field: %q", text)
	}
}

func TestWithMetadataAddsDeploymentFields(t *testing.T) {
	var output bytes.Buffer
	base, err := NewLogger("info", "json", &output)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	WithMetadata(base, "kailopay", "api", "local", "dev").Info("started")
	text := output.String()
	for _, field := range []string{`"service":"kailopay"`, `"process":"api"`, `"environment":"local"`, `"release_version":"dev"`} {
		if !strings.Contains(text, field) {
			t.Fatalf("log output = %q, missing %s", text, field)
		}
	}
}
