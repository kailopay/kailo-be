package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	PaymentMethodQRIS  = entity.PaymentMethodQRIS
	PaymentMethodBRIVA = entity.PaymentMethodBRIVA
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
	FindReplay(ctx context.Context, clientID, idempotencyKeyHash, requestHash string) (OrderView, bool, error)
	ReserveAndCreate(ctx context.Context, record CreateRecord, observedBalance entity.Stroops) error
	AttachCheckout(ctx context.Context, orderID string, checkout Checkout) (OrderView, error)
	FailCheckout(ctx context.Context, orderID, reason string) error
	MarkCheckoutUnknown(ctx context.Context, orderID, reason string) error
	Get(ctx context.Context, clientID, orderID string) (OrderView, error)
	List(ctx context.Context, clientID string, limit int, cursor string) ([]OrderView, string, error)
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
	ClientID       string
	IdempotencyKey string
	Amount         entity.IDR
	PaymentMethod  entity.PaymentMethod
	Destination    string
	Memo           string
}

type CreateRecord struct {
	OrderID            string
	ClientID           string
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
	Method            entity.PaymentMethod
	Status            string
	PresentationType  string
	PresentationValue string
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
	command.ClientID = strings.TrimSpace(command.ClientID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.Destination = strings.TrimSpace(command.Destination)
	command.Memo = strings.TrimSpace(command.Memo)
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
	requestHash := digest(fmt.Sprintf("%d|%s|%s|%s", command.Amount, command.PaymentMethod, command.Destination, command.Memo))
	replayed, found, err := s.dependencies.Repository.FindReplay(ctx, command.ClientID, idempotencyHash, requestHash)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("checking idempotency: %w", err)
	}
	if found {
		if replayed.FailureCode == "checkout_unknown" {
			return replayed, true, ErrCheckoutUnknown
		}
		return replayed, true, nil
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
	record := CreateRecord{OrderID: orderID, ClientID: command.ClientID, IdempotencyKeyHash: idempotencyHash,
		RequestHash: requestHash, PaymentMethod: command.PaymentMethod, Destination: command.Destination,
		Memo: command.Memo, Quote: quote, CreatedAt: now}
	if err := s.dependencies.Repository.ReserveAndCreate(ctx, record, observedBalance); err != nil {
		return OrderView{}, false, fmt.Errorf("reserving treasury inventory: %w", err)
	}
	checkout, err := s.dependencies.Gateway.CreateCheckout(ctx, CheckoutInput{
		OrderID: orderID, Method: command.PaymentMethod, Amount: command.Amount, ExpiresAt: quote.ExpiresAt,
	})
	if err != nil {
		var gatewayError *GatewayError
		if errors.As(err, &gatewayError) && gatewayError.Unknown {
			_ = s.dependencies.Repository.MarkCheckoutUnknown(ctx, orderID, "provider outcome unknown")
			return OrderView{}, false, ErrCheckoutUnknown
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

func (s *OnrampUsecase) Get(ctx context.Context, clientID, orderID string) (OrderView, error) {
	return s.dependencies.Repository.Get(ctx, clientID, orderID)
}

func (s *OnrampUsecase) List(ctx context.Context, clientID string, limit int, cursor string) ([]OrderView, string, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.dependencies.Repository.List(ctx, clientID, limit, cursor)
}

func validateCommand(command Command) error {
	if command.ClientID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 255 || command.Amount.Validate() != nil ||
		(command.PaymentMethod != PaymentMethodQRIS && command.PaymentMethod != PaymentMethodBRIVA) ||
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
