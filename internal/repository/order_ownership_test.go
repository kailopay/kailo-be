package repository

import (
	"errors"
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestOrderOwnershipColumnsSeparateAPIAndRetailScopes(t *testing.T) {
	tests := []struct {
		name         string
		principal    usecase.OrderPrincipal
		clientID     string
		userID       string
		sessionID    string
		retailUserID string
	}{
		{
			name:      "api client",
			principal: usecase.OrderPrincipal{Kind: usecase.OrderPrincipalAPIClient, ClientID: "client-1", OwnerUserID: "owner-1"},
			clientID:  "client-1",
		},
		{
			name:         "retail session",
			principal:    usecase.OrderPrincipal{Kind: usecase.OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"},
			userID:       "user-1",
			sessionID:    "session-1",
			retailUserID: "user-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownership, err := orderOwnershipFor(tt.principal)
			if err != nil {
				t.Fatalf("orderOwnershipFor() error = %v", err)
			}
			if derefOptionalString(ownership.ClientID) != tt.clientID ||
				derefOptionalString(ownership.CreatedByUserID) != tt.userID ||
				derefOptionalString(ownership.RetailSessionID) != tt.sessionID ||
				derefOptionalString(ownership.RetailUserID) != tt.retailUserID {
				t.Fatalf("ownership = %+v", ownership)
			}
		})
	}
}

func TestOrderOwnershipRejectsInvalidPrincipal(t *testing.T) {
	if _, err := orderOwnershipFor(usecase.OrderPrincipal{}); !errors.Is(err, usecase.ErrInvalidOrderPrincipal) {
		t.Fatalf("orderOwnershipFor() error = %v, want %v", err, usecase.ErrInvalidOrderPrincipal)
	}
}

func derefOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
