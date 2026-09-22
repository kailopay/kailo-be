package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

func TestSEP24InteractiveStartDoesNotCreateOrderAndStoresOnlyBrowserTokenHash(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	repository := &interactiveSessionRepositoryFake{}
	transfers := &interactiveTransfersFake{}
	service := newInteractiveService(t, repository, transfers, &interactiveKYCFake{})
	service.config.Now = func() time.Time { return now }

	view, err := service.Start(context.Background(), SEP10Principal{Account: "GACCOUNT"}, Sep24InteractiveRequest{
		Kind:           Sep24KindDeposit,
		AssetCode:      NativeXLMAssetCode,
		AmountMinor:    10000,
		IdempotencyKey: "request-1",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if transfers.depositCalls != 0 || transfers.withdrawCalls != 0 {
		t.Fatalf("Start() created an order before completion: %+v", transfers)
	}
	if len(repository.session.BrowserTokenHash) == 0 || strings.Contains(view.URL, string(repository.session.BrowserTokenHash)) {
		t.Fatalf("browser token was not stored as a hash: URL=%q hash=%x", view.URL, repository.session.BrowserTokenHash)
	}
	parsed, err := url.Parse(view.URL)
	if err != nil {
		t.Fatalf("parsing interactive URL: %v", err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatal("interactive URL has no browser token")
	}
	hash := sha256.Sum256([]byte(token))
	if string(hash[:]) != string(repository.session.BrowserTokenHash) {
		t.Fatalf("browser token hash mismatch")
	}
}

func TestSEP24InteractiveLinksUserRequiresKYCAndIsIdempotent(t *testing.T) {
	repository := &interactiveSessionRepositoryFake{}
	transfers := &interactiveTransfersFake{}
	kyc := &interactiveKYCFake{}
	service := newInteractiveService(t, repository, transfers, kyc)
	view, err := service.Start(context.Background(), SEP10Principal{Account: "GACCOUNT"}, Sep24InteractiveRequest{
		Kind: Sep24KindWithdraw, AssetCode: NativeXLMAssetCode, AssetAmount: "1.25", IdempotencyKey: "withdraw-1",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	user := AuthenticatedUser{User: UserProfile{ID: "user-1", EmailVerified: true}, SessionID: "session-1"}
	if _, err := service.Link(context.Background(), view.ID, user); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	other := AuthenticatedUser{User: UserProfile{ID: "user-2", EmailVerified: true}, SessionID: "session-2"}
	if _, err := service.Link(context.Background(), view.ID, other); !errors.Is(err, ErrSEP24InteractiveUserMismatch) {
		t.Fatalf("Link(other) error = %v, want user mismatch", err)
	}
	if _, err := service.Complete(context.Background(), view.ID, user, Sep24InteractiveCompletion{DestinationToken: "random-bank-reference"}); !errors.Is(err, ErrKYCRequired) {
		t.Fatalf("Complete() error = %v, want KYC required", err)
	}
	if transfers.withdrawCalls != 0 {
		t.Fatal("withdrawal order was created before KYC approval")
	}
	kyc.approved = true
	completed, err := service.Complete(context.Background(), view.ID, user, Sep24InteractiveCompletion{DestinationToken: "random-bank-reference"})
	if err != nil {
		t.Fatalf("approved Complete() error = %v", err)
	}
	if completed.ID == "" || transfers.withdrawCalls != 1 {
		t.Fatalf("approved completion did not create exactly one order: view=%+v calls=%d", completed, transfers.withdrawCalls)
	}
	replayed, err := service.Complete(context.Background(), view.ID, user, Sep24InteractiveCompletion{})
	if err != nil {
		t.Fatalf("duplicate Complete() error = %v", err)
	}
	if replayed.ID != completed.ID || transfers.withdrawCalls != 1 {
		t.Fatalf("duplicate completion was not idempotent: replay=%+v calls=%d", replayed, transfers.withdrawCalls)
	}
}

func TestSEP24InteractiveExpiredSessionCannotLoad(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	repository := &interactiveSessionRepositoryFake{}
	service := newInteractiveService(t, repository, &interactiveTransfersFake{}, &interactiveKYCFake{})
	service.config.Now = func() time.Time { return now }
	view, err := service.Start(context.Background(), SEP10Principal{Account: "GACCOUNT"}, Sep24InteractiveRequest{
		Kind: Sep24KindDeposit, AssetCode: NativeXLMAssetCode, AmountMinor: 1,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	service.config.Now = func() time.Time { return now.Add(31 * time.Minute) }
	parsed, _ := url.Parse(view.URL)
	_, err = service.LoadBrowser(context.Background(), view.ID, parsed.Query().Get("token"))
	if !errors.Is(err, ErrSEP24InteractiveExpired) {
		t.Fatalf("LoadBrowser() error = %v, want expired", err)
	}
}

func TestSEP24InteractiveTestAutoApprovalRequiresTestEnvironmentAndCompletes(t *testing.T) {
	_, err := NewSEP24InteractiveUsecase(Sep24InteractiveDependencies{
		Sessions:  &interactiveSessionRepositoryFake{},
		Transfers: &interactiveTransfersFake{},
	}, Sep24InteractiveConfig{
		InteractiveURLBase: "https://anchor.example/sep24/interactive",
		SessionTTL:         time.Minute,
		Environment:        "local",
		Network:            StellarTestnetNetwork,
		TestAutoApproveKYC: true,
		Now:                time.Now,
		NewID:              func() (string, error) { return "id", nil },
	})
	if err == nil {
		t.Fatal("NewSEP24InteractiveUsecase() accepted automatic approval outside test environment")
	}

	repository := &interactiveSessionRepositoryFake{}
	transfers := &interactiveTransfersFake{}
	service, err := NewSEP24InteractiveUsecase(Sep24InteractiveDependencies{Sessions: repository, Transfers: transfers}, Sep24InteractiveConfig{
		InteractiveURLBase: "https://anchor.example/sep24/interactive",
		SessionTTL:         time.Minute,
		Environment:        "test",
		Network:            StellarTestnetNetwork,
		TestAutoApproveKYC: true,
		Now:                time.Now,
		NewID:              func() (string, error) { return "id", nil },
	})
	if err != nil {
		t.Fatalf("NewSEP24InteractiveUsecase() error = %v", err)
	}
	view, err := service.Start(context.Background(), SEP10Principal{Account: "GACCOUNT"}, Sep24InteractiveRequest{Kind: Sep24KindDeposit, AssetCode: NativeXLMAssetCode, AmountMinor: 1})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	user := AuthenticatedUser{User: UserProfile{ID: "user-1", EmailVerified: true}, SessionID: "session-1"}
	if _, err := service.Complete(context.Background(), view.ID, user, Sep24InteractiveCompletion{}); err != nil {
		t.Fatalf("automatic approval Complete() error = %v", err)
	}
}

func newInteractiveService(t *testing.T, repository *interactiveSessionRepositoryFake, transfers *interactiveTransfersFake, kyc KYCStatusReader) *Sep24InteractiveUsecase {
	t.Helper()
	service, err := NewSEP24InteractiveUsecase(Sep24InteractiveDependencies{Sessions: repository, Transfers: transfers, KYC: kyc}, Sep24InteractiveConfig{
		InteractiveURLBase: "https://anchor.example/sep24/interactive",
		SessionTTL:         30 * time.Minute,
		Environment:        "test",
		Network:            StellarTestnetNetwork,
		Now:                time.Now,
		NewID:              func() (string, error) { return "session-id", nil },
	})
	if err != nil {
		t.Fatalf("NewSEP24InteractiveUsecase() error = %v", err)
	}
	return service
}

type interactiveSessionRepositoryFake struct {
	session entity.SEP24InteractiveSession
}

func (r *interactiveSessionRepositoryFake) Create(_ context.Context, session entity.SEP24InteractiveSession) error {
	if r.session.ID != "" {
		return ErrSEP24InteractiveConflict
	}
	r.session = session
	return nil
}

func (r *interactiveSessionRepositoryFake) FindByTransactionID(_ context.Context, transactionID string) (entity.SEP24InteractiveSession, error) {
	if r.session.TransactionID != transactionID {
		return entity.SEP24InteractiveSession{}, ErrSEP24InteractiveNotFound
	}
	return r.session, nil
}

func (r *interactiveSessionRepositoryFake) FindByBrowserTokenHash(_ context.Context, transactionID string, tokenHash []byte) (entity.SEP24InteractiveSession, error) {
	if r.session.TransactionID != transactionID || string(r.session.BrowserTokenHash) != string(tokenHash) {
		return entity.SEP24InteractiveSession{}, ErrSEP24InteractiveNotFound
	}
	return r.session, nil
}

func (r *interactiveSessionRepositoryFake) LinkUser(_ context.Context, transactionID, walletAccount, userID, sessionID, kycStatus string, now time.Time) error {
	if r.session.TransactionID != transactionID || r.session.WalletAccount != walletAccount {
		return ErrSEP24InteractiveNotFound
	}
	if r.session.LinkedUserID != nil && *r.session.LinkedUserID != userID {
		return ErrSEP24InteractiveUserMismatch
	}
	r.session.LinkedUserID = &userID
	r.session.LinkedSessionID = &sessionID
	r.session.KYCStatus = kycStatus
	r.session.UpdatedAt = now
	return nil
}

func (r *interactiveSessionRepositoryFake) Complete(_ context.Context, transactionID, userID, orderID string, completedAt time.Time) error {
	if r.session.TransactionID != transactionID {
		return ErrSEP24InteractiveNotFound
	}
	if r.session.OrderID != nil {
		return ErrSEP24InteractiveCompleted
	}
	if r.session.LinkedUserID == nil || *r.session.LinkedUserID != userID {
		return ErrSEP24InteractiveUserMismatch
	}
	r.session.OrderID = &orderID
	r.session.CompletedAt = &completedAt
	return nil
}

type interactiveKYCFake struct{ approved bool }

func (f *interactiveKYCFake) IsApproved(context.Context, string) (bool, error) {
	return f.approved, nil
}

type interactiveTransfersFake struct {
	depositCalls  int
	withdrawCalls int
	view          Sep24TransactionView
}

func (f *interactiveTransfersFake) StartDeposit(context.Context, Sep24DepositCommand) (Sep24TransactionView, error) {
	f.depositCalls++
	f.view = Sep24TransactionView{ID: "deposit-order", Kind: Sep24KindDeposit, Status: "pending_user_transfer_start"}
	return f.view, nil
}

func (f *interactiveTransfersFake) StartWithdraw(context.Context, Sep24WithdrawCommand) (Sep24TransactionView, error) {
	f.withdrawCalls++
	f.view = Sep24TransactionView{ID: "withdraw-order", Kind: Sep24KindWithdraw, Status: "pending_user_transfer_start"}
	return f.view, nil
}

func (f *interactiveTransfersFake) GetTransaction(context.Context, OrderPrincipal, string) (Sep24TransactionView, error) {
	if f.view.ID == "" {
		return Sep24TransactionView{}, ErrSEP24TransactionNotFound
	}
	return f.view, nil
}
