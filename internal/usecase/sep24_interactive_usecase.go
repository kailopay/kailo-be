package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrSEP24InteractiveNotFound       = errors.New("sep-24 interactive session not found")
	ErrSEP24InteractiveExpired        = errors.New("sep-24 interactive session expired")
	ErrSEP24InteractiveCompleted      = errors.New("sep-24 interactive session already completed")
	ErrSEP24InteractiveUserMismatch   = errors.New("sep-24 interactive session belongs to another user")
	ErrSEP24InteractiveWalletMismatch = errors.New("sep-24 interactive session belongs to another wallet")
	ErrSEP24InteractiveConflict       = errors.New("sep-24 interactive session conflicts with an existing request")
)

const (
	interactiveKYCStatusPending  = string(KYCStatusPending)
	interactiveKYCStatusApproved = string(KYCStatusApproved)
	interactiveKYCStatusExpired  = string(KYCStatusExpired)
)

// Sep24InteractiveRequest contains the request fields that must survive the
// browser hand-off. The wallet account is authenticated by SEP-10 and is not
// trusted merely because it was supplied in a form field.
type Sep24InteractiveRequest struct {
	Kind             string
	AssetCode        string
	AmountMinor      entity.IDR
	AssetAmount      string
	Account          string
	Memo             string
	PaymentMethod    entity.PaymentMethod
	DestinationToken string
	QuoteID          string
	IdempotencyKey   string
}

// Sep24InteractiveCompletion contains values collected by the interactive
// browser. DestinationToken is deliberately a sandbox reference for
// withdrawals; it is not parsed as a bank account by this service.
type Sep24InteractiveCompletion struct {
	DestinationToken string
}

type Sep24InteractiveView struct {
	ID            string
	URL           string
	Kind          string
	Status        string
	WalletAccount string
	LinkedUserID  string
	OrderID       string
	KYCStatus     string
	KYCRequired   bool
	ExpiresAt     time.Time
	Transaction   *Sep24TransactionView
}

type SEP24InteractiveRepository interface {
	Create(ctx context.Context, session entity.SEP24InteractiveSession) error
	FindByTransactionID(ctx context.Context, transactionID string) (entity.SEP24InteractiveSession, error)
	FindByBrowserTokenHash(ctx context.Context, transactionID string, tokenHash []byte) (entity.SEP24InteractiveSession, error)
	LinkUser(ctx context.Context, transactionID, walletAccount, userID, sessionID, kycStatus string, now time.Time) error
	Complete(ctx context.Context, transactionID, userID, orderID string, completedAt time.Time) error
}

type Sep24InteractiveTransferService interface {
	StartDeposit(ctx context.Context, command Sep24DepositCommand) (Sep24TransactionView, error)
	StartWithdraw(ctx context.Context, command Sep24WithdrawCommand) (Sep24TransactionView, error)
	GetTransaction(ctx context.Context, principal OrderPrincipal, transactionID string) (Sep24TransactionView, error)
}

type Sep24InteractiveKYC interface {
	KYCStatusReader
	StartInquiry(ctx context.Context, userID string) (KYCInquiryView, error)
}

// TestAutoApproveKYCStatusReader is a composition-root decorator used only
// when platform validation has accepted the explicit test-only flag. Keeping
// it as a small adapter avoids adding a request-controlled bypass to the
// normal KYC workflow.
type TestAutoApproveKYCStatusReader struct {
	Delegate KYCStatusReader
}

func (r TestAutoApproveKYCStatusReader) IsApproved(context.Context, string) (bool, error) {
	return true, nil
}

type Sep24InteractiveDependencies struct {
	Sessions  SEP24InteractiveRepository
	Transfers Sep24InteractiveTransferService
	KYC       KYCStatusReader
	Quotes    Sep38QuoteService
}

type Sep24InteractiveConfig struct {
	InteractiveURLBase string
	SessionTTL         time.Duration
	Environment        string
	Network            string
	TestAutoApproveKYC bool
	Now                func() time.Time
	NewID              func() (string, error)
	Random             io.Reader
}

type Sep24InteractiveService interface {
	Start(ctx context.Context, principal SEP10Principal, request Sep24InteractiveRequest) (Sep24InteractiveView, error)
	Load(ctx context.Context, principal SEP10Principal, transactionID string) (Sep24InteractiveView, error)
	LoadBrowser(ctx context.Context, transactionID, token string) (Sep24InteractiveView, error)
	Link(ctx context.Context, transactionID string, user AuthenticatedUser) (Sep24InteractiveView, error)
	Complete(ctx context.Context, transactionID string, user AuthenticatedUser, input Sep24InteractiveCompletion) (Sep24TransactionView, error)
}

type Sep24InteractiveUsecase struct {
	dependencies Sep24InteractiveDependencies
	config       Sep24InteractiveConfig
}

func NewSEP24InteractiveUsecase(dependencies Sep24InteractiveDependencies, config Sep24InteractiveConfig) (*Sep24InteractiveUsecase, error) {
	if dependencies.Sessions == nil || dependencies.Transfers == nil || config.Now == nil || config.NewID == nil ||
		strings.TrimSpace(config.InteractiveURLBase) == "" || config.SessionTTL <= 0 {
		return nil, errors.New("valid SEP-24 interactive dependencies and configuration are required")
	}
	if !config.TestAutoApproveKYC && dependencies.KYC == nil {
		return nil, errors.New("kyc status reader is required when automatic approval is disabled")
	}
	if config.TestAutoApproveKYC && (strings.ToLower(strings.TrimSpace(config.Environment)) != "test" ||
		strings.TrimSpace(config.Network) != StellarTestnetNetwork) {
		return nil, errors.New("automatic kyc approval requires test environment and Stellar testnet")
	}
	if config.Random == nil {
		config.Random = rand.Reader
	}
	return &Sep24InteractiveUsecase{dependencies: dependencies, config: config}, nil
}

// NewSep24InteractiveUsecase keeps the constructor spelling consistent with
// the existing SEP-24 usecase while retaining the acronym-safe export above.
func NewSep24InteractiveUsecase(dependencies Sep24InteractiveDependencies, config Sep24InteractiveConfig) (*Sep24InteractiveUsecase, error) {
	return NewSEP24InteractiveUsecase(dependencies, config)
}

func (s *Sep24InteractiveUsecase) Start(ctx context.Context, principal SEP10Principal, request Sep24InteractiveRequest) (Sep24InteractiveView, error) {
	account := strings.TrimSpace(principal.Account)
	if !validClassicAccount(account) {
		return Sep24InteractiveView{}, ErrSEP10InvalidAccount
	}
	normalized, err := normalizeInteractiveRequest(account, request)
	if err != nil {
		return Sep24InteractiveView{}, err
	}
	if normalized.QuoteID != "" {
		if s.dependencies.Quotes == nil {
			return Sep24InteractiveView{}, ErrSEP24InvalidRequest
		}
		quote, quoteErr := s.dependencies.Quotes.Get(ctx, principal, normalized.QuoteID)
		if quoteErr != nil {
			return Sep24InteractiveView{}, quoteErr
		}
		if err := applyInteractiveQuote(&normalized, quote); err != nil {
			return Sep24InteractiveView{}, err
		}
	}

	id, err := s.config.NewID()
	if err != nil {
		return Sep24InteractiveView{}, fmt.Errorf("generating SEP-24 interactive session id: %w", err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Sep24InteractiveView{}, ErrSEP24InteractiveConflict
	}
	token, err := s.browserToken()
	if err != nil {
		return Sep24InteractiveView{}, fmt.Errorf("generating SEP-24 browser token: %w", err)
	}
	now := s.config.Now().UTC()
	expiresAt := now.Add(s.config.SessionTTL)
	payload, err := json.Marshal(normalized)
	if err != nil {
		return Sep24InteractiveView{}, fmt.Errorf("encoding SEP-24 interactive request: %w", err)
	}
	requestHash := sha256.Sum256(payload)
	browserTokenHash := sha256.Sum256([]byte(token))
	transactionID := Sep24InteractiveID(normalized.Kind, id)
	session := entity.SEP24InteractiveSession{
		ID:               id,
		TransactionID:    transactionID,
		Kind:             normalized.Kind,
		WalletAccount:    account,
		BrowserTokenHash: append([]byte(nil), browserTokenHash[:]...),
		RequestHash:      append([]byte(nil), requestHash[:]...),
		RequestPayload:   payload,
		AssetCode:        normalized.AssetCode,
		AssetAmount:      optionalString(normalized.AssetAmount),
		FiatAmountMinor:  optionalIDR(normalized.AmountMinor),
		PaymentMethod:    optionalPaymentMethod(normalized.PaymentMethod),
		DestinationToken: []byte(normalized.DestinationToken),
		QuoteID:          optionalString(normalized.QuoteID),
		KYCStatus:        interactiveKYCStatusPending,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.dependencies.Sessions.Create(ctx, session); err != nil {
		if errors.Is(err, ErrSEP24InteractiveConflict) {
			return Sep24InteractiveView{}, err
		}
		return Sep24InteractiveView{}, fmt.Errorf("creating SEP-24 interactive session: %w", err)
	}
	return s.view(session, token), nil
}

func (s *Sep24InteractiveUsecase) Load(ctx context.Context, principal SEP10Principal, transactionID string) (Sep24InteractiveView, error) {
	account := strings.TrimSpace(principal.Account)
	if !validClassicAccount(account) {
		return Sep24InteractiveView{}, ErrSEP10InvalidAccount
	}
	session, err := s.loadSession(ctx, transactionID)
	if err != nil {
		return Sep24InteractiveView{}, err
	}
	if session.WalletAccount != account {
		return Sep24InteractiveView{}, ErrSEP24InteractiveWalletMismatch
	}
	return s.loadView(ctx, session)
}

func (s *Sep24InteractiveUsecase) LoadBrowser(ctx context.Context, transactionID, token string) (Sep24InteractiveView, error) {
	transactionID = strings.TrimSpace(transactionID)
	token = strings.TrimSpace(token)
	if transactionID == "" || token == "" {
		return Sep24InteractiveView{}, ErrSEP24InteractiveNotFound
	}
	tokenHash := sha256.Sum256([]byte(token))
	session, err := s.dependencies.Sessions.FindByBrowserTokenHash(ctx, transactionID, tokenHash[:])
	if err != nil {
		return Sep24InteractiveView{}, mapInteractiveRepositoryError(err)
	}
	if err := s.ensureActive(session); err != nil {
		return Sep24InteractiveView{}, err
	}
	return s.loadView(ctx, session)
}

func (s *Sep24InteractiveUsecase) Link(ctx context.Context, transactionID string, user AuthenticatedUser) (Sep24InteractiveView, error) {
	session, err := s.loadSession(ctx, transactionID)
	if err != nil {
		return Sep24InteractiveView{}, err
	}
	if err := s.ensureActive(session); err != nil {
		return Sep24InteractiveView{}, err
	}
	if _, err := NewRetailSessionOrderPrincipal(user); err != nil {
		return Sep24InteractiveView{}, ErrSEP24InteractiveUserMismatch
	}
	userID := strings.TrimSpace(user.User.ID)
	if session.LinkedUserID != nil && strings.TrimSpace(*session.LinkedUserID) != userID {
		return Sep24InteractiveView{}, ErrSEP24InteractiveUserMismatch
	}
	kycStatus := interactiveKYCStatusPending
	if s.config.TestAutoApproveKYC {
		kycStatus = interactiveKYCStatusApproved
	}
	if err := s.dependencies.Sessions.LinkUser(ctx, session.TransactionID, session.WalletAccount, userID, strings.TrimSpace(user.SessionID), kycStatus, s.config.Now().UTC()); err != nil {
		return Sep24InteractiveView{}, mapInteractiveRepositoryError(err)
	}
	session.LinkedUserID = stringPointer(userID)
	session.LinkedSessionID = stringPointer(strings.TrimSpace(user.SessionID))
	session.KYCStatus = kycStatus
	return s.loadView(ctx, session)
}

func (s *Sep24InteractiveUsecase) Complete(ctx context.Context, transactionID string, user AuthenticatedUser, input Sep24InteractiveCompletion) (Sep24TransactionView, error) {
	session, err := s.loadSession(ctx, transactionID)
	if err != nil {
		return Sep24TransactionView{}, err
	}
	if err := s.ensureActive(session); err != nil {
		if session.OrderID != nil && strings.TrimSpace(*session.OrderID) != "" {
			return s.loadCompleted(ctx, session, user)
		}
		return Sep24TransactionView{}, err
	}
	principal, err := NewRetailSessionOrderPrincipal(user)
	if err != nil {
		return Sep24TransactionView{}, ErrSEP24InteractiveUserMismatch
	}
	userID := strings.TrimSpace(user.User.ID)
	if session.LinkedUserID != nil && strings.TrimSpace(*session.LinkedUserID) != userID {
		return Sep24TransactionView{}, ErrSEP24InteractiveUserMismatch
	}
	if session.OrderID != nil && strings.TrimSpace(*session.OrderID) != "" {
		return s.dependencies.Transfers.GetTransaction(ctx, principal, session.TransactionID)
	}
	if session.LinkedUserID == nil {
		if _, err := s.Link(ctx, session.TransactionID, user); err != nil {
			return Sep24TransactionView{}, err
		}
	}
	approved, err := s.approved(ctx, userID)
	if err != nil {
		return Sep24TransactionView{}, fmt.Errorf("checking interactive kyc status: %w", err)
	}
	if !approved {
		return Sep24TransactionView{}, ErrKYCRequired
	}

	request, err := decodeInteractiveRequest(session.RequestPayload)
	if err != nil {
		return Sep24TransactionView{}, ErrSEP24InteractiveConflict
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		request.IdempotencyKey = session.TransactionID
	}
	if request.QuoteID != "" {
		if s.dependencies.Quotes == nil {
			return Sep24TransactionView{}, ErrSEP24InvalidRequest
		}
		if err := s.dependencies.Quotes.Consume(ctx, SEP10Principal{Account: session.WalletAccount}, request.QuoteID); err != nil {
			return Sep24TransactionView{}, err
		}
	}
	var view Sep24TransactionView
	switch session.Kind {
	case Sep24KindDeposit:
		view, err = s.dependencies.Transfers.StartDeposit(ctx, Sep24DepositCommand{
			Principal:      principal,
			WalletAccount:  session.WalletAccount,
			QuoteID:        request.QuoteID,
			IdempotencyKey: request.IdempotencyKey,
			AssetCode:      request.AssetCode,
			AmountMinor:    request.AmountMinor,
			Destination:    session.WalletAccount,
			Memo:           request.Memo,
			PaymentMethod:  request.PaymentMethod,
		})
	case Sep24KindWithdraw:
		destination := strings.TrimSpace(input.DestinationToken)
		if destination == "" {
			destination = strings.TrimSpace(request.DestinationToken)
		}
		if destination == "" {
			return Sep24TransactionView{}, ErrSEP24InvalidRequest
		}
		view, err = s.dependencies.Transfers.StartWithdraw(ctx, Sep24WithdrawCommand{
			Principal:        principal,
			WalletAccount:    session.WalletAccount,
			QuoteID:          request.QuoteID,
			IdempotencyKey:   request.IdempotencyKey,
			AssetCode:        request.AssetCode,
			AssetAmount:      request.AssetAmount,
			DestinationToken: destination,
		})
	default:
		return Sep24TransactionView{}, ErrSEP24InvalidRequest
	}
	if err != nil {
		return Sep24TransactionView{}, err
	}
	if err := s.dependencies.Sessions.Complete(ctx, session.TransactionID, userID, view.ID, s.config.Now().UTC()); err != nil {
		if errors.Is(err, ErrSEP24InteractiveCompleted) {
			return s.dependencies.Transfers.GetTransaction(ctx, principal, session.TransactionID)
		}
		return Sep24TransactionView{}, mapInteractiveRepositoryError(err)
	}
	return view, nil
}

func (s *Sep24InteractiveUsecase) approved(ctx context.Context, userID string) (bool, error) {
	if s.config.TestAutoApproveKYC {
		return true, nil
	}
	return s.dependencies.KYC.IsApproved(ctx, userID)
}

func (s *Sep24InteractiveUsecase) loadCompleted(ctx context.Context, session entity.SEP24InteractiveSession, user AuthenticatedUser) (Sep24TransactionView, error) {
	principal, err := NewRetailSessionOrderPrincipal(user)
	if err != nil {
		return Sep24TransactionView{}, ErrSEP24InteractiveUserMismatch
	}
	if session.LinkedUserID == nil || strings.TrimSpace(*session.LinkedUserID) != principal.OwnerUserID {
		return Sep24TransactionView{}, ErrSEP24InteractiveUserMismatch
	}
	return s.dependencies.Transfers.GetTransaction(ctx, principal, session.TransactionID)
}

func (s *Sep24InteractiveUsecase) loadSession(ctx context.Context, transactionID string) (entity.SEP24InteractiveSession, error) {
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" || len(transactionID) > 255 {
		return entity.SEP24InteractiveSession{}, ErrSEP24InteractiveNotFound
	}
	session, err := s.dependencies.Sessions.FindByTransactionID(ctx, transactionID)
	if err != nil {
		return entity.SEP24InteractiveSession{}, mapInteractiveRepositoryError(err)
	}
	return session, nil
}

func (s *Sep24InteractiveUsecase) loadView(ctx context.Context, session entity.SEP24InteractiveSession) (Sep24InteractiveView, error) {
	if err := s.ensureActive(session); err != nil && session.OrderID == nil {
		return Sep24InteractiveView{}, err
	}
	view := s.view(session, "")
	if session.OrderID != nil && strings.TrimSpace(*session.OrderID) != "" && session.LinkedUserID != nil && session.LinkedSessionID != nil {
		principal := OrderPrincipal{Kind: OrderPrincipalRetailSession, OwnerUserID: strings.TrimSpace(*session.LinkedUserID), SessionID: strings.TrimSpace(*session.LinkedSessionID)}
		transaction, err := s.dependencies.Transfers.GetTransaction(ctx, principal, session.TransactionID)
		if err != nil {
			return Sep24InteractiveView{}, err
		}
		view.Transaction = &transaction
		view.Status = transaction.Status
	}
	return view, nil
}

func (s *Sep24InteractiveUsecase) ensureActive(session entity.SEP24InteractiveSession) error {
	if session.CompletedAt != nil || session.OrderID != nil {
		return nil
	}
	if !s.config.Now().UTC().Before(session.ExpiresAt.UTC()) {
		return ErrSEP24InteractiveExpired
	}
	return nil
}

func (s *Sep24InteractiveUsecase) view(session entity.SEP24InteractiveSession, token string) Sep24InteractiveView {
	status := "pending_user_transfer_start"
	if session.OrderID != nil {
		status = "completed"
	}
	view := Sep24InteractiveView{
		ID:            session.TransactionID,
		URL:           strings.TrimRight(s.config.InteractiveURLBase, "/") + "/" + url.PathEscape(session.TransactionID),
		Kind:          session.Kind,
		Status:        status,
		WalletAccount: session.WalletAccount,
		KYCStatus:     session.KYCStatus,
		KYCRequired:   session.KYCStatus != interactiveKYCStatusApproved,
		ExpiresAt:     session.ExpiresAt,
	}
	if token != "" {
		view.URL += "?token=" + url.QueryEscape(token)
	}
	if session.LinkedUserID != nil {
		view.LinkedUserID = strings.TrimSpace(*session.LinkedUserID)
	}
	if session.OrderID != nil {
		view.OrderID = strings.TrimSpace(*session.OrderID)
	}
	return view
}

func (s *Sep24InteractiveUsecase) browserToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(s.config.Random, buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func normalizeInteractiveRequest(account string, request Sep24InteractiveRequest) (Sep24InteractiveRequest, error) {
	request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
	if request.Kind == "withdrawal" {
		request.Kind = Sep24KindWithdraw
	}
	request.AssetCode = strings.TrimSpace(request.AssetCode)
	request.Account = strings.TrimSpace(request.Account)
	request.AssetAmount = strings.TrimSpace(request.AssetAmount)
	request.Memo = strings.TrimSpace(request.Memo)
	request.DestinationToken = strings.TrimSpace(request.DestinationToken)
	request.QuoteID = strings.TrimSpace(request.QuoteID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.Kind != Sep24KindDeposit && request.Kind != Sep24KindWithdraw || request.AssetCode != NativeXLMAssetCode {
		return Sep24InteractiveRequest{}, ErrSEP24InvalidRequest
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = "sep24-interactive-" + account
	}
	if request.Kind == Sep24KindDeposit {
		if request.AmountMinor <= 0 && request.QuoteID == "" {
			return Sep24InteractiveRequest{}, ErrSEP24InvalidRequest
		}
		if request.Account != "" && request.Account != account {
			return Sep24InteractiveRequest{}, ErrSEP24InteractiveWalletMismatch
		}
		request.Account = account
	} else if request.AssetAmount == "" && request.QuoteID == "" {
		return Sep24InteractiveRequest{}, ErrSEP24InvalidRequest
	}
	return request, nil
}

func applyInteractiveQuote(request *Sep24InteractiveRequest, quote Sep38QuoteView) error {
	if request == nil || quote.QuoteID == "" {
		return ErrSEP24InvalidRequest
	}
	if request.Kind == Sep24KindDeposit {
		if quote.SellAsset != Sep38IDRAsset || quote.BuyAsset != Sep38XLMAsset {
			return ErrSEP24InvalidRequest
		}
		if request.AmountMinor == 0 {
			amount, err := parseSEP38IDR(quote.SellAmount)
			if err != nil {
				return ErrSEP24InvalidRequest
			}
			request.AmountMinor = entity.IDR(amount)
		} else if strconv.FormatInt(int64(request.AmountMinor), 10) != quote.SellAmount {
			return ErrSEP24InvalidRequest
		}
		return nil
	}
	if quote.SellAsset != Sep38XLMAsset || quote.BuyAsset != Sep38IDRAsset {
		return ErrSEP24InvalidRequest
	}
	if request.AssetAmount == "" {
		request.AssetAmount = quote.SellAmount
	}
	if request.AssetAmount != quote.SellAmount {
		return ErrSEP24InvalidRequest
	}
	return nil
}

func decodeInteractiveRequest(payload []byte) (Sep24InteractiveRequest, error) {
	var request Sep24InteractiveRequest
	if len(payload) == 0 {
		return Sep24InteractiveRequest{}, ErrSEP24InteractiveConflict
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return Sep24InteractiveRequest{}, err
	}
	return request, nil
}

func mapInteractiveRepositoryError(err error) error {
	switch {
	case errors.Is(err, ErrSEP24InteractiveNotFound), errors.Is(err, ErrSEP24TransactionNotFound):
		return ErrSEP24InteractiveNotFound
	case errors.Is(err, ErrSEP24InteractiveConflict):
		return ErrSEP24InteractiveConflict
	case errors.Is(err, ErrSEP24InteractiveCompleted):
		return ErrSEP24InteractiveCompleted
	default:
		return err
	}
}

func validClassicAccount(account string) bool {
	if len(account) < 2 || !strings.HasPrefix(account, "G") {
		return false
	}
	for _, character := range account {
		if !((character >= 'A' && character <= 'Z') || (character >= '2' && character <= '7')) {
			return false
		}
	}
	return true
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalIDR(value entity.IDR) *int64 {
	if value <= 0 {
		return nil
	}
	converted := int64(value)
	return &converted
}

func optionalPaymentMethod(value entity.PaymentMethod) *string {
	if value == entity.PaymentMethodUnknown || strings.TrimSpace(string(value)) == "" {
		return nil
	}
	converted := string(value)
	return &converted
}

func stringPointer(value string) *string { return &value }

// Sep24InteractiveID is intentionally opaque to wallets while remaining
// stable enough for the browser and later transaction mapping.
func Sep24InteractiveID(kind, id string) string {
	return strings.TrimSpace(kind) + "-interactive-" + strings.TrimSpace(id)
}
