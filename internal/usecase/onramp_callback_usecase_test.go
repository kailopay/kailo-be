package usecase

import (
	"context"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
)

type callbackGatewayFake struct{}

func (callbackGatewayFake) VerifyCallback([]byte, string) (Callback, error) {
	return Callback{EventID: "py-1", EventType: "payment.capture", PaymentRequestID: "pr-1"}, nil
}
func (callbackGatewayFake) GetPaymentRequest(context.Context, string) (PaymentState, error) {
	return PaymentState{ProviderID: "pr-1", ReferenceID: "order-1", Status: "SUCCEEDED", Currency: "IDR", Amount: 100_000, Channel: "QRIS"}, nil
}

type callbackStoreFake struct{ recorded, confirmed int }

func (s *callbackStoreFake) RecordCallbackReceipt(context.Context, CallbackReceipt) (bool, error) {
	s.recorded++
	return s.recorded > 1, nil
}
func (s *callbackStoreFake) ExpectedPayment(context.Context, string) (ExpectedPayment, error) {
	return ExpectedPayment{OrderID: "order-1", ProviderID: "pr-1", Amount: 100_000, Currency: "IDR", Channel: "QRIS", AssetAmount: entity.Stroops(400_000_000)}, nil
}
func (s *callbackStoreFake) ConfirmPaymentAndEnqueue(context.Context, PaymentConfirmation) error {
	s.confirmed++
	return nil
}
func (s *callbackStoreFake) CompleteCallback(context.Context, string, string) error { return nil }

func TestPaidCallbackReplayCreatesOneSettlementIntent(t *testing.T) {
	store := &callbackStoreFake{}
	service, err := NewOnrampCallbackUsecase(callbackGatewayFake{}, store)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := service.Process(context.Background(), []byte(`{"event":"payment.capture"}`), "token"); err != nil {
			t.Fatal(err)
		}
	}
	if store.confirmed != 1 {
		t.Fatalf("confirmed = %d, want 1", store.confirmed)
	}
}
