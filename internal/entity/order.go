package entity

import "errors"

var ErrInvalidOrderState = errors.New("invalid order state transition")

type OrderStatus string

const (
	OrderStatusUnknown              OrderStatus = ""
	OrderStatusCreated              OrderStatus = "created"
	OrderStatusPaymentPending       OrderStatus = "payment_pending"
	OrderStatusPaymentConfirmed     OrderStatus = "payment_confirmed"
	OrderStatusStellarProcessing    OrderStatus = "stellar_processing"
	OrderStatusCompleted            OrderStatus = "completed"
	OrderStatusExpired              OrderStatus = "expired"
	OrderStatusPaymentFailed        OrderStatus = "payment_failed"
	OrderStatusStellarFailed        OrderStatus = "stellar_failed"
	OrderStatusCancelled            OrderStatus = "cancelled"
	OrderStatusAssetPending         OrderStatus = "asset_pending"
	OrderStatusAssetReceived        OrderStatus = "asset_received"
	OrderStatusAssetInvalid         OrderStatus = "asset_invalid"
	OrderStatusRetirementProcessing OrderStatus = "retirement_processing"
	OrderStatusWithdrawalProcessing OrderStatus = "withdrawal_processing"
	OrderStatusRetirementFailed     OrderStatus = "retirement_failed"
	OrderStatusWithdrawalFailed     OrderStatus = "withdrawal_failed"
)

// WithdrawalMethod names the fiat side of an off-ramp order. The sandbox
// release supports only the simulated bank-transfer rail (ADR-003).
type WithdrawalMethod string

const (
	WithdrawalMethodUnknown         WithdrawalMethod = ""
	WithdrawalMethodSandboxTransfer WithdrawalMethod = "sandbox_bank_transfer"
)

type PaymentMethod string

const (
	PaymentMethodUnknown PaymentMethod = ""
	PaymentMethodQRIS    PaymentMethod = "qris"
	PaymentMethodBRIVA   PaymentMethod = "bri_va"
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
		return next == OrderStatusPaymentPending || next == OrderStatusAssetPending ||
			next == OrderStatusPaymentFailed || next == OrderStatusCancelled
	case OrderStatusPaymentPending:
		return next == OrderStatusPaymentConfirmed || next == OrderStatusExpired ||
			next == OrderStatusPaymentFailed || next == OrderStatusCancelled
	case OrderStatusPaymentConfirmed:
		return next == OrderStatusStellarProcessing
	case OrderStatusStellarProcessing:
		return next == OrderStatusCompleted || next == OrderStatusStellarFailed
	// Off-ramp lifecycle (ORDER-STATE-MACHINES §3).
	case OrderStatusAssetPending:
		return next == OrderStatusAssetReceived || next == OrderStatusExpired ||
			next == OrderStatusAssetInvalid || next == OrderStatusCancelled
	case OrderStatusAssetReceived:
		return next == OrderStatusRetirementProcessing
	case OrderStatusRetirementProcessing:
		return next == OrderStatusWithdrawalProcessing || next == OrderStatusRetirementFailed
	case OrderStatusWithdrawalProcessing:
		return next == OrderStatusCompleted || next == OrderStatusWithdrawalFailed
	default:
		return false
	}
}
