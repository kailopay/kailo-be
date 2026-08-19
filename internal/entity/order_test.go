package entity

import (
	"errors"
	"testing"
)

func TestOrderTransitionsFollowOnrampStateMachine(t *testing.T) {
	order := Order{Status: OrderStatusCreated, Version: 1}

	for _, next := range []OrderStatus{
		OrderStatusPaymentPending,
		OrderStatusPaymentConfirmed,
		OrderStatusStellarProcessing,
		OrderStatusCompleted,
	} {
		if err := order.Transition(next); err != nil {
			t.Fatalf("Transition(%q) error = %v", next, err)
		}
	}

	if order.Version != 5 {
		t.Fatalf("version = %d, want 5", order.Version)
	}
}

func TestOrderRejectsIllegalTransition(t *testing.T) {
	order := Order{Status: OrderStatusCreated, Version: 1}

	err := order.Transition(OrderStatusCompleted)
	if !errors.Is(err, ErrInvalidOrderState) {
		t.Fatalf("Transition() error = %v, want ErrInvalidOrderState", err)
	}
	if order.Status != OrderStatusCreated || order.Version != 1 {
		t.Fatalf("order mutated to status=%q version=%d", order.Status, order.Version)
	}
}

func TestTerminalOrderCannotTransition(t *testing.T) {
	for _, status := range []OrderStatus{
		OrderStatusCompleted,
		OrderStatusExpired,
		OrderStatusPaymentFailed,
		OrderStatusStellarFailed,
		OrderStatusCancelled,
	} {
		order := Order{Status: status, Version: 3}
		if err := order.Transition(OrderStatusPaymentPending); !errors.Is(err, ErrInvalidOrderState) {
			t.Errorf("status %q: error = %v", status, err)
		}
	}
}
