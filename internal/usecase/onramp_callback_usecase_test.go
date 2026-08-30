package usecase

import (
	"context"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
)

type callbackGatewayFake struct{}

func (callbackGatewayFake) VerifyCallback([]byte, string) (Callback, error) {
	return Callback{EventID: "py-1", EventType: "payment.capture", CheckoutID: "pr-1"}, nil
}
func (callbackGatewayFake) GetPaymentState(context.Context, string) (PaymentState, error) {
	return PaymentState{ProviderID: "pr-1", ReferenceID: "order-1", Status: "SUCCEEDED", Currency: "IDR", Amount: 100_000, Channel: "QRIS"}, nil
}

type callbackStoreFake struct{ recorded, confirmed int }

func (s *callbackStoreFake) FindReplay(context.Context, OrderPrincipal, string, string) (OrderView, bool, error) {
	return OrderView{}, false, nil
}
func (s *callbackStoreFake) ReserveAndCreate(context.Context, CreateRecord, entity.Stroops) error {
	return nil
}
func (s *callbackStoreFake) AttachCheckout(context.Context, string, Checkout) (OrderView, error) {
	return OrderView{}, nil
}
func (s *callbackStoreFake) FailCheckout(context.Context, string, string) error        { return nil }
func (s *callbackStoreFake) MarkCheckoutUnknown(context.Context, string, string) error { return nil }
func (s *callbackStoreFake) Get(context.Context, OrderPrincipal, string) (OrderView, error) {
	return OrderView{}, nil
}
func (s *callbackStoreFake) List(context.Context, OrderPrincipal, int, string) ([]OrderView, string, error) {
	return []OrderView{}, "", nil
}

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

type hostedCallbackGatewayFake struct{}

func (hostedCallbackGatewayFake) VerifyCallback([]byte, string) (Callback, error) {
	return Callback{EventID: "payment_session.completed:ps-1", EventType: "payment_session.completed", CheckoutID: "ps-1"}, nil
}

func (hostedCallbackGatewayFake) GetPaymentState(context.Context, string) (PaymentState, error) {
	return PaymentState{ProviderID: "ps-1", ReferenceID: "order-1", Status: "COMPLETED", Currency: "IDR", Amount: 100_000}, nil
}

type hostedCallbackStoreFake struct{ callbackStoreFake }

func (s *hostedCallbackStoreFake) ExpectedPayment(context.Context, string) (ExpectedPayment, error) {
	return ExpectedPayment{OrderID: "order-1", ProviderID: "ps-1", Amount: 100_000, Currency: "IDR", AssetAmount: entity.Stroops(400_000_000)}, nil
}

func TestCompletedPaymentSessionCreatesOneSettlementIntent(t *testing.T) {
	store := &hostedCallbackStoreFake{}
	service, err := NewOnrampCallbackUsecase(hostedCallbackGatewayFake{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Process(context.Background(), []byte(`{"event":"payment_session.completed"}`), "token"); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if store.confirmed != 1 {
		t.Fatalf("confirmed = %d, want 1", store.confirmed)
	}
}
