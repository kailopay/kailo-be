package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
)

type sep24OnrampCreatorFake struct {
	command Command
	view    OrderView
	err     error
}

func (f *sep24OnrampCreatorFake) Create(_ context.Context, command Command) (OrderView, bool, error) {
	f.command = command
	return f.view, false, f.err
}

type sep24OfframpCreatorFake struct {
	command OfframpCommand
	view    OrderView
	err     error
}

func (f *sep24OfframpCreatorFake) Create(_ context.Context, command OfframpCommand) (OrderView, bool, error) {
	f.command = command
	return f.view, false, f.err
}

type sep24OrderReaderFake struct {
	principal OrderPrincipal
	orderID   string
	view      OrderView
	err       error
}

func (f *sep24OrderReaderFake) Get(_ context.Context, principal OrderPrincipal, orderID string) (OrderView, error) {
	f.principal = principal
	f.orderID = orderID
	return f.view, f.err
}

type sep24TransactionRepositoryFake struct {
	record        Sep24TransactionRecord
	records       map[string]Sep24TransactionRecord
	createCalls   int
	findPrincipal OrderPrincipal
}

func (f *sep24TransactionRepositoryFake) Create(_ context.Context, record Sep24TransactionRecord) error {
	f.createCalls++
	f.record = record
	if f.records == nil {
		f.records = make(map[string]Sep24TransactionRecord)
	}
	f.records[record.TransactionID] = record
	return nil
}

func (f *sep24TransactionRepositoryFake) Find(_ context.Context, principal OrderPrincipal, transactionID string) (Sep24TransactionRecord, error) {
	f.findPrincipal = principal
	record, ok := f.records[transactionID]
	if !ok {
		return Sep24TransactionRecord{}, ErrSEP24TransactionNotFound
	}
	return record, nil
}

func (f *sep24TransactionRepositoryFake) List(_ context.Context, principal OrderPrincipal, limit int) ([]Sep24TransactionRecord, error) {
	f.findPrincipal = principal
	if limit > len(f.records) {
		limit = len(f.records)
	}
	list := make([]Sep24TransactionRecord, 0, limit)
	for _, record := range f.records {
		if len(list) == limit {
			break
		}
		list = append(list, record)
	}
	return list, nil
}

func TestSep24StartDepositCreatesAndMapsOwnedOrder(t *testing.T) {
	principal := sep24RetailPrincipal()
	onramp := &sep24OnrampCreatorFake{view: OrderView{
		ID: "order-deposit-1", Status: entity.OrderStatusPaymentPending,
	}}
	transactions := &sep24TransactionRepositoryFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       onramp,
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       &sep24OrderReaderFake{},
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	view, err := service.StartDeposit(context.Background(), Sep24DepositCommand{
		Principal:      principal,
		IdempotencyKey: "wallet-deposit-1",
		AssetCode:      NativeXLMAssetCode,
		AmountMinor:    entity.IDR(100_000),
		Destination:    "G" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Memo:           "wallet-memo",
	})
	if err != nil {
		t.Fatalf("StartDeposit() error = %v", err)
	}
	if view.ID != "deposit-order-deposit-1" || view.Kind != Sep24KindDeposit || view.Status != "pending_user_transfer_start" {
		t.Fatalf("view = %+v", view)
	}
	if onramp.command.Principal != principal {
		t.Fatalf("principal = %+v, want %+v", onramp.command.Principal, principal)
	}
	if onramp.command.IdempotencyKey != "sep24-deposit-wallet-deposit-1" {
		t.Fatalf("idempotency key = %q", onramp.command.IdempotencyKey)
	}
	if transactions.createCalls != 1 || transactions.record.OrderID != "order-deposit-1" || transactions.record.Principal != principal {
		t.Fatalf("mapping = %+v, calls = %d", transactions.record, transactions.createCalls)
	}
}

func TestSep24StartWithdrawCreatesAndMapsOwnedOrder(t *testing.T) {
	principal := sep24APIPrincipal()
	offramp := &sep24OfframpCreatorFake{view: OrderView{
		ID: "order-withdraw-1", Status: entity.OrderStatusAssetPending,
	}}
	transactions := &sep24TransactionRepositoryFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       &sep24OnrampCreatorFake{},
		Offramp:      offramp,
		Orders:       &sep24OrderReaderFake{},
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	view, err := service.StartWithdraw(context.Background(), Sep24WithdrawCommand{
		Principal:        principal,
		IdempotencyKey:   "wallet-withdraw-1",
		AssetCode:        NativeXLMAssetCode,
		AssetAmount:      "40",
		DestinationToken: "sandbox-destination",
	})
	if err != nil {
		t.Fatalf("StartWithdraw() error = %v", err)
	}
	if view.ID != "withdraw-order-withdraw-1" || view.Kind != Sep24KindWithdraw || view.Status != "pending_user_transfer_start" {
		t.Fatalf("view = %+v", view)
	}
	if offramp.command.Principal != principal || offramp.command.AssetNetwork != StellarTestnetNetwork || offramp.command.AssetCode != NativeXLMAssetCode {
		t.Fatalf("offramp command = %+v", offramp.command)
	}
	if offramp.command.IdempotencyKey != "sep24-withdraw-wallet-withdraw-1" {
		t.Fatalf("idempotency key = %q", offramp.command.IdempotencyKey)
	}
	if transactions.record.Kind != Sep24KindWithdraw || transactions.record.OrderID != "order-withdraw-1" {
		t.Fatalf("mapping = %+v", transactions.record)
	}
}

func TestSep24StartWithdrawRejectsMissingDestinationToken(t *testing.T) {
	offramp := &sep24OfframpCreatorFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       &sep24OnrampCreatorFake{},
		Offramp:      offramp,
		Orders:       &sep24OrderReaderFake{},
		Transactions: &sep24TransactionRepositoryFake{},
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	_, err = service.StartWithdraw(context.Background(), Sep24WithdrawCommand{
		Principal:      sep24APIPrincipal(),
		IdempotencyKey: "wallet-withdraw-missing-destination",
		AssetCode:      NativeXLMAssetCode,
		AssetAmount:    "40",
	})
	if !errors.Is(err, ErrSEP24InvalidRequest) {
		t.Fatalf("StartWithdraw() error = %v, want %v", err, ErrSEP24InvalidRequest)
	}
	if offramp.command != (OfframpCommand{}) {
		t.Fatalf("offramp was called for an invalid request: %+v", offramp.command)
	}
}

func TestSep24StartPropagatesKYCRequiredBeforeMapping(t *testing.T) {
	principal := sep24RetailPrincipal()
	onramp := &sep24OnrampCreatorFake{err: ErrKYCRequired}
	transactions := &sep24TransactionRepositoryFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       onramp,
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       &sep24OrderReaderFake{},
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	_, err = service.StartDeposit(context.Background(), Sep24DepositCommand{
		Principal:      principal,
		IdempotencyKey: "wallet-deposit-kyc",
		AssetCode:      NativeXLMAssetCode,
		AmountMinor:    entity.IDR(100_000),
		Destination:    "G" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if !errors.Is(err, ErrKYCRequired) {
		t.Fatalf("StartDeposit() error = %v, want %v", err, ErrKYCRequired)
	}
	if transactions.createCalls != 0 {
		t.Fatalf("mapping created for a non-approved user: %d", transactions.createCalls)
	}
}

func TestSep24StartRejectsMissingIdempotencyKeyBeforeCreatingOrder(t *testing.T) {
	onramp := &sep24OnrampCreatorFake{}
	transactions := &sep24TransactionRepositoryFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       onramp,
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       &sep24OrderReaderFake{},
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	_, err = service.StartDeposit(context.Background(), Sep24DepositCommand{
		Principal:   sep24RetailPrincipal(),
		AssetCode:   NativeXLMAssetCode,
		AmountMinor: entity.IDR(100_000),
		Destination: "G" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if !errors.Is(err, ErrSEP24InvalidRequest) {
		t.Fatalf("StartDeposit() error = %v, want %v", err, ErrSEP24InvalidRequest)
	}
	if onramp.command != (Command{}) || transactions.createCalls != 0 {
		t.Fatalf("side effects occurred: command = %+v, mappings = %d", onramp.command, transactions.createCalls)
	}
}

func TestSep24StartDepositMapsOrderWhenCheckoutOutcomeIsUnknown(t *testing.T) {
	principal := sep24RetailPrincipal()
	onramp := &sep24OnrampCreatorFake{err: &CheckoutUnknownError{OrderID: "order-unknown-1"}}
	orders := &sep24OrderReaderFake{view: OrderView{ID: "order-unknown-1", Status: entity.OrderStatusCreated, FailureCode: "checkout_unknown"}}
	transactions := &sep24TransactionRepositoryFake{}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       onramp,
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       orders,
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	view, err := service.StartDeposit(context.Background(), Sep24DepositCommand{
		Principal:      principal,
		IdempotencyKey: "wallet-deposit-unknown",
		AssetCode:      NativeXLMAssetCode,
		AmountMinor:    entity.IDR(100_000),
		Destination:    "G" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if err != nil {
		t.Fatalf("StartDeposit() error = %v", err)
	}
	if view.ID != "deposit-order-unknown-1" || view.Status != "pending_external" || transactions.createCalls != 1 || orders.orderID != "order-unknown-1" {
		t.Fatalf("view = %+v, mappings = %d, order lookup = %q", view, transactions.createCalls, orders.orderID)
	}
}

func TestSep24GetTransactionReadsCurrentOwnedOrderStatus(t *testing.T) {
	principal := sep24RetailPrincipal()
	transactions := &sep24TransactionRepositoryFake{records: map[string]Sep24TransactionRecord{
		"withdraw-order-1": {TransactionID: "withdraw-order-1", OrderID: "order-1", Kind: Sep24KindWithdraw},
	}}
	orders := &sep24OrderReaderFake{view: OrderView{ID: "order-1", Status: entity.OrderStatusAssetPending}}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       &sep24OnrampCreatorFake{},
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       orders,
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	view, err := service.GetTransaction(context.Background(), principal, "withdraw-order-1")
	if err != nil {
		t.Fatalf("GetTransaction() error = %v", err)
	}
	if view.Status != "pending_user_transfer_start" || view.Kind != Sep24KindWithdraw || view.Order.ID != "order-1" {
		t.Fatalf("view = %+v", view)
	}
	if orders.principal != principal || transactions.findPrincipal != principal {
		t.Fatalf("ownership was not propagated: order = %+v, mapping = %+v", orders.principal, transactions.findPrincipal)
	}
}

func TestSep24ListTransactionsLoadsCurrentOwnedOrderStatus(t *testing.T) {
	principal := sep24RetailPrincipal()
	transactions := &sep24TransactionRepositoryFake{records: map[string]Sep24TransactionRecord{
		"deposit-order-1": {TransactionID: "deposit-order-1", OrderID: "order-1", Kind: Sep24KindDeposit},
	}}
	orders := &sep24OrderReaderFake{view: OrderView{ID: "order-1", Status: entity.OrderStatusCompleted}}
	service, err := NewSep24Usecase(Sep24Dependencies{
		Onramp:       &sep24OnrampCreatorFake{},
		Offramp:      &sep24OfframpCreatorFake{},
		Orders:       orders,
		Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewSep24Usecase() error = %v", err)
	}

	views, err := service.ListTransactions(context.Background(), principal, 20)
	if err != nil {
		t.Fatalf("ListTransactions() error = %v", err)
	}
	if len(views) != 1 || views[0].ID != "deposit-order-1" || views[0].Status != "completed" {
		t.Fatalf("views = %+v", views)
	}
	if transactions.findPrincipal != principal || orders.principal != principal {
		t.Fatalf("ownership was not propagated: mapping = %+v, order = %+v", transactions.findPrincipal, orders.principal)
	}
}

func sep24RetailPrincipal() OrderPrincipal {
	return OrderPrincipal{Kind: OrderPrincipalRetailSession, OwnerUserID: "user-1", SessionID: "session-1"}
}

func sep24APIPrincipal() OrderPrincipal {
	return OrderPrincipal{Kind: OrderPrincipalAPIClient, ClientID: "client-1", OwnerUserID: "user-1"}
}
