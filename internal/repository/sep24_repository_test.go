package repository

import (
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestSEP24OrderDirection(t *testing.T) {
	tests := []struct {
		kind      string
		direction string
	}{
		{kind: usecase.Sep24KindDeposit, direction: "onramp"},
		{kind: usecase.Sep24KindWithdraw, direction: "offramp"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := sep24OrderDirection(tt.kind); got != tt.direction {
				t.Fatalf("sep24OrderDirection(%q) = %q, want %q", tt.kind, got, tt.direction)
			}
		})
	}

	if got := sep24OrderDirection("unknown"); got != "" {
		t.Fatalf("sep24OrderDirection(unknown) = %q, want empty", got)
	}
}
