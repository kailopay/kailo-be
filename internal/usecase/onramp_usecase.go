package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	onrampRequestOperation = "onramp.create"
	PaymentMethodQRIS      = entity.PaymentMethodQRIS
	PaymentMethodBRIVA     = entity.PaymentMethodBRIVA
	PaymentMethodXendit    = entity.PaymentMethodXendit
)

var (
	ErrInvalidCommand        = errors.New("invalid onramp command")
	ErrInvalidDestination    = errors.New("invalid stellar destination")
	ErrAmountOutOfRange      = errors.New("fiat amount is outside the supported range")
	ErrInsufficientLiquidity = errors.New("insufficient treasury liquidity")
	ErrIdempotencyConflict   = errors.New("idempotency key was reused with a different request")
	ErrOrderNotFound         = errors.New("order not found")
	ErrCheckoutUnknown       = errors.New("checkout outcome is unknown")
)

// CheckoutUnknownError identifies an order whose provider outcome must be
// reconciled before the client retries. It unwraps to ErrCheckoutUnknown so
// callers can keep the stable error classification.
type CheckoutUnknownError struct {
	OrderID string
}

func (e *CheckoutUnknownError) Error() string { return ErrCheckoutUnknown.Error() }
func (e *CheckoutUnknownError) Unwrap() error { return ErrCheckoutUnknown }

type PriceReader interface {
	LatestXLMIDR(ctx context.Context) (MarketPrice, error)
}

type TreasuryReader interface {
	SpendableBalance(ctx context.Context, account string) (entity.Stroops, error)
}

type DestinationValidator interface {
	ValidateDestination(account string) error
}

type PaymentGateway interface {
	CreateCheckout(ctx context.Context, input CheckoutInput) (Checkout, error)
}

type OnrampRepository interface {
	FindReplay(ctx context.Context, principal OrderPrincipal, idempotencyKeyHash, requestHash string) (OrderView, bool, error)
	ReserveAndCreate(ctx context.Context, record CreateRecord, observedBalance entity.Stroops) error
	AttachCheckout(ctx context.Context, orderID string, checkout Checkout) (OrderView, error)
	FailCheckout(ctx context.Context, orderID, reason string) error
	MarkCheckoutUnknown(ctx context.Context, orderID, reason string) error
	Get(ctx context.Context, principal OrderPrincipal, orderID string) (OrderView, error)
	List(ctx context.Context, principal OrderPrincipal, limit int, cursor string) ([]OrderView, string, error)
	RecordCallbackReceipt(ctx context.Context, receipt CallbackReceipt) (processed bool, err error)
	ExpectedPayment(ctx context.Context, providerID string) (ExpectedPayment, error)
	ConfirmPaymentAndEnqueue(ctx context.Context, confirmation PaymentConfirmation) error
	CompleteCallback(ctx context.Context, eventID, result string) error
}

type OnrampDependencies struct {
	Repository   OnrampRepository
	Prices       PriceReader
	Treasury     TreasuryReader
	Gateway      PaymentGateway
	Destinations DestinationValidator
}

type ServiceConfig struct {
	QuotePolicy     QuotePolicy
	MinIDR          entity.IDR
	MaxIDR          entity.IDR
	TreasuryAccount string
	NewID           func() (string, error)
	Now             func() time.Time
}

type Command struct {
	Principal      OrderPrincipal
	IdempotencyKey string
	Amount         entity.IDR
	PaymentMethod  entity.PaymentMethod
	Destination    string
	Memo           string
}

type CreateRecord struct {
	OrderID            string
	Principal          OrderPrincipal
	IdempotencyKeyHash string
	RequestHash        string
	PaymentMethod      entity.PaymentMethod
	Destination        string
	Memo               string
	Quote              Quote
	CreatedAt          time.Time
}

type CheckoutInput struct {
	OrderID   string
	Method    entity.PaymentMethod
	Amount    entity.IDR
	ExpiresAt time.Time
}

type Checkout struct {
	ProviderID        string
	PaymentRequestID  string
	Method            entity.PaymentMethod
	Status            string
	PresentationType  string
	PresentationValue string
	PaymentLinkURL    string
	ExpiresAt         *time.Time
}

type OrderView struct {
	ID                     string               `json:"id"`
	Status                 entity.OrderStatus   `json:"status"`
	FiatAmountMinor        entity.IDR           `json:"-"`
	AssetAmount            entity.Stroops       `json:"-"`
	QuoteRate              string               `json:"quote_rate"`
	QuoteAdjustedRate      string               `json:"quote_adjusted_rate"`
	QuoteSpreadBPS         int                  `json:"quote_spread_bps"`
	QuoteSourceAt          time.Time            `json:"quote_source_at"`
	QuoteExpiresAt         time.Time            `json:"quote_expires_at"`
	PaymentMethod          entity.PaymentMethod `json:"payment_method"`
	StellarDestination     string               `json:"stellar_destination"`
	StellarMemo            string               `json:"stellar_memo,omitempty"`
	Checkout               *Checkout            `json:"checkout,omitempty"`
	StellarTransactionHash string               `json:"stellar_transaction_hash,omitempty"`
	DepositTransactionHash string               `json:"deposit_transaction_hash,omitempty"`
	Payout                 *PayoutView          `json:"payout,omitempty"`
	FailureCode            string               `json:"failure_code,omitempty"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
}

type GatewayError struct {
	Unknown bool
	Err     error
}

func (e *GatewayError) Error() string { return e.Err.Error() }
func (e *GatewayError) Unwrap() error { return e.Err }

type OnrampUsecase struct {
	dependencies OnrampDependencies
	config       ServiceConfig
}

func NewOnrampUsecase(dependencies OnrampDependencies, config ServiceConfig) (*OnrampUsecase, error) {
	if dependencies.Repository == nil || dependencies.Prices == nil || dependencies.Treasury == nil || dependencies.Gateway == nil || dependencies.Destinations == nil ||
		config.NewID == nil || config.Now == nil || strings.TrimSpace(config.TreasuryAccount) == "" ||
		config.MinIDR <= 0 || config.MaxIDR < config.MinIDR {
		return nil, errors.New("valid onramp dependencies and configuration are required")
	}
	return &OnrampUsecase{dependencies: dependencies, config: config}, nil
}

func (s *OnrampUsecase) Create(ctx context.Context, command Command) (OrderView, bool, error) {
	if err := command.Principal.Validate(); err != nil {
		return OrderView{}, false, ErrInvalidCommand
	}
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Destination = strings.TrimSpace(command.Destination)
	command.Memo = strings.TrimSpace(command.Memo)
	if command.PaymentMethod == entity.PaymentMethodUnknown {
		command.PaymentMethod = PaymentMethodXendit
	}
	if err := validateCommand(command); err != nil {
		return OrderView{}, false, err
	}
	if err := s.dependencies.Destinations.ValidateDestination(command.Destination); err != nil {
		return OrderView{}, false, ErrInvalidDestination
	}
	if command.Amount < s.config.MinIDR || command.Amount > s.config.MaxIDR {
		return OrderView{}, false, ErrAmountOutOfRange
	}
	idempotencyHash := digest(command.IdempotencyKey)
	requestHash := orderRequestHash(onrampRequestOperation, strconv.FormatInt(int64(command.Amount), 10), string(command.PaymentMethod), command.Destination, command.Memo)
	replayed, found, err := s.dependencies.Repository.FindReplay(ctx, command.Principal, idempotencyHash, requestHash)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("checking idempotency: %w", err)
	}
	if found {
		return replayed, true, replayError(replayed)
	}
	market, err := s.dependencies.Prices.LatestXLMIDR(ctx)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("reading XLM IDR price: %w", err)
	}
	now := s.config.Now().UTC()
	quote, err := s.config.QuotePolicy.Create(now, command.Amount, market)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("creating quote: %w", err)
	}
	observedBalance, err := s.dependencies.Treasury.SpendableBalance(ctx, s.config.TreasuryAccount)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("reading treasury balance: %w", err)
	}
	orderID, err := s.config.NewID()
	if err != nil {
		return OrderView{}, false, fmt.Errorf("generating order id: %w", err)
	}
	record := CreateRecord{OrderID: orderID, Principal: command.Principal, IdempotencyKeyHash: idempotencyHash,
		RequestHash: requestHash, PaymentMethod: command.PaymentMethod, Destination: command.Destination,
		Memo: command.Memo, Quote: quote, CreatedAt: now}
	if err := s.dependencies.Repository.ReserveAndCreate(ctx, record, observedBalance); err != nil {
		if replayed, found, replayErr := s.dependencies.Repository.FindReplay(ctx, command.Principal, idempotencyHash, requestHash); replayErr == nil && found {
			return replayed, true, replayError(replayed)
		} else if replayErr != nil && errors.Is(replayErr, ErrIdempotencyConflict) {
			return OrderView{}, false, replayErr
		}
		return OrderView{}, false, fmt.Errorf("reserving treasury inventory: %w", err)
	}
	checkout, err := s.dependencies.Gateway.CreateCheckout(ctx, CheckoutInput{
		OrderID: orderID, Method: command.PaymentMethod, Amount: command.Amount, ExpiresAt: quote.ExpiresAt,
	})
	if err != nil {
		var gatewayError *GatewayError
		if errors.As(err, &gatewayError) && gatewayError.Unknown {
			_ = s.dependencies.Repository.MarkCheckoutUnknown(ctx, orderID, "provider outcome unknown")
			return OrderView{}, false, &CheckoutUnknownError{OrderID: orderID}
		}
		_ = s.dependencies.Repository.FailCheckout(ctx, orderID, "provider rejected checkout")
		return OrderView{}, false, fmt.Errorf("creating payment checkout: %w", err)
	}
	view, err := s.dependencies.Repository.AttachCheckout(ctx, orderID, checkout)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("attaching payment checkout: %w", err)
	}
	return view, false, nil
}

func (s *OnrampUsecase) Get(ctx context.Context, principal OrderPrincipal, orderID string) (OrderView, error) {
	if err := principal.Validate(); err != nil {
		return OrderView{}, ErrOrderNotFound
	}
	return s.dependencies.Repository.Get(ctx, principal, orderID)
}

func (s *OnrampUsecase) List(ctx context.Context, principal OrderPrincipal, limit int, cursor string) ([]OrderView, string, error) {
	if err := principal.Validate(); err != nil {
		return nil, "", ErrOrderNotFound
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.dependencies.Repository.List(ctx, principal, limit, cursor)
}

func validateCommand(command Command) error {
	if command.IdempotencyKey == "" || len(command.IdempotencyKey) > 255 || command.Amount.Validate() != nil ||
		(command.PaymentMethod != PaymentMethodQRIS && command.PaymentMethod != PaymentMethodBRIVA && command.PaymentMethod != PaymentMethodXendit) ||
		len(command.Destination) != 56 || !strings.HasPrefix(command.Destination, "G") || len(command.Memo) > 28 {
		return ErrInvalidCommand
	}
	for _, character := range command.Destination {
		if !(character >= 'A' && character <= 'Z') && !(character >= '2' && character <= '7') {
			return ErrInvalidCommand
		}
	}
	return nil
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func replayError(view OrderView) error {
	if view.FailureCode == "checkout_unknown" {
		return &CheckoutUnknownError{OrderID: view.ID}
	}
	return nil
}

func orderRequestHash(operation string, values ...string) string {
	var builder strings.Builder
	parts := make([]string, 0, len(values)+1)
	parts = append(parts, operation)
	parts = append(parts, values...)
	for _, part := range parts {
		builder.WriteString(strconv.Itoa(len(part)))
		builder.WriteByte(':')
		builder.WriteString(part)
	}
	return digest(builder.String())
}
