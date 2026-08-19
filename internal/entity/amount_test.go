package entity

import "testing"

func TestStroopsString(t *testing.T) {
	tests := []struct {
		name  string
		value Stroops
		want  string
	}{
		{name: "one stroop", value: 1, want: "0.0000001"},
		{name: "one xlm", value: 10_000_000, want: "1.0000000"},
		{name: "fractional xlm", value: 123_456_789, want: "12.3456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAmountsRequirePositiveValues(t *testing.T) {
	if err := IDR(0).Validate(); err == nil {
		t.Fatal("IDR.Validate() error = nil")
	}
	if err := Stroops(-1).Validate(); err == nil {
		t.Fatal("Stroops.Validate() error = nil")
	}
}
