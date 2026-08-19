package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fakeProvider struct {
	identity           Identity
	state              string
	nonce              string
	challenge          string
	code               string
	passwordResetEmail string
	passwordResetErr   error
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

func (p *fakeProvider) RequestPasswordReset(_ context.Context, email string) error {
	p.passwordResetEmail = email
	return p.passwordResetErr
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
	profile           UserProfile
	created           SessionRecord
	authenticated     AuthenticatedUser
	revoked           bool
	revokedSubject    string
	updatedName       string
	avatarKey         string
	previousAvatarKey string
	err               error
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

func (s *fakeUserSessionStore) UpdateDisplayName(_ context.Context, _ string, displayName string) (UserProfile, error) {
	s.updatedName = displayName
	s.profile.DisplayName = displayName
	return s.profile, s.err
}

func (s *fakeUserSessionStore) FindProfile(_ context.Context, _ string) (UserProfile, error) {
	return s.profile, s.err
}

func (s *fakeUserSessionStore) ReplaceAvatarObjectKey(_ context.Context, _ string, objectKey string) (UserProfile, string, error) {
	s.avatarKey = objectKey
	s.profile.AvatarObjectKey = objectKey
	return s.profile, s.previousAvatarKey, s.err
}

func (s *fakeUserSessionStore) ClearAvatarObjectKey(_ context.Context, _ string) (UserProfile, string, error) {
	previous := s.avatarKey
	if previous == "" {
		previous = s.previousAvatarKey
	}
	s.avatarKey = ""
	s.profile.AvatarObjectKey = ""
	return s.profile, previous, s.err
}

func (s *fakeUserSessionStore) RevokeSessionsForIdentity(_ context.Context, _, subject string, _ time.Time) error {
	s.revokedSubject = subject
	return s.err
}

type fakeAvatarStore struct {
	putObject   AvatarObject
	openedKey   string
	opened      AvatarFile
	deletedKeys []string
	putErr      error
	openErr     error
	deleteErr   error
}

func (s *fakeAvatarStore) Put(_ context.Context, object AvatarObject) error {
	s.putObject = object
	return s.putErr
}

func (s *fakeAvatarStore) Open(_ context.Context, objectKey string) (AvatarFile, error) {
	s.openedKey = objectKey
	return s.opened, s.openErr
}

func (s *fakeAvatarStore) Delete(_ context.Context, objectKey string) error {
	s.deletedKeys = append(s.deletedKeys, objectKey)
	return s.deleteErr
}

func testAuthUsecase(t *testing.T) (*AuthUsecase, *fakeProvider, *fakeTransactionStore, *fakeUserSessionStore, *fakeAvatarStore, fixedClock) {
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
	avatars := &fakeAvatarStore{}
	key := []byte("01234567890123456789012345678901")
	service, err := NewAuthUsecase(Dependencies{
		Provider:       provider,
		PasswordReset:  provider,
		Transactions:   transactions,
		Sessions:       users,
		Profiles:       users,
		SessionRevoker: users,
		Avatars:        avatars,
	}, clock, Config{
		TransactionEncryptionKey: key,
		SessionHMACKey:           key,
		SessionAbsoluteLifetime:  8 * time.Hour,
		SessionIdleLifetime:      30 * time.Minute,
		TransactionLifetime:      10 * time.Minute,
		AvatarMaxBytes:           5 << 20,
	})
	if err != nil {
		t.Fatalf("NewAuthUsecase() error = %v", err)
	}
	return service, provider, transactions, users, avatars, clock
}

func TestAuthUsecaseBeginLoginCreatesPKCETransaction(t *testing.T) {
	service, provider, transactions, _, _, _ := testAuthUsecase(t)

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
	service, provider, transactions, users, _, clock := testAuthUsecase(t)
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
			service, provider, transactions, _, _, clock := testAuthUsecase(t)
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
	service, _, _, users, _, clock := testAuthUsecase(t)
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

func TestAuthUsecaseUpdatesOnlyTheAuthenticatedUsersDisplayName(t *testing.T) {
	service, _, _, users, _, _ := testAuthUsecase(t)

	profile, err := service.UpdateProfile(context.Background(), "user-id", UpdateProfileInput{DisplayName: "  New Name  "})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if users.updatedName != "New Name" || profile.DisplayName != "New Name" {
		t.Fatalf("updated/profile display name = %q/%q, want New Name", users.updatedName, profile.DisplayName)
	}
	if profile.Email != "user@example.com" {
		t.Fatalf("profile email = %q, want existing verified email", profile.Email)
	}
}

func TestAuthUsecaseRejectsInvalidDisplayNames(t *testing.T) {
	service, _, _, _, _, _ := testAuthUsecase(t)

	for _, displayName := range []string{"   ", strings.Repeat("a", 101)} {
		if _, err := service.UpdateProfile(context.Background(), "user-id", UpdateProfileInput{DisplayName: displayName}); !errors.Is(err, ErrInvalidProfile) {
			t.Fatalf("UpdateProfile(%q) error = %v, want %v", displayName, err, ErrInvalidProfile)
		}
	}
}

func TestAuthUsecaseRequestsPasswordResetWithoutLookingUpLocalUsers(t *testing.T) {
	service, provider, _, _, _, _ := testAuthUsecase(t)

	if err := service.RequestPasswordReset(context.Background(), "  USER@Example.COM "); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if provider.passwordResetEmail != "user@example.com" {
		t.Fatalf("password reset email = %q, want normalized email", provider.passwordResetEmail)
	}
}

func TestAuthUsecaseReplacesAvatarAndRemovesPreviousObject(t *testing.T) {
	service, _, _, users, avatars, _ := testAuthUsecase(t)
	users.previousAvatarKey = "avatars/user-id/old.png"

	profile, err := service.UpdateAvatar(context.Background(), "user-id", AvatarUpload{
		Body:        strings.NewReader("png-bytes"),
		Size:        9,
		ContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("UpdateAvatar() error = %v", err)
	}
	if !strings.HasPrefix(avatars.putObject.Key, "avatars/user-id/") || avatars.putObject.ContentType != "image/png" {
		t.Fatalf("stored avatar = %#v", avatars.putObject)
	}
	if profile.AvatarURL != "/auth/me/avatar" {
		t.Fatalf("avatar URL = %q", profile.AvatarURL)
	}
	if len(avatars.deletedKeys) != 1 || avatars.deletedKeys[0] != "avatars/user-id/old.png" {
		t.Fatalf("deleted keys = %#v", avatars.deletedKeys)
	}
}

func TestAuthUsecaseCleansUpNewAvatarWhenProfilePersistenceFails(t *testing.T) {
	service, _, _, users, avatars, _ := testAuthUsecase(t)
	users.err = errors.New("database unavailable")

	_, err := service.UpdateAvatar(context.Background(), "user-id", AvatarUpload{
		Body:        strings.NewReader("jpeg-bytes"),
		Size:        10,
		ContentType: "image/jpeg",
	})
	if err == nil {
		t.Fatal("UpdateAvatar() error = nil")
	}
	if len(avatars.deletedKeys) != 1 || avatars.deletedKeys[0] != avatars.putObject.Key {
		t.Fatalf("cleanup keys = %#v, stored key = %q", avatars.deletedKeys, avatars.putObject.Key)
	}
}

func TestAuthUsecaseOpensOnlyTheAuthenticatedUsersAvatar(t *testing.T) {
	service, _, _, users, avatars, _ := testAuthUsecase(t)
	users.profile.AvatarObjectKey = "avatars/user-id/avatar.webp"
	avatars.opened = AvatarFile{
		Body:        io.NopCloser(strings.NewReader("avatar")),
		Size:        6,
		ContentType: "image/webp",
		ETag:        "etag-value",
	}

	file, err := service.OpenAvatar(context.Background(), "user-id")
	if err != nil {
		t.Fatalf("OpenAvatar() error = %v", err)
	}
	defer file.Body.Close()
	if avatars.openedKey != users.profile.AvatarObjectKey || file.ContentType != "image/webp" {
		t.Fatalf("opened key/content type = %q/%q", avatars.openedKey, file.ContentType)
	}
}

func TestAuthUsecaseRevokesAllLocalSessionsAfterAuth0PasswordReset(t *testing.T) {
	service, _, _, users, _, _ := testAuthUsecase(t)

	if err := service.CompletePasswordReset(context.Background(), "auth0|user-1"); err != nil {
		t.Fatalf("CompletePasswordReset() error = %v", err)
	}
	if users.revokedSubject != "auth0|user-1" {
		t.Fatalf("revoked subject = %q", users.revokedSubject)
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
