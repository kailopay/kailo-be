package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	Sep24KindDeposit  = "deposit"
	Sep24KindWithdraw = "withdraw"

	Sep24InteractiveResponseType = "interactive_customer_info_needed"
	sep24IdempotencyPrefix       = "sep24-"
)

var (
	ErrSEP24InvalidRequest      = errors.New("invalid sep-24 request")
	ErrSEP24TransactionNotFound = errors.New("sep-24 transaction not found")
	ErrSEP24TransactionConflict = errors.New("sep-24 transaction conflicts with an existing mapping")
	ErrSEP24StatusUnavailable   = errors.New("sep-24 status is unavailable")
)

// Sep24DepositCommand is the authenticated input needed to create a sandbox
// on-ramp order from the SEP-24 interactive endpoint. AmountMinor is IDR minor
// units, an explicit KailoPay extension because the regular order flow needs a
// fiat amount before it can reserve a quote and checkout.
type Sep24DepositCommand struct {
	Principal      OrderPrincipal
	WalletAccount  string
	QuoteID        string
	IdempotencyKey string
	AssetCode      string
	AmountMinor    entity.IDR
	Destination    string
	Memo           string
	PaymentMethod  entity.PaymentMethod
}

// Sep24WithdrawCommand is the authenticated input needed to create a sandbox
// off-ramp order from the SEP-24 interactive endpoint.
type Sep24WithdrawCommand struct {
	Principal        OrderPrincipal
	WalletAccount    string
	QuoteID          string
	IdempotencyKey   string
	AssetCode        string
	AssetAmount      string
	DestinationToken string
}

// Sep24TransactionRecord is the durable bridge between a SEP-24 transaction
// identifier and the owned KailoPay order. Ownership remains authoritative on
// the order so session and API-client scopes cannot diverge.
type Sep24TransactionRecord struct {
	Principal             OrderPrincipal
	WalletAccount         string
	QuoteID               string
	TransactionID         string
	OrderID               string
	Kind                  string
	StellarTransactionID  string
	ExternalTransactionID string
}

// Sep24TransactionView is the use-case projection consumed by the protocol
// handler. Order contains the current persisted order state.
type Sep24TransactionView struct {
	ID                    string
	Kind                  string
	Status                string
	QuoteID               string
	StellarTransactionID  string
	ExternalTransactionID string
	Order                 OrderView
}

type Sep24OnrampCreator interface {
	Create(ctx context.Context, command Command) (OrderView, bool, error)
}

type Sep24OfframpCreator interface {
	Create(ctx context.Context, command OfframpCommand) (OrderView, bool, error)
}

type Sep24OrderReader interface {
	Get(ctx context.Context, principal OrderPrincipal, orderID string) (OrderView, error)
}

type Sep24TransactionRepository interface {
	Create(ctx context.Context, record Sep24TransactionRecord) error
	Find(ctx context.Context, principal OrderPrincipal, transactionID string) (Sep24TransactionRecord, error)
	List(ctx context.Context, principal OrderPrincipal, limit int) ([]Sep24TransactionRecord, error)
}

type Sep24WalletTransactionReader interface {
	FindByWallet(ctx context.Context, walletAccount, transactionID string) (Sep24TransactionRecord, error)
	ListByWallet(ctx context.Context, walletAccount string, limit int) ([]Sep24TransactionRecord, error)
}

type Sep24WalletIdentifierReader interface {
	FindByWalletIdentifier(ctx context.Context, walletAccount, identifier string) (Sep24TransactionRecord, error)
}

type Sep24WalletOrderReader interface {
	GetByWallet(ctx context.Context, walletAccount, orderID string) (OrderView, error)
}

type Sep24HistoryFilter struct {
	AssetCode   string
	Kind        string
	Limit       int
	NoOlderThan *time.Time
	PagingID    string
}

type Sep24Dependencies struct {
	Onramp       Sep24OnrampCreator
	Offramp      Sep24OfframpCreator
	Orders       Sep24OrderReader
	Transactions Sep24TransactionRepository
}

// Sep24Usecase coordinates protocol transactions with the normal order
// workflows. The order workflows remain responsible for KYC, quoting,
// idempotency, and business validation.
type Sep24Usecase struct {
	dependencies Sep24Dependencies
}

func NewSep24Usecase(dependencies Sep24Dependencies) (*Sep24Usecase, error) {
	if dependencies.Onramp == nil || dependencies.Offramp == nil || dependencies.Orders == nil || dependencies.Transactions == nil {
		return nil, errors.New("valid sep-24 dependencies are required")
	}
	return &Sep24Usecase{dependencies: dependencies}, nil
}

func (s *Sep24Usecase) StartDeposit(ctx context.Context, command Sep24DepositCommand) (Sep24TransactionView, error) {
	if err := validateSep24Start(command.Principal, command.IdempotencyKey, command.AssetCode); err != nil {
		return Sep24TransactionView{}, err
	}
	if command.AmountMinor <= 0 || strings.TrimSpace(command.Destination) == "" {
		return Sep24TransactionView{}, ErrSEP24InvalidRequest
	}
	view, _, err := s.dependencies.Onramp.Create(ctx, Command{
		Principal:      command.Principal,
		WalletAccount:  strings.TrimSpace(command.WalletAccount),
		QuoteID:        strings.TrimSpace(command.QuoteID),
		IdempotencyKey: sep24IdempotencyKey(Sep24KindDeposit, command.IdempotencyKey),
		Amount:         command.AmountMinor,
		PaymentMethod:  command.PaymentMethod,
		Destination:    command.Destination,
		Memo:           command.Memo,
	})
	if err != nil {
		var unknown *CheckoutUnknownError
		if !errors.As(err, &unknown) || unknown.OrderID == "" {
			return Sep24TransactionView{}, err
		}
		view, err = s.dependencies.Orders.Get(ctx, command.Principal, unknown.OrderID)
		if err != nil {
			return Sep24TransactionView{}, fmt.Errorf("loading checkout-unknown order: %w", err)
		}
	}
	return s.recordTransaction(ctx, command.Principal, command.WalletAccount, command.QuoteID, Sep24KindDeposit, view)
}

func (s *Sep24Usecase) StartWithdraw(ctx context.Context, command Sep24WithdrawCommand) (Sep24TransactionView, error) {
	if err := validateSep24Start(command.Principal, command.IdempotencyKey, command.AssetCode); err != nil {
		return Sep24TransactionView{}, err
	}
	if strings.TrimSpace(command.AssetAmount) == "" || strings.TrimSpace(command.DestinationToken) == "" {
		return Sep24TransactionView{}, ErrSEP24InvalidRequest
	}
	view, _, err := s.dependencies.Offramp.Create(ctx, OfframpCommand{
		Principal:        command.Principal,
		WalletAccount:    strings.TrimSpace(command.WalletAccount),
		QuoteID:          strings.TrimSpace(command.QuoteID),
		IdempotencyKey:   sep24IdempotencyKey(Sep24KindWithdraw, command.IdempotencyKey),
		AssetNetwork:     StellarTestnetNetwork,
		AssetCode:        command.AssetCode,
		FiatCurrency:     IDRCurrency,
		AssetAmount:      command.AssetAmount,
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer,
		DestinationToken: command.DestinationToken,
	})
	if err != nil {
		return Sep24TransactionView{}, err
	}
	return s.recordTransaction(ctx, command.Principal, command.WalletAccount, command.QuoteID, Sep24KindWithdraw, view)
}

func (s *Sep24Usecase) GetTransaction(ctx context.Context, principal OrderPrincipal, transactionID string) (Sep24TransactionView, error) {
	if err := principal.Validate(); err != nil {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" || len(transactionID) > 255 {
		return Sep24TransactionView{}, ErrSEP24InvalidRequest
	}
	record, err := s.dependencies.Transactions.Find(ctx, principal, transactionID)
	if err != nil {
		return Sep24TransactionView{}, err
	}
	view, err := s.dependencies.Orders.Get(ctx, principal, record.OrderID)
	if errors.Is(err, ErrOrderNotFound) {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if err != nil {
		return Sep24TransactionView{}, fmt.Errorf("loading sep-24 order: %w", err)
	}
	return s.transactionView(record, view)
}

func (s *Sep24Usecase) ListTransactions(ctx context.Context, principal OrderPrincipal, limit int) ([]Sep24TransactionView, error) {
	if err := principal.Validate(); err != nil {
		return []Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	records, err := s.dependencies.Transactions.List(ctx, principal, limit)
	if err != nil {
		return []Sep24TransactionView{}, fmt.Errorf("listing sep-24 transactions: %w", err)
	}
	views := make([]Sep24TransactionView, 0, len(records))
	for _, record := range records {
		view, err := s.GetTransaction(ctx, principal, record.TransactionID)
		if err != nil {
			return []Sep24TransactionView{}, fmt.Errorf("loading sep-24 transaction %q: %w", record.TransactionID, err)
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Sep24Usecase) GetWalletTransaction(ctx context.Context, walletAccount, transactionID string) (Sep24TransactionView, error) {
	reader, ok := s.dependencies.Transactions.(Sep24WalletTransactionReader)
	if !ok {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	orders, ok := s.dependencies.Orders.(Sep24WalletOrderReader)
	if !ok {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	walletAccount = strings.TrimSpace(walletAccount)
	if walletAccount == "" {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	record, err := reader.FindByWallet(ctx, walletAccount, strings.TrimSpace(transactionID))
	if err != nil {
		return Sep24TransactionView{}, err
	}
	order, err := orders.GetByWallet(ctx, walletAccount, record.OrderID)
	if errors.Is(err, ErrOrderNotFound) {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if err != nil {
		return Sep24TransactionView{}, fmt.Errorf("loading wallet SEP-24 order: %w", err)
	}
	return s.transactionView(record, order)
}

func (s *Sep24Usecase) GetWalletTransactionByIdentifier(ctx context.Context, walletAccount, identifier string) (Sep24TransactionView, error) {
	reader, ok := s.dependencies.Transactions.(Sep24WalletIdentifierReader)
	if !ok {
		return s.GetWalletTransaction(ctx, walletAccount, identifier)
	}
	orders, ok := s.dependencies.Orders.(Sep24WalletOrderReader)
	if !ok {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	record, err := reader.FindByWalletIdentifier(ctx, strings.TrimSpace(walletAccount), strings.TrimSpace(identifier))
	if err != nil {
		return Sep24TransactionView{}, err
	}
	order, err := orders.GetByWallet(ctx, walletAccount, record.OrderID)
	if errors.Is(err, ErrOrderNotFound) {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if err != nil {
		return Sep24TransactionView{}, fmt.Errorf("loading wallet SEP-24 order: %w", err)
	}
	return s.transactionView(record, order)
}

func (s *Sep24Usecase) ListWalletTransactions(ctx context.Context, walletAccount string, limit int) ([]Sep24TransactionView, error) {
	reader, ok := s.dependencies.Transactions.(Sep24WalletTransactionReader)
	if !ok {
		return []Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if _, ok := s.dependencies.Orders.(Sep24WalletOrderReader); !ok {
		return []Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	records, err := reader.ListByWallet(ctx, strings.TrimSpace(walletAccount), limit)
	if err != nil {
		return []Sep24TransactionView{}, fmt.Errorf("listing wallet SEP-24 transactions: %w", err)
	}
	views := make([]Sep24TransactionView, 0, len(records))
	for _, record := range records {
		view, err := s.GetWalletTransaction(ctx, walletAccount, record.TransactionID)
		if err != nil {
			return []Sep24TransactionView{}, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Sep24Usecase) ListWalletTransactionsFiltered(ctx context.Context, walletAccount string, filter Sep24HistoryFilter) ([]Sep24TransactionView, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 20
	}
	views, err := s.ListWalletTransactions(ctx, walletAccount, 100)
	if err != nil {
		return []Sep24TransactionView{}, err
	}
	assetCode := strings.ToLower(strings.TrimSpace(filter.AssetCode))
	kind := strings.ToLower(strings.TrimSpace(filter.Kind))
	pagingPassed := strings.TrimSpace(filter.PagingID) == ""
	filtered := make([]Sep24TransactionView, 0, filter.Limit)
	for _, view := range views {
		if !pagingPassed {
			if view.ID == strings.TrimSpace(filter.PagingID) {
				pagingPassed = true
			}
			continue
		}
		if filter.NoOlderThan != nil && view.Order.CreatedAt.After(filter.NoOlderThan.UTC()) {
			continue
		}
		if assetCode != "" && assetCode != "xlm" && assetCode != "native" && assetCode != "stellar:native" {
			continue
		}
		viewKind := strings.ToLower(view.Kind)
		if viewKind == Sep24KindWithdraw {
			viewKind = "withdrawal"
		}
		if kind != "" && kind != viewKind && !(kind == Sep24KindWithdraw && viewKind == "withdrawal") {
			continue
		}
		filtered = append(filtered, view)
		if len(filtered) == filter.Limit {
			break
		}
	}
	return filtered, nil
}

func (s *Sep24Usecase) recordTransaction(ctx context.Context, principal OrderPrincipal, walletAccount, quoteID, kind string, order OrderView) (Sep24TransactionView, error) {
	if strings.TrimSpace(order.ID) == "" {
		return Sep24TransactionView{}, ErrSEP24StatusUnavailable
	}
	transactionID := Sep24TransactionID(kind, order.ID)
	record := Sep24TransactionRecord{Principal: principal, WalletAccount: strings.TrimSpace(walletAccount), QuoteID: strings.TrimSpace(quoteID), TransactionID: transactionID, OrderID: order.ID, Kind: kind}
	if err := s.dependencies.Transactions.Create(ctx, record); err != nil {
		return Sep24TransactionView{}, fmt.Errorf("recording sep-24 transaction: %w", err)
	}
	return s.transactionView(record, order)
}

func (s *Sep24Usecase) transactionView(record Sep24TransactionRecord, order OrderView) (Sep24TransactionView, error) {
	status, ok := Sep24OrderStatus(order)
	if !ok {
		return Sep24TransactionView{}, ErrSEP24StatusUnavailable
	}
	return Sep24TransactionView{ID: record.TransactionID, Kind: record.Kind, Status: status, QuoteID: record.QuoteID,
		StellarTransactionID: record.StellarTransactionID, ExternalTransactionID: record.ExternalTransactionID, Order: order}, nil
}

func validateSep24Start(principal OrderPrincipal, idempotencyKey, assetCode string) error {
	if err := principal.Validate(); err != nil {
		return ErrSEP24InvalidRequest
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 240 || strings.TrimSpace(assetCode) != NativeXLMAssetCode {
		return ErrSEP24InvalidRequest
	}
	return nil
}

func sep24IdempotencyKey(kind, key string) string {
	return sep24IdempotencyPrefix + kind + "-" + strings.TrimSpace(key)
}

// Sep24Status maps internal order states onto the SEP-24 transaction status
// vocabulary (STELLAR-ANCHOR-INTEGRATION §8). ok is false for states with no
// SEP-24 counterpart.
func Sep24Status(status string) (string, bool) {
	switch entity.OrderStatus(status) {
	case entity.OrderStatusCreated:
		return "pending_user_transfer_start", true
	case entity.OrderStatusPaymentPending, entity.OrderStatusAssetPending:
		return "pending_user_transfer_start", true
	case entity.OrderStatusPaymentConfirmed, entity.OrderStatusAssetReceived:
		return "pending_anchor", true
	case entity.OrderStatusStellarProcessing, entity.OrderStatusRetirementProcessing, entity.OrderStatusWithdrawalProcessing:
		return "pending_external", true
	case entity.OrderStatusCompleted:
		return "completed", true
	case entity.OrderStatusExpired:
		return "expired", true
	case entity.OrderStatusPaymentFailed, entity.OrderStatusStellarFailed,
		entity.OrderStatusAssetInvalid, entity.OrderStatusRetirementFailed,
		entity.OrderStatusWithdrawalFailed, entity.OrderStatusCancelled:
		return "error", true
	default:
		return "", false
	}
}

// Sep24OrderStatus maps an order and its safe failure metadata to the current
// SEP-24 state. A provider timeout is explicitly pending external
// reconciliation; it must not look like a fresh order awaiting user action.
func Sep24OrderStatus(order OrderView) (string, bool) {
	if order.FailureCode == "checkout_unknown" {
		return "pending_external", true
	}
	return Sep24Status(string(order.Status))
}

// MapToSEP24Status exposes the mapping with the exported naming used by
// handlers; ok is false when no SEP-24 counterpart exists.
func MapToSEP24Status(status interface{ String() string }) (string, bool) {
	return Sep24Status(status.String())
}

// FederationResolver resolves synthetic demo federation names of the form
// <memo>*kailopay to the configured deposit account. Only names matching the
// documented sandbox pattern resolve; everything else 404s.
type FederationResolver struct {
	DepositAccount string
}

const federationDomain = "kailopay"

func (r *FederationResolver) Resolve(query string) (address, memo string, ok bool) {
	query = strings.TrimSpace(strings.ToLower(query))
	name, domain, found := strings.Cut(query, "*")
	if !found || domain != federationDomain || name == "" || len(name) > 28 ||
		strings.ContainsAny(name, " \t\r\n") || !isFederationNameSafe(name) {
		return "", "", false
	}
	return r.DepositAccount, "fed-" + name, true
}

func isFederationNameSafe(name string) bool {
	for _, character := range name {
		alnum := character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if !alnum && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

// Sep24TransactionID returns a stable opaque identifier derived from the
// already opaque order ID. Stable derivation lets a retry restore a mapping if
// the order was created before the mapping insert completed.
func Sep24TransactionID(kind, orderID string) string {
	return strings.TrimSpace(kind) + "-" + strings.TrimSpace(orderID)
}
