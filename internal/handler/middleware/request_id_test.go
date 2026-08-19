package middleware

import "testing"

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid", input: "req-123_abc", want: "req-123_abc"},
		{name: "trims whitespace", input: "  req-123  ", want: "req-123"},
		{name: "rejects newline", input: "req-123\nforged", want: ""},
		{name: "rejects oversized", input: string(make([]byte, maxRequestIDLength+1)), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRequestID(tt.input); got != tt.want {
				t.Fatalf("normalizeRequestID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
