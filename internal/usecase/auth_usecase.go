package usecase

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

type UserSessionStore interface {
	UpsertIdentityAndCreateSession(ctx context.Context, identity Identity, session SessionRecord) (UserProfile, error)
	RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error
	FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (AuthenticatedUser, error)
}

const ProviderAuth0 = "auth0"

var (
	ErrInvalidTransaction = errors.New("invalid authentication transaction")
	ErrInvalidIdentity    = errors.New("invalid authenticated identity")
	ErrProvider           = errors.New("authentication provider unavailable")
	ErrInvalidSession     = errors.New("invalid session")
	ErrUserDisabled       = errors.New("user is disabled")
)

type Identity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
}

type LoginTransaction struct {
	ID                     string
	StateHash              []byte
	NonceHash              []byte
	CodeVerifierCiphertext []byte
	ExpiresAt              time.Time
	ConsumedAt             *time.Time
}

type SessionRecord struct {
	TokenHash  []byte
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

type UserProfile struct {
	ID               string
	DisplayName      string
	Email            string
	EmailVerified    bool
	DeveloperEnabled bool
}

type AuthenticatedUser struct {
	User       UserProfile
	SessionID  string
	LastUsedAt time.Time
}

type SessionResult struct {
	RawToken  string
	User      UserProfile
	ExpiresAt time.Time
}

type Clock interface {
	Now() time.Time
}

type Config struct {
	TransactionEncryptionKey []byte
	SessionHMACKey           []byte
	SessionAbsoluteLifetime  time.Duration
	SessionIdleLifetime      time.Duration
	TransactionLifetime      time.Duration
}

type Provider interface {
	AuthorizationURL(ctx context.Context, state, nonce, codeChallenge string) (string, error)
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (Identity, error)
}

type TransactionStore interface {
	Create(ctx context.Context, transaction LoginTransaction) error
	Consume(ctx context.Context, stateHash []byte, now time.Time) (LoginTransaction, error)
}

const (
	randomValueBytes  = 32
	callbackSeparator = "\x00"
)

type AuthUsecase struct {
	provider     Provider
	transactions TransactionStore
	users        UserSessionStore
	clock        Clock
	config       Config
}

func NewAuthUsecase(provider Provider, transactions TransactionStore, users UserSessionStore, clock Clock, config Config) (*AuthUsecase, error) {
	if provider == nil || transactions == nil || users == nil {
		return nil, errors.New("auth usecase dependencies are required")
	}
	if clock == nil {
		clock = systemClock{}
	}
	if len(config.TransactionEncryptionKey) != 32 || len(config.SessionHMACKey) != 32 {
		return nil, errors.New("auth keys must be 32 bytes")
	}
	if config.SessionAbsoluteLifetime <= 0 || config.SessionIdleLifetime <= 0 || config.TransactionLifetime <= 0 {
		return nil, errors.New("auth durations must be positive")
	}
	if config.SessionIdleLifetime > config.SessionAbsoluteLifetime {
		return nil, errors.New("auth session idle lifetime cannot exceed absolute lifetime")
	}
	return &AuthUsecase{provider: provider, transactions: transactions, users: users, clock: clock, config: config}, nil
}

func (s *AuthUsecase) BeginLogin(ctx context.Context) (string, error) {
	state, err := randomString(randomValueBytes)
	if err != nil {
		return "", fmt.Errorf("generating login state: %w", err)
	}
	nonce, err := randomString(randomValueBytes)
	if err != nil {
		return "", fmt.Errorf("generating login nonce: %w", err)
	}
	verifier, err := randomString(randomValueBytes)
	if err != nil {
		return "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	ciphertext, err := encryptCallbackData(s.config.TransactionEncryptionKey, nonce, verifier)
	if err != nil {
		return "", fmt.Errorf("protecting login transaction: %w", err)
	}
	now := s.clock.Now().UTC()
	transaction := LoginTransaction{
		ID:                     newUUID(),
		StateHash:              hashValue(s.config.SessionHMACKey, state),
		NonceHash:              hashValue(s.config.SessionHMACKey, nonce),
		CodeVerifierCiphertext: ciphertext,
		ExpiresAt:              now.Add(s.config.TransactionLifetime),
	}
	if err := s.transactions.Create(ctx, transaction); err != nil {
		return "", fmt.Errorf("creating login transaction: %w", err)
	}
	redirectURL, err := s.provider.AuthorizationURL(ctx, state, nonce, pkceChallenge(verifier))
	if err != nil {
		return "", errors.Join(ErrProvider, err)
	}
	return redirectURL, nil
}

func (s *AuthUsecase) CompleteLogin(ctx context.Context, code, state string) (SessionResult, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" {
		return SessionResult{}, ErrInvalidTransaction
	}
	now := s.clock.Now().UTC()
	transaction, err := s.transactions.Consume(ctx, hashValue(s.config.SessionHMACKey, state), now)
	if err != nil {
		return SessionResult{}, err
	}
	nonce, verifier, err := decryptCallbackData(s.config.TransactionEncryptionKey, transaction.CodeVerifierCiphertext)
	if err != nil || subtle.ConstantTimeCompare(hashValue(s.config.SessionHMACKey, nonce), transaction.NonceHash) != 1 {
		return SessionResult{}, ErrInvalidTransaction
	}
	identity, err := s.provider.Exchange(ctx, code, verifier, nonce)
	if err != nil {
		return SessionResult{}, err
	}
	if err := validateIdentity(identity); err != nil {
		return SessionResult{}, err
	}
	rawToken, err := randomString(randomValueBytes)
	if err != nil {
		return SessionResult{}, fmt.Errorf("generating session token: %w", err)
	}
	expiresAt := now.Add(s.config.SessionAbsoluteLifetime)
	profile, err := s.users.UpsertIdentityAndCreateSession(ctx, identity, SessionRecord{
		TokenHash:  hashValue(s.config.SessionHMACKey, rawToken),
		ExpiresAt:  expiresAt,
		LastUsedAt: &now,
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("creating local session: %w", err)
	}
	return SessionResult{RawToken: rawToken, User: profile, ExpiresAt: expiresAt}, nil
}

func (s *AuthUsecase) Logout(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	if err := s.users.RevokeSession(ctx, hashValue(s.config.SessionHMACKey, rawToken), s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoking local session: %w", err)
	}
	return nil
}

func (s *AuthUsecase) Authenticate(ctx context.Context, rawToken string) (AuthenticatedUser, error) {
	if strings.TrimSpace(rawToken) == "" {
		return AuthenticatedUser{}, ErrInvalidSession
	}
	user, err := s.users.FindActiveSession(ctx, hashValue(s.config.SessionHMACKey, rawToken), s.clock.Now().UTC())
	if err != nil {
		return AuthenticatedUser{}, err
	}
	return user, nil
}

func validateIdentity(identity Identity) error {
	if identity.Provider != ProviderAuth0 || strings.TrimSpace(identity.Subject) == "" ||
		strings.TrimSpace(identity.Email) == "" || !identity.EmailVerified {
		return ErrInvalidIdentity
	}
	return nil
}

func randomString(size int) (string, error) {
	b := make([]byte, size)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashValue(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func encryptCallbackData(key []byte, nonce, verifier string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := cryptorand.Read(iv); err != nil {
		return nil, err
	}
	plaintext := []byte(nonce + callbackSeparator + verifier)
	return gcm.Seal(iv, iv, plaintext, nil), nil
}

func decryptCallbackData(key, ciphertext []byte) (string, string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(ciphertext) < gcm.NonceSize() {
		return "", "", ErrInvalidTransaction
	}
	plaintext, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return "", "", ErrInvalidTransaction
	}
	nonce, verifier, ok := bytes.Cut(plaintext, []byte(callbackSeparator))
	if !ok || len(nonce) == 0 || len(verifier) == 0 {
		return "", "", ErrInvalidTransaction
	}
	return string(nonce), string(verifier), nil
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
