package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type createStoreFake struct {
	calls           []string
	created         CreateRecord
	attached        Checkout
	replayed        OrderView
	found           bool
	findCalls       int
	reserveErr      error
	findPrincipal   OrderPrincipal
	findKeyHash     string
	findRequestHash string
}

func (s *createStoreFake) FindReplay(_ context.Context, principal OrderPrincipal, keyHash, requestHash string) (OrderView, bool, error) {
	s.findCalls++
	s.findPrincipal = principal
	s.findKeyHash = keyHash
	s.findRequestHash = requestHash
	return s.replayed, s.found, nil
}
func (s *createStoreFake) ReserveAndCreate(_ context.Context, record CreateRecord, _ entity.Stroops) error {
	s.calls = append(s.calls, "reserve-and-create")
	s.created = record
	if s.reserveErr != nil {
		s.found = true
	}
	return s.reserveErr
}
func (s *createStoreFake) AttachCheckout(_ context.Context, _ string, checkout Checkout) (OrderView, error) {
	s.calls = append(s.calls, "attach-checkout")
	s.attached = checkout
	return OrderView{ID: "order-1", Status: entity.OrderStatusPaymentPending}, nil
}
func (s *createStoreFake) FailCheckout(context.Context, string, string) error        { return nil }
func (s *createStoreFake) MarkCheckoutUnknown(context.Context, string, string) error { return nil }
func (s *createStoreFake) Get(context.Context, OrderPrincipal, string) (OrderView, error) {
	return OrderView{}, nil
}
func (s *createStoreFake) List(context.Context, OrderPrincipal, int, string) ([]OrderView, string, error) {
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

type unknownGatewayFake struct{}

func (unknownGatewayFake) CreateCheckout(context.Context, CheckoutInput) (Checkout, error) {
	return Checkout{}, &GatewayError{Unknown: true, Err: errors.New("provider timeout")}
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
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-1", Amount: entity.IDR(100_000),
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

func TestCreateDefaultsToHostedXenditPaymentMethod(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: &gatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "order-1", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	if _, _, err := service.Create(context.Background(), Command{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-hosted-1", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.created.PaymentMethod != PaymentMethodXendit {
		t.Fatalf("payment method = %q, want %q", store.created.PaymentMethod, PaymentMethodXendit)
	}
}

func apiOrderPrincipal(clientID, ownerUserID string) OrderPrincipal {
	return OrderPrincipal{Kind: OrderPrincipalAPIClient, ClientID: clientID, OwnerUserID: ownerUserID}
}

func TestCreateCarriesRetailPrincipalAndScopesRequestHashToOperation(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: &gatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "order-retail-1", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	principal := OrderPrincipal{Kind: OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"}
	if _, replay, err := service.Create(context.Background(), Command{
		Principal: principal, IdempotencyKey: "idem-retail-1", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	}); err != nil || replay {
		t.Fatalf("Create() = %v, replay=%v", err, replay)
	}
	if store.created.Principal != principal {
		t.Fatalf("created principal = %+v, want %+v", store.created.Principal, principal)
	}
	wantHash := orderRequestHash(onrampRequestOperation, "100000", "xendit", "G"+strings.Repeat("A", 55), "")
	if store.findRequestHash != wantHash {
		t.Fatalf("request hash = %q, want %q", store.findRequestHash, wantHash)
	}
}

func TestCreateRejectsInvalidOrderPrincipalBeforeSideEffects(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: &gatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "order-invalid-principal", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, _, err = service.Create(context.Background(), Command{
		IdempotencyKey: "idem-invalid-principal", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	})
	if !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("Create() error = %v, want %v", err, ErrInvalidCommand)
	}
	if len(store.calls) != 0 {
		t.Fatalf("repository calls = %#v, want none", store.calls)
	}
}

func TestCreateReturnsTrackableOrderIDWhenCheckoutOutcomeIsUnknown(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: unknownGatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "order-unknown-1", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, replay, err := service.Create(context.Background(), Command{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-unknown-1", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	})
	var unknown *CheckoutUnknownError
	if !errors.As(err, &unknown) || unknown.OrderID != "order-unknown-1" || replay {
		t.Fatalf("Create() = err %v, replay %v, unknown %+v", err, replay, unknown)
	}
}

func TestCreateReplaysTheSameTrackableOrderAfterUnknownCheckout(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{replayed: OrderView{ID: "order-unknown-existing", FailureCode: "checkout_unknown"}, found: true}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: unknownGatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "should-not-create", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, replay, err := service.Create(context.Background(), Command{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-unknown-existing", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	})
	var unknown *CheckoutUnknownError
	if !errors.As(err, &unknown) || unknown.OrderID != "order-unknown-existing" || !replay {
		t.Fatalf("Create() = err %v, replay %v, unknown %+v", err, replay, unknown)
	}
}

func TestCreateRecoversConcurrentIdempotencyReplayAfterReservationRace(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{
		replayed:   OrderView{ID: "order-race-existing", Status: entity.OrderStatusPaymentPending},
		reserveErr: errors.New("duplicate idempotency record"),
	}
	service, err := NewOnrampUsecase(OnrampDependencies{Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: &gatewayFake{}, Destinations: destinationFake{}}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute},
		MinIDR:      10_000, MaxIDR: 10_000_000, TreasuryAccount: "G" + string(make([]byte, 55)),
		NewID: func() (string, error) { return "should-not-create", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, replay, err := service.Create(context.Background(), Command{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-race-1", Amount: entity.IDR(100_000),
		Destination: "G" + strings.Repeat("A", 55),
	})
	if err != nil || !replay || store.findCalls != 2 {
		t.Fatalf("Create() = err %v, replay %v, find calls %d", err, replay, store.findCalls)
	}
}
