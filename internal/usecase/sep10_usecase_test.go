package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type sep10SignerFake struct {
	challenge     string
	challengeHash []byte
	account       string
	buildErr      error
	verifyErr     error
	validateErr   error
}

func (f *sep10SignerFake) BuildChallenge(string) (string, []byte, error) {
	if f.buildErr != nil {
		return "", nil, f.buildErr
	}
	return f.challenge, append([]byte(nil), f.challengeHash...), nil
}

func (f *sep10SignerFake) VerifyChallenge(string) (string, []byte, error) {
	if f.verifyErr != nil {
		return "", nil, f.verifyErr
	}
	return f.account, append([]byte(nil), f.challengeHash...), nil
}

func (f *sep10SignerFake) ValidateAccount(string) error { return f.validateErr }

type sep10ChallengeRepositoryFake struct {
	created    *entity.SEP10Challenge
	stored     entity.SEP10Challenge
	findErr    error
	consumeErr error
}

func (f *sep10ChallengeRepositoryFake) Create(_ context.Context, challenge entity.SEP10Challenge) error {
	f.created = &challenge
	f.stored = challenge
	return nil
}

func (f *sep10ChallengeRepositoryFake) FindByHash(_ context.Context, _ []byte) (entity.SEP10Challenge, error) {
	if f.findErr != nil {
		return entity.SEP10Challenge{}, f.findErr
	}
	return f.stored, nil
}

func (f *sep10ChallengeRepositoryFake) Consume(_ context.Context, _ string, now time.Time) error {
	if f.consumeErr != nil {
		return f.consumeErr
	}
	f.stored.ConsumedAt = &now
	return nil
}

func newSEP10TestService(t *testing.T, signer *sep10SignerFake, repository *sep10ChallengeRepositoryFake, now *time.Time, audience string) *SEP10Usecase {
	t.Helper()
	service, err := NewSEP10Usecase(SEP10Dependencies{Challenges: repository, Signer: signer}, SEP10ServiceConfig{
		Network:       "testnet",
		HomeDomain:    "anchor.example.com",
		ChallengeTTL:  5 * time.Minute,
		TokenTTL:      2 * time.Minute,
		TokenIssuer:   "kailopay",
		TokenAudience: audience,
		TokenSecret:   "sep10-test-secret",
		Now:           func() time.Time { return *now },
		NewID:         func() (string, error) { return "token-id-1", nil },
	})
	if err != nil {
		t.Fatalf("NewSEP10Usecase() error = %v", err)
	}
	return service
}

func TestSEP10ChallengeStoresOnlyHashAndReturnsTransaction(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	signer := &sep10SignerFake{challenge: "signed-server-challenge", challengeHash: []byte("challenge-hash")}
	repository := &sep10ChallengeRepositoryFake{}
	service := newSEP10TestService(t, signer, repository, &now, "sep10")

	view, err := service.Challenge(context.Background(), "GACCOUNT")
	if err != nil {
		t.Fatalf("Challenge() error = %v", err)
	}
	if view.Transaction != "signed-server-challenge" || view.ExpiresAt != now.Add(5*time.Minute) {
		t.Fatalf("challenge view = %+v", view)
	}
	if repository.created == nil || string(repository.created.ChallengeHash) != "challenge-hash" || repository.created.Account != "GACCOUNT" {
		t.Fatalf("stored challenge = %+v", repository.created)
	}
}

func TestSEP10ExchangeIssuesWalletBoundTokenAndRejectsReplay(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	signer := &sep10SignerFake{challengeHash: []byte("challenge-hash"), account: "GACCOUNT"}
	repository := &sep10ChallengeRepositoryFake{stored: entity.SEP10Challenge{
		ID: "challenge-id", ChallengeHash: []byte("challenge-hash"), Account: "GACCOUNT", Network: "testnet",
		ExpiresAt: now.Add(time.Minute), CreatedAt: now,
	}}
	service := newSEP10TestService(t, signer, repository, &now, "sep10")

	view, err := service.Exchange(context.Background(), "wallet-signed-challenge")
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	principal, err := service.Authenticate(context.Background(), view.Token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if principal.Account != "GACCOUNT" || principal.TokenID != "token-id-1" {
		t.Fatalf("principal = %+v", principal)
	}

	repository.consumeErr = ErrSEP10ChallengeConsumed
	if _, err := service.Exchange(context.Background(), "wallet-signed-challenge"); !errors.Is(err, ErrSEP10ChallengeConsumed) {
		t.Fatalf("replayed Exchange() error = %v, want %v", err, ErrSEP10ChallengeConsumed)
	}
}

func TestSEP10ExchangeRejectsExpiredConsumedInvalidAndWrongAccountChallenges(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*time.Time, *sep10ChallengeRepositoryFake, *sep10SignerFake)
		wantError error
	}{
		{name: "expired", mutate: func(now *time.Time, repository *sep10ChallengeRepositoryFake, _ *sep10SignerFake) {
			repository.stored.ExpiresAt = now.Add(-time.Second)
		}, wantError: ErrSEP10ChallengeExpired},
		{name: "consumed", mutate: func(now *time.Time, repository *sep10ChallengeRepositoryFake, _ *sep10SignerFake) {
			consumedAt := now.Add(-time.Second)
			repository.stored.ConsumedAt = &consumedAt
		}, wantError: ErrSEP10ChallengeConsumed},
		{name: "invalid wallet signature", mutate: func(_ *time.Time, _ *sep10ChallengeRepositoryFake, signer *sep10SignerFake) {
			signer.verifyErr = ErrSEP10InvalidChallenge
		}, wantError: ErrSEP10InvalidChallenge},
		{name: "wrong wallet account", mutate: func(_ *time.Time, _ *sep10ChallengeRepositoryFake, signer *sep10SignerFake) {
			signer.account = "GOTHER"
		}, wantError: ErrSEP10AccountMismatch},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
			signer := &sep10SignerFake{challengeHash: []byte("challenge-hash"), account: "GACCOUNT"}
			repository := &sep10ChallengeRepositoryFake{stored: entity.SEP10Challenge{
				ID: "challenge-id", ChallengeHash: []byte("challenge-hash"), Account: "GACCOUNT", Network: "testnet",
				ExpiresAt: now.Add(time.Minute), CreatedAt: now,
			}}
			testCase.mutate(&now, repository, signer)
			service := newSEP10TestService(t, signer, repository, &now, "sep10")
			if _, err := service.Exchange(context.Background(), "wallet-signed-challenge"); !errors.Is(err, testCase.wantError) {
				t.Fatalf("Exchange() error = %v, want %v", err, testCase.wantError)
			}
		})
	}
}

func TestSEP10AuthenticateRejectsExpiredAndWrongAudienceTokens(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	signer := &sep10SignerFake{challengeHash: []byte("challenge-hash"), account: "GACCOUNT"}
	repository := &sep10ChallengeRepositoryFake{stored: entity.SEP10Challenge{
		ID: "challenge-id", ChallengeHash: []byte("challenge-hash"), Account: "GACCOUNT", Network: "testnet", ExpiresAt: now.Add(time.Minute), CreatedAt: now,
	}}
	service := newSEP10TestService(t, signer, repository, &now, "sep10")
	token, err := service.Exchange(context.Background(), "wallet-signed-challenge")
	if err != nil {
		t.Fatalf("Exchange() error = %v", err)
	}
	now = now.Add(3 * time.Minute)
	if _, err := service.Authenticate(context.Background(), token.Token); err == nil {
		t.Fatal("Authenticate() error = nil for expired token")
	}

	now = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	repository.stored.ConsumedAt = nil
	other := newSEP10TestService(t, signer, repository, &now, "other-audience")
	otherToken, err := other.Exchange(context.Background(), "wallet-signed-challenge")
	if err != nil {
		t.Fatalf("other Exchange() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), otherToken.Token); err == nil {
		t.Fatal("Authenticate() error = nil for wrong audience")
	}
}
