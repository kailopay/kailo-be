package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type createStoreFake struct {
	calls    []string
	created  CreateRecord
	attached Checkout
}

func (s *createStoreFake) FindReplay(context.Context, string, string, string) (OrderView, bool, error) {
	return OrderView{}, false, nil
}
func (s *createStoreFake) ReserveAndCreate(_ context.Context, record CreateRecord, _ entity.Stroops) error {
	s.calls = append(s.calls, "reserve-and-create")
	s.created = record
	return nil
}
func (s *createStoreFake) AttachCheckout(_ context.Context, _ string, checkout Checkout) (OrderView, error) {
	s.calls = append(s.calls, "attach-checkout")
	s.attached = checkout
	return OrderView{ID: "order-1", Status: entity.OrderStatusPaymentPending}, nil
}
func (s *createStoreFake) FailCheckout(context.Context, string, string) error        { return nil }
func (s *createStoreFake) MarkCheckoutUnknown(context.Context, string, string) error { return nil }
func (s *createStoreFake) Get(context.Context, string, string) (OrderView, error) {
	return OrderView{}, nil
}
func (s *createStoreFake) List(context.Context, string, int, string) ([]OrderView, string, error) {
	return []OrderView{}, "", nil
}
func (s *createStoreFake) RecordCallbackReceipt(context.Context, CallbackReceipt) (bool, error) {
	return false, nil
}
func (s *createStoreFake) ExpectedPayment(context.Context, string) (ExpectedPayment, error) {
	return ExpectedPayment{}, nil
}
func (s *createStoreFake) ConfirmPaymentAndEnqueue(context.Context, PaymentConfirmation) error {
	return nil
}
func (s *createStoreFake) CompleteCallback(context.Context, string, string) error { return nil }

type priceFake struct{ now time.Time }

func (f priceFake) LatestXLMIDR(context.Context) (MarketPrice, error) {
	return MarketPrice{IDRPerXLM: "2500", ObservedAt: f.now}, nil
}

type treasuryFake struct{}

func (treasuryFake) SpendableBalance(context.Context, string) (entity.Stroops, error) {
	return entity.Stroops(1_000_000_000), nil
}

type destinationFake struct{}

func (destinationFake) ValidateDestination(string) error { return nil }

type gatewayFake struct{ calls []string }

func (g *gatewayFake) CreateCheckout(_ context.Context, input CheckoutInput) (Checkout, error) {
	g.calls = append(g.calls, "create-checkout")
	return Checkout{ProviderID: "pr-1", Method: input.Method, Status: "PENDING", PresentationType: "QR_STRING", PresentationValue: "qr-data"}, nil
}

func TestCreateReservesBeforeExposingCheckout(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	gateway := &gatewayFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: gateway, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "order-1", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, replay, err := service.Create(context.Background(), Command{
		ClientID: "client-1", IdempotencyKey: "idem-1", Amount: entity.IDR(100_000),
		PaymentMethod: PaymentMethodQRIS, Destination: "G" + strings.Repeat("A", 55),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if replay {
		t.Fatal("Create() replay = true")
	}
	if len(store.calls) < 2 || store.calls[0] != "reserve-and-create" || gateway.calls[0] != "create-checkout" || store.calls[1] != "attach-checkout" {
		t.Fatalf("store calls = %#v, gateway calls = %#v", store.calls, gateway.calls)
	}
}
