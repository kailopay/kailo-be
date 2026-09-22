package repository

import (
	"testing"
	"time"
)

func TestDeveloperCursorRoundTripsCreatedAtAndID(t *testing.T) {
	want := developerCursor{CreatedAt: time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC), ID: "order-1"}
	cursor, err := encodeDeveloperCursor(want)
	if err != nil {
		t.Fatalf("encodeDeveloperCursor() error = %v", err)
	}
	got, err := decodeDeveloperCursor(cursor)
	if err != nil {
		t.Fatalf("decodeDeveloperCursor() error = %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
}

func TestDeveloperCursorRejectsMalformedInput(t *testing.T) {
	for _, cursor := range []string{"", "not-base64", "Zm9v", "eyJpZCI6IiJ9"} {
		if _, err := decodeDeveloperCursor(cursor); err == nil {
			t.Errorf("decodeDeveloperCursor(%q) error = nil", cursor)
		}
	}
}

func TestAnalyticsBucketExpressionUsesAllowlist(t *testing.T) {
	for _, bucket := range []string{"hour", "day", "week"} {
		if expression, ok := analyticsBucketExpression(bucket); !ok || expression == "" {
			t.Errorf("analyticsBucketExpression(%q) = %q/%v, want allowed expression", bucket, expression, ok)
		}
	}
	if expression, ok := analyticsBucketExpression("day); DROP TABLE orders;--"); ok || expression != "" {
		t.Fatalf("analyticsBucketExpression() accepted unsafe input: %q/%v", expression, ok)
	}
}
