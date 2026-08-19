package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeProvider struct {
	identity  Identity
	state     string
	nonce     string
	challenge string
	code      string
}

func (p *fakeProvider) AuthorizationURL(_ context.Context, state, nonce, challenge string) (string, error) {
	p.state, p.nonce, p.challenge = state, nonce, challenge
	return "https://tenant.example.com/authorize", nil
}

func (p *fakeProvider) Exchange(_ context.Context, code, _, nonce string) (Identity, error) {
	p.code = code
	if nonce != p.nonce {
		return Identity{}, ErrInvalidIdentity
	}
	return p.identity, nil
}

type fakeTransactionStore struct {
	created  []LoginTransaction
	consumed LoginTransaction
	err      error
}

func (s *fakeTransactionStore) Create(_ context.Context, tx LoginTransaction) error {
	s.created = append(s.created, tx)
	return s.err
}

func (s *fakeTransactionStore) Consume(_ context.Context, _ []byte, _ time.Time) (LoginTransaction, error) {
	if s.err != nil {
		return LoginTransaction{}, s.err
	}
	return s.consumed, nil
}

type fakeUserSessionStore struct {
	profile       UserProfile
	created       SessionRecord
	authenticated AuthenticatedUser
	revoked       bool
	err           error
}

func (s *fakeUserSessionStore) UpsertIdentityAndCreateSession(_ context.Context, _ Identity, session SessionRecord) (UserProfile, error) {
	s.created = session
	return s.profile, s.err
}

func (s *fakeUserSessionStore) RevokeSession(_ context.Context, _ []byte, _ time.Time) error {
	s.revoked = true
	return s.err
}

func (s *fakeUserSessionStore) FindActiveSession(_ context.Context, _ []byte, _ time.Time) (AuthenticatedUser, error) {
	return s.authenticated, s.err
}

func testAuthUsecase(t *testing.T) (*AuthUsecase, *fakeProvider, *fakeTransactionStore, *fakeUserSessionStore, fixedClock) {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, time.August, 19, 10, 0, 0, 0, time.UTC)}
	provider := &fakeProvider{identity: Identity{
		Provider:      ProviderAuth0,
		Subject:       "auth0|user-1",
		Email:         "user@example.com",
		EmailVerified: true,
		DisplayName:   "Test User",
	}}
	transactions := &fakeTransactionStore{}
	users := &fakeUserSessionStore{profile: UserProfile{ID: "user-id", DisplayName: "Test User", Email: "user@example.com", EmailVerified: true}}
	key := []byte("01234567890123456789012345678901")
	service, err := NewAuthUsecase(provider, transactions, users, clock, Config{
		TransactionEncryptionKey: key,
		SessionHMACKey:           key,
		SessionAbsoluteLifetime:  8 * time.Hour,
		SessionIdleLifetime:      30 * time.Minute,
		TransactionLifetime:      10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewAuthUsecase() error = %v", err)
	}
	return service, provider, transactions, users, clock
}

func TestAuthUsecaseBeginLoginCreatesPKCETransaction(t *testing.T) {
	service, provider, transactions, _, _ := testAuthUsecase(t)

	redirect, err := service.BeginLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginLogin() error = %v", err)
	}
	if redirect == "" || len(transactions.created) != 1 {
		t.Fatalf("redirect/transaction = %q/%d, want a redirect and one transaction", redirect, len(transactions.created))
	}
	if provider.state == "" || provider.nonce == "" || provider.challenge == "" {
		t.Fatal("provider did not receive generated authorization parameters")
	}
	if len(transactions.created[0].StateHash) == 0 || len(transactions.created[0].CodeVerifierCiphertext) == 0 {
		t.Fatal("transaction contains empty protected values")
	}
	if string(transactions.created[0].StateHash) == provider.state {
		t.Fatal("state was persisted in plaintext")
	}
}

func TestAuthUsecaseCompleteLoginCreatesLocalSession(t *testing.T) {
	service, provider, transactions, users, clock := testAuthUsecase(t)
	if _, err := service.BeginLogin(context.Background()); err != nil {
		t.Fatalf("BeginLogin() error = %v", err)
	}
	verifier := randomVerifier(t)
	transactions.consumed = LoginTransaction{
		StateHash:              hashValue(service.config.SessionHMACKey, provider.state),
		NonceHash:              hashValue(service.config.SessionHMACKey, provider.nonce),
		CodeVerifierCiphertext: testEncryptCallbackData(t, service.config.TransactionEncryptionKey, provider.nonce, verifier),
		ExpiresAt:              clock.now.Add(service.config.TransactionLifetime),
	}

	result, err := service.CompleteLogin(context.Background(), "authorization-code", provider.state)
	if err != nil {
		t.Fatalf("CompleteLogin() error = %v", err)
	}
	if provider.code != "authorization-code" || result.RawToken == "" {
		t.Fatal("callback did not exchange the code and create a local token")
	}
	if len(users.created.TokenHash) == 0 || string(users.created.TokenHash) == result.RawToken {
		t.Fatal("raw session token was persisted")
	}
	if !result.ExpiresAt.Equal(clock.now.Add(8 * time.Hour)) {
		t.Fatalf("session expiry = %v", result.ExpiresAt)
	}
}

func TestAuthUsecaseRejectsInvalidTransactionAndProviderIdentity(t *testing.T) {
	tests := []struct {
		name           string
		transactionErr error
		identity       Identity
		want           error
	}{
		{name: "invalid transaction", transactionErr: ErrInvalidTransaction, want: ErrInvalidTransaction},
		{name: "unverified email", identity: Identity{Provider: ProviderAuth0, Subject: "auth0|user", Email: "user@example.com"}, want: ErrInvalidIdentity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, provider, transactions, _, clock := testAuthUsecase(t)
			transactions.err = tt.transactionErr
			if tt.transactionErr == nil {
				provider.identity = tt.identity
				if _, err := service.BeginLogin(context.Background()); err != nil {
					t.Fatalf("BeginLogin() error = %v", err)
				}
				verifier := randomVerifier(t)
				transactions.consumed = LoginTransaction{
					StateHash:              hashValue(service.config.SessionHMACKey, provider.state),
					NonceHash:              hashValue(service.config.SessionHMACKey, provider.nonce),
					CodeVerifierCiphertext: testEncryptCallbackData(t, service.config.TransactionEncryptionKey, provider.nonce, verifier),
					ExpiresAt:              clock.now.Add(service.config.TransactionLifetime),
				}
			}
			if _, err := service.CompleteLogin(context.Background(), "code", "state"); !errors.Is(err, tt.want) {
				t.Fatalf("CompleteLogin() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAuthUsecaseLogoutAndAuthenticateHashTokens(t *testing.T) {
	service, _, _, users, clock := testAuthUsecase(t)
	users.authenticated = AuthenticatedUser{User: users.profile, SessionID: "session-id", LastUsedAt: clock.now}

	if err := service.Logout(context.Background(), "raw-session-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if !users.revoked {
		t.Fatal("Logout() did not revoke the session")
	}
	got, err := service.Authenticate(context.Background(), "raw-session-token")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.SessionID != "session-id" {
		t.Fatalf("session id = %q, want session-id", got.SessionID)
	}
}

func randomVerifier(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func testEncryptCallbackData(t *testing.T, key []byte, nonce, verifier string) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM() error = %v", err)
	}
	plaintext := []byte(nonce + "\x00" + verifier)
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return gcm.Seal(iv, iv, plaintext, nil)
}
