package usecase

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrInvalidWithdrawal = errors.New("invalid withdrawal request")
)

// DepositWatcher supplies recent native payments credited to the deposit
// account. The stellar adapter implements it from Horizon payment history.
type DepositWatcher interface {
	RecentPayments(ctx context.Context, account string, limit int) ([]ObservedPayment, error)
}

// ObservedPayment mirrors the adapter observation so the usecase owns its
// view of external facts.
type ObservedPayment struct {
	TransactionHash string
	From            string
	To              string
	Amount          entity.Stroops
	Memo            string
	LedgerAt        time.Time
}

// BurnAddress is the standard unspendable Stellar address (ADR-003).
const BurnAddress = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWH4"

// OfframpDepositMemo returns the required deposit memo for an order.
func OfframpDepositMemo(orderID string) string {
	return "off" + strings.ReplaceAll(orderID, "-", "")
}

// WithdrawalMethod names the fiat rail; only simulation exists in sandbox.
type WithdrawalMethod = entity.WithdrawalMethod

// OfframpRepository is the persistence port consumed by the off-ramp
// workflows and worker jobs.
type OfframpRepository interface {
	FindOfframpReplay(ctx context.Context, clientID, idempotencyKeyHash, requestHash string) (OrderView, bool, error)
	CreateOfframp(ctx context.Context, record OfframpCreateRecord) error
	Get(ctx context.Context, clientID, orderID string) (OrderView, error)
	List(ctx context.Context, clientID string, limit int, cursor string) ([]OrderView, string, error)

	FindDepositCandidates(ctx context.Context, depositAccount string, limit int) ([]OrderView, error)
	RecordAssetReceived(ctx context.Context, orderID string, payment ObservedPayment) error
	RecordAssetInvalid(ctx context.Context, orderID string, hash, safeReason string) error

	LoadRetirement(ctx context.Context, intentID string) (RetirementIntent, error)
	SaveRetirementHash(ctx context.Context, intentID, hash string, now time.Time) error
	ConfirmRetirement(ctx context.Context, intentID, hash string, now time.Time) error
	FailRetirement(ctx context.Context, intentID, safeError string) error

	CompleteSimulatedPayout(ctx context.Context, orderID string, amountMinor int64, now time.Time) (string, error)
}

// OfframpCreateRecord carries everything needed to persist one off-ramp
// order with its deposit instructions atomically.
type OfframpCreateRecord struct {
	OrderID            string
	ClientID           string
	IdempotencyKeyHash string
	RequestHash        string
	AssetAmount        entity.Stroops
	Quote              Quote
	WithdrawalMethod   entity.WithdrawalMethod
	DestinationToken   []byte
	Memo               string
	ExpiresAt          time.Time
	CreatedAt          time.Time
}

// RetirementIntent is the worker-side view of a pending burn submission.
type RetirementIntent struct {
	IntentID   string
	OrderID    string
	Source     string
	BurnTarget string
	Amount     entity.Stroops
	Memo       string
	TransactionHash string
}

// PayoutView is the public simulated-payout representation. Simulated is
// always true in this release (ADR-003) and must surface in every client.
type PayoutView struct {
	Reference   string
	Method      string
	AmountMinor int64
	State       string
	Simulated   *bool
}

// OfframpDependencies bundles the ports the off-ramp create flow consumes.
type OfframpDependencies struct {
	Repository  OfframpRepository
	Prices      PriceReader
	Destinations DestinationValidator
}

// OfframpServiceConfig configures the off-ramp create flow.
type OfframpServiceConfig struct {
	QuotePolicy     QuotePolicy
	MinIDR          entity.IDR
	MaxIDR          entity.IDR
	DepositAccount  string
	DepositExpiry   time.Duration
	NewID           func() (string, error)
	Now             func() time.Time
}

// OfframpCommand is the public create-off-ramp input.
type OfframpCommand struct {
	ClientID         string
	IdempotencyKey   string
	AssetAmount      string
	WithdrawalMethod entity.WithdrawalMethod
	DestinationToken string
}

// OfframpUsecase implements the sell-side application workflow.
type OfframpUsecase struct {
	dependencies OfframpDependencies
	config       OfframpServiceConfig
}

func NewOfframpUsecase(dependencies OfframpDependencies, config OfframpServiceConfig) (*OfframpUsecase, error) {
	if dependencies.Repository == nil || dependencies.Prices == nil || dependencies.Destinations == nil ||
		config.NewID == nil || config.Now == nil ||
		strings.TrimSpace(config.DepositAccount) == "" || config.DepositExpiry <= 0 ||
		config.MinIDR <= 0 || config.MaxIDR < config.MinIDR {
		return nil, errors.New("valid off-ramp dependencies and configuration are required")
	}
	if err := dependencies.Destinations.ValidateDestination(config.DepositAccount); err != nil {
		return nil, errors.New("off-ramp deposit account must be a valid Stellar testnet address")
	}
	return &OfframpUsecase{dependencies: dependencies, config: config}, nil
}

// Create validates the sell command, prices it against the current market,
// and persists the order with deterministic deposit instructions.
func (s *OfframpUsecase) Create(ctx context.Context, command OfframpCommand) (OrderView, bool, error) {
	command.ClientID = strings.TrimSpace(command.ClientID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.AssetAmount = strings.TrimSpace(command.AssetAmount)
	if command.ClientID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 255 ||
		command.WithdrawalMethod != entity.WithdrawalMethodSandboxTransfer || len(command.DestinationToken) > 200 {
		return OrderView{}, false, ErrInvalidWithdrawal
	}
	stroops, err := parseDecimalStroops(command.AssetAmount)
	if err != nil {
		return OrderView{}, false, ErrInvalidWithdrawal
	}
	now := s.config.Now().UTC()

	market, err := s.dependencies.Prices.LatestXLMIDR(ctx)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("reading XLM IDR price: %w", err)
	}
	quote, err := s.config.QuotePolicy.CreateReverse(now, stroops, market)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("creating quote: %w", err)
	}
	if quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR {
		return OrderView{}, false, ErrAmountOutOfRange
	}

	idempotencyHash := digest(command.IdempotencyKey)
	requestHash := digest(fmt.Sprintf("%s|%s|%s", command.AssetAmount, command.WithdrawalMethod, command.DestinationToken))
	replayed, found, err := s.dependencies.Repository.FindOfframpReplay(ctx, command.ClientID, idempotencyHash, requestHash)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("checking idempotency: %w", err)
	}
	if found {
		return replayed, true, nil
	}

	orderID, err := s.config.NewID()
	if err != nil {
		return OrderView{}, false, fmt.Errorf("generating order id: %w", err)
	}
	record := OfframpCreateRecord{
		OrderID:            orderID,
		ClientID:           command.ClientID,
		IdempotencyKeyHash: idempotencyHash,
		RequestHash:        requestHash,
		AssetAmount:        stroops,
		Quote:              quote,
		WithdrawalMethod:   command.WithdrawalMethod,
		DestinationToken:   []byte(command.DestinationToken),
		Memo:               OfframpDepositMemo(orderID),
		ExpiresAt:          now.Add(s.config.DepositExpiry),
		CreatedAt:          now,
	}
	if err := s.dependencies.Repository.CreateOfframp(ctx, record); err != nil {
		return OrderView{}, false, fmt.Errorf("creating off-ramp order: %w", err)
	}
	view, err := s.dependencies.Repository.Get(ctx, command.ClientID, orderID)
	if err != nil {
		return OrderView{}, false, fmt.Errorf("loading created order: %w", err)
	}
	return view, false, nil
}

// parseDecimalStroops converts an exact decimal XLM string (up to 7
// fractional digits) into stroops without any floating-point arithmetic.
func parseDecimalStroops(value string) (entity.Stroops, error) {
	whole, fraction, ok := strings.Cut(value, ".")
	if !ok {
		fraction = ""
	}
	if whole == "" && fraction == "" {
		return 0, errors.New("empty XLM amount")
	}
	if len(fraction) > 7 {
		return 0, errors.New("more than stroop precision")
	}
	for _, character := range whole + fraction {
		if character < '0' || character > '9' {
			return 0, errors.New("non-numeric XLM amount")
		}
	}
	if len(whole) > 0 && whole[0] == '0' && len(whole) > 1 {
		return 0, errors.New("leading zero in XLM amount")
	}
	digits := whole + fraction + strings.Repeat("0", 7-len(fraction))
	number, ok := new(big.Int).SetString(digits, 10)
	if !ok || !number.IsInt64() || number.Sign() <= 0 {
		return 0, errors.New("XLM amount out of range")
	}
	return entity.Stroops(number.Int64()), nil
}
