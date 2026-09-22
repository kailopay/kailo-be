package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrInvalidDeveloperWallet         = errors.New("invalid developer wallet")
	ErrDeveloperWalletProofRequired   = errors.New("developer wallet SEP-10 proof is required")
	ErrDeveloperWalletAccountMismatch = errors.New("developer wallet SEP-10 account does not match")
	ErrDeveloperWalletNotFound        = errors.New("developer wallet not found")
)

type DeveloperWalletInput struct {
	ClientID      string
	Network       string
	WalletAccount string
	Label         string
	IsPrimary     bool
	SEP10Token    string
}

type DeveloperWalletUpdate struct {
	Label     *string
	IsPrimary *bool
}

type DeveloperWalletView struct {
	ID                 string
	UserID             string
	ClientID           string
	Network            string
	WalletAccount      string
	Label              string
	IsPrimary          bool
	VerificationMethod string
	VerifiedAt         time.Time
	Status             string
	RevokedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type DeveloperWalletRepository interface {
	Upsert(ctx context.Context, wallet entity.DeveloperWallet) (DeveloperWalletView, error)
	List(ctx context.Context, userID string) ([]DeveloperWalletView, error)
	Update(ctx context.Context, userID, walletID string, update DeveloperWalletUpdate, now time.Time) (DeveloperWalletView, error)
	Revoke(ctx context.Context, userID, walletID string, now time.Time) error
}

type DeveloperWalletProofVerifier interface {
	Authenticate(ctx context.Context, token string) (SEP10Principal, error)
}

type DeveloperWalletUsecase struct {
	repository DeveloperWalletRepository
	proof      DeveloperWalletProofVerifier
	newID      func() (string, error)
	now        func() time.Time
	network    string
}

func NewDeveloperWalletUsecase(repository DeveloperWalletRepository, proof DeveloperWalletProofVerifier, newID func() (string, error), now func() time.Time, network string) (*DeveloperWalletUsecase, error) {
	if repository == nil || proof == nil || newID == nil || now == nil || strings.TrimSpace(network) == "" {
		return nil, errors.New("developer wallet dependencies and network are required")
	}
	return &DeveloperWalletUsecase{repository: repository, proof: proof, newID: newID, now: now, network: network}, nil
}

func (s *DeveloperWalletUsecase) Register(ctx context.Context, userID string, input DeveloperWalletInput) (DeveloperWalletView, error) {
	userID = strings.TrimSpace(userID)
	input.ClientID = strings.TrimSpace(input.ClientID)
	input.Network = strings.TrimSpace(input.Network)
	input.WalletAccount = strings.TrimSpace(input.WalletAccount)
	input.Label = strings.TrimSpace(input.Label)
	input.SEP10Token = strings.TrimSpace(input.SEP10Token)
	if userID == "" || input.Network != s.network || input.WalletAccount == "" || len(input.WalletAccount) > 100 || input.Label == "" || len(input.Label) > 100 || input.SEP10Token == "" || len(input.SEP10Token) > 8192 {
		return DeveloperWalletView{}, ErrInvalidDeveloperWallet
	}
	principal, err := s.proof.Authenticate(ctx, input.SEP10Token)
	if err != nil {
		return DeveloperWalletView{}, ErrDeveloperWalletProofRequired
	}
	if principal.Account != input.WalletAccount {
		return DeveloperWalletView{}, ErrDeveloperWalletAccountMismatch
	}
	id, err := s.newID()
	if err != nil {
		return DeveloperWalletView{}, fmt.Errorf("generating developer wallet id: %w", err)
	}
	now := s.now().UTC()
	var clientID *string
	if input.ClientID != "" {
		clientID = &input.ClientID
	}
	view, err := s.repository.Upsert(ctx, entity.DeveloperWallet{
		ID: id, UserID: userID, ClientID: clientID, Network: input.Network,
		WalletAccount: input.WalletAccount, Label: input.Label, IsPrimary: input.IsPrimary,
		VerificationMethod: "sep10", VerifiedAt: now, Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return DeveloperWalletView{}, fmt.Errorf("storing developer wallet: %w", err)
	}
	return view, nil
}

func (s *DeveloperWalletUsecase) List(ctx context.Context, userID string) ([]DeveloperWalletView, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalidDeveloperWallet
	}
	views, err := s.repository.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing developer wallets: %w", err)
	}
	if views == nil {
		views = []DeveloperWalletView{}
	}
	return views, nil
}

func (s *DeveloperWalletUsecase) Update(ctx context.Context, userID, walletID string, update DeveloperWalletUpdate) (DeveloperWalletView, error) {
	userID = strings.TrimSpace(userID)
	walletID = strings.TrimSpace(walletID)
	if userID == "" || walletID == "" || update.Label != nil && (strings.TrimSpace(*update.Label) == "" || len(strings.TrimSpace(*update.Label)) > 100) {
		return DeveloperWalletView{}, ErrInvalidDeveloperWallet
	}
	if update.Label != nil {
		value := strings.TrimSpace(*update.Label)
		update.Label = &value
	}
	if update.Label == nil && update.IsPrimary == nil {
		return DeveloperWalletView{}, ErrInvalidDeveloperWallet
	}
	view, err := s.repository.Update(ctx, userID, walletID, update, s.now().UTC())
	if err != nil {
		return DeveloperWalletView{}, fmt.Errorf("updating developer wallet: %w", err)
	}
	return view, nil
}

func (s *DeveloperWalletUsecase) Revoke(ctx context.Context, userID, walletID string) error {
	userID = strings.TrimSpace(userID)
	walletID = strings.TrimSpace(walletID)
	if userID == "" || walletID == "" {
		return ErrInvalidDeveloperWallet
	}
	if err := s.repository.Revoke(ctx, userID, walletID, s.now().UTC()); err != nil {
		return fmt.Errorf("revoking developer wallet: %w", err)
	}
	return nil
}
