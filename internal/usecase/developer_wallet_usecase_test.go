package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type developerWalletRepositoryFake struct {
	view      DeveloperWalletView
	views     []DeveloperWalletView
	created   entity.DeveloperWallet
	userID    string
	revokedID string
	updatedID string
}

func (f *developerWalletRepositoryFake) Upsert(_ context.Context, wallet entity.DeveloperWallet) (DeveloperWalletView, error) {
	f.created = wallet
	return f.view, nil
}

func (f *developerWalletRepositoryFake) List(_ context.Context, userID string) ([]DeveloperWalletView, error) {
	f.userID = userID
	return f.views, nil
}

func (f *developerWalletRepositoryFake) Update(_ context.Context, userID, walletID string, _ DeveloperWalletUpdate, _ time.Time) (DeveloperWalletView, error) {
	f.userID = userID
	f.updatedID = walletID
	return f.view, nil
}

func (f *developerWalletRepositoryFake) Revoke(_ context.Context, userID, walletID string, _ time.Time) error {
	f.userID = userID
	f.revokedID = walletID
	return nil
}

type developerWalletSEP10Fake struct {
	principal SEP10Principal
	err       error
	token     string
}

func (f *developerWalletSEP10Fake) Authenticate(_ context.Context, token string) (SEP10Principal, error) {
	f.token = token
	return f.principal, f.err
}

func TestDeveloperWalletRegisterRequiresMatchingSEP10Proof(t *testing.T) {
	repository := &developerWalletRepositoryFake{view: DeveloperWalletView{ID: "wallet-1", UserID: "user-1", WalletAccount: "GACCOUNT"}}
	proof := &developerWalletSEP10Fake{principal: SEP10Principal{Account: "GACCOUNT", TokenID: "token-1"}}
	service, err := NewDeveloperWalletUsecase(repository, proof, func() (string, error) { return "wallet-1", nil }, func() time.Time {
		return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	}, StellarTestnetNetwork)
	if err != nil {
		t.Fatalf("NewDeveloperWalletUsecase() error = %v", err)
	}

	view, err := service.Register(context.Background(), "user-1", DeveloperWalletInput{
		ClientID: "client-1", Network: StellarTestnetNetwork, WalletAccount: "GACCOUNT", Label: "Treasury",
		IsPrimary: true, SEP10Token: "sep10-token",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if proof.token != "sep10-token" || repository.created.UserID != "user-1" || repository.created.ClientID == nil || *repository.created.ClientID != "client-1" ||
		repository.created.VerificationMethod != "sep10" || !repository.created.IsPrimary || view.ID != "wallet-1" {
		t.Fatalf("proof/storage/view = %q/%#v/%#v", proof.token, repository.created, view)
	}
}

func TestDeveloperWalletRegisterRejectsProofForAnotherAccount(t *testing.T) {
	repository := &developerWalletRepositoryFake{}
	proof := &developerWalletSEP10Fake{principal: SEP10Principal{Account: "GOTHER", TokenID: "token-1"}}
	service, err := NewDeveloperWalletUsecase(repository, proof, func() (string, error) { return "wallet-1", nil }, time.Now, StellarTestnetNetwork)
	if err != nil {
		t.Fatalf("NewDeveloperWalletUsecase() error = %v", err)
	}

	_, err = service.Register(context.Background(), "user-1", DeveloperWalletInput{
		Network: StellarTestnetNetwork, WalletAccount: "GACCOUNT", Label: "Treasury", SEP10Token: "sep10-token",
	})
	if !errors.Is(err, ErrDeveloperWalletAccountMismatch) {
		t.Fatalf("Register() error = %v, want %v", err, ErrDeveloperWalletAccountMismatch)
	}
	if repository.created.ID != "" {
		t.Fatal("repository was called for mismatched wallet proof")
	}
}

func TestDeveloperWalletListInitializesEmptySliceAndScopesUser(t *testing.T) {
	repository := &developerWalletRepositoryFake{}
	service, err := NewDeveloperWalletUsecase(repository, &developerWalletSEP10Fake{}, func() (string, error) { return "wallet-1", nil }, time.Now, StellarTestnetNetwork)
	if err != nil {
		t.Fatalf("NewDeveloperWalletUsecase() error = %v", err)
	}
	views, err := service.List(context.Background(), "user-1")
	if err != nil || views == nil || len(views) != 0 || repository.userID != "user-1" {
		t.Fatalf("List() = %#v/%v, repository user = %q", views, err, repository.userID)
	}
}
