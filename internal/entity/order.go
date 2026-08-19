package entity

import "errors"

var ErrInvalidOrderState = errors.New("invalid order state transition")

type OrderStatus string

const (
	OrderStatusUnknown           OrderStatus = ""
	OrderStatusCreated           OrderStatus = "created"
	OrderStatusPaymentPending    OrderStatus = "payment_pending"
	OrderStatusPaymentConfirmed  OrderStatus = "payment_confirmed"
	OrderStatusStellarProcessing OrderStatus = "stellar_processing"
	OrderStatusCompleted         OrderStatus = "completed"
	OrderStatusExpired           OrderStatus = "expired"
	OrderStatusPaymentFailed     OrderStatus = "payment_failed"
	OrderStatusStellarFailed     OrderStatus = "stellar_failed"
	OrderStatusCancelled         OrderStatus = "cancelled"
)

type PaymentMethod string

const (
	PaymentMethodUnknown      PaymentMethod = ""
	PaymentMethodQRIS         PaymentMethod = "qris"
	PaymentMethodBankTransfer PaymentMethod = "bank_transfer"
)

type Order struct {
	Status  OrderStatus
	Version int
}

func (order *Order) Transition(next OrderStatus) error {
	if !allowedTransition(order.Status, next) {
		return ErrInvalidOrderState
	}
	order.Status = next
	order.Version++
	return nil
}

func allowedTransition(current, next OrderStatus) bool {
	switch current {
	case OrderStatusCreated:
		return next == OrderStatusPaymentPending || next == OrderStatusPaymentFailed || next == OrderStatusCancelled
	case OrderStatusPaymentPending:
		return next == OrderStatusPaymentConfirmed || next == OrderStatusExpired ||
			next == OrderStatusPaymentFailed || next == OrderStatusCancelled
	case OrderStatusPaymentConfirmed:
		return next == OrderStatusStellarProcessing
	case OrderStatusStellarProcessing:
		return next == OrderStatusCompleted || next == OrderStatusStellarFailed
	default:
		return false
	}
}
