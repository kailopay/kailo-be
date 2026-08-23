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
	identity  Identity
	state     string
	nonce     string
	challenge string
	code      string
}

func (p *fakeProvider) AuthorizationURL(_ context.Context, state, nonce, challenge string) (string, error) {
	p.state, p.nonce, p.challenge = state, nonce, challenge
	return "https://accounts.google.com/o/oauth2/v2/auth", nil
}

func (p *fakeProvider) Exchange(_ context.Context, code, _, nonce string) (Identity, error) {
	p.code = code
	if nonce != p.nonce {
		return Identity{}, ErrInvalidIdentity
	}
	return p.identity, nil
}

type fakeAuthRepository struct {
	created      []LoginTransaction
	consumed     LoginTransaction
	transactions error

	profile           UserProfile
	session           SessionRecord
	authenticated     AuthenticatedUser
	revokedSession    bool
	updatedName       string
	developerEnabled  *bool
	avatarKey         string
	previousAvatarKey string
	storeErr          error

	registered    []RegisterRecord
	registerErr   error
	credentials   map[string]CredentialState
	usersByEmail  map[string]UserProfile
	challenges    []ChallengeRecord
	consumeErr    error
	consumedToken []byte
	consumedUser  string
	failureCount  int
	lockedUntil   *time.Time
	updatedHash   string
	verifiedUsers []string
	revokedUsers  []string
}

func (s *fakeAuthRepository) Create(_ context.Context, transaction LoginTransaction) error {
	s.created = append(s.created, transaction)
	return s.transactions
}

func (s *fakeAuthRepository) Consume(_ context.Context, _ []byte, _ time.Time) (LoginTransaction, error) {
	if s.transactions != nil {
		return LoginTransaction{}, s.transactions
	}
	return s.consumed, nil
}

func (s *fakeAuthRepository) UpsertIdentityAndCreateSession(_ context.Context, _ Identity, session SessionRecord) (UserProfile, error) {
	s.session = session
	return s.profile, s.storeErr
}

func (s *fakeAuthRepository) RevokeSession(context.Context, []byte, time.Time) error {
	s.revokedSession = true
	return s.storeErr
}

func (s *fakeAuthRepository) FindActiveSession(_ context.Context, _ []byte, _ time.Time) (AuthenticatedUser, error) {
	return s.authenticated, s.storeErr
}

func (s *fakeAuthRepository) UpdateDisplayName(_ context.Context, _ string, displayName string) (UserProfile, error) {
	s.updatedName = displayName
	s.profile.DisplayName = displayName
	return s.profile, s.storeErr
}

func (s *fakeAuthRepository) SetDeveloperMode(_ context.Context, _ string, enabled bool) (UserProfile, error) {
	s.developerEnabled = &enabled
	s.profile.DeveloperEnabled = enabled
	return s.profile, s.storeErr
}

func (s *fakeAuthRepository) FindProfile(context.Context, string) (UserProfile, error) {
	return s.profile, s.storeErr
}

func (s *fakeAuthRepository) ReplaceAvatarObjectKey(_ context.Context, _ string, objectKey string) (UserProfile, string, error) {
	s.avatarKey = objectKey
	s.profile.AvatarObjectKey = objectKey
	return s.profile, s.previousAvatarKey, s.storeErr
}

func (s *fakeAuthRepository) ClearAvatarObjectKey(_ context.Context, _ string) (UserProfile, string, error) {
	previous := s.avatarKey
	if previous == "" {
		previous = s.previousAvatarKey
	}
	s.avatarKey = ""
	s.profile.AvatarObjectKey = ""
	return s.profile, previous, s.storeErr
}

func (s *fakeAuthRepository) RevokeSessionsForUser(_ context.Context, userID string, _ time.Time) error {
	s.revokedUsers = append(s.revokedUsers, userID)
	return s.storeErr
}

func (s *fakeAuthRepository) CreateUserWithCredential(_ context.Context, record RegisterRecord) error {
	if s.registerErr != nil {
		return s.registerErr
	}
	s.registered = append(s.registered, record)
	if s.credentials == nil {
		s.credentials = map[string]CredentialState{}
	}
	s.credentials[record.Email] = CredentialState{
		UserID:       record.UserID,
		Email:        record.Email,
		DisplayName:  record.DisplayName,
		PasswordHash: record.PasswordHash,
	}
	return nil
}

func (s *fakeAuthRepository) FindCredentialByEmail(_ context.Context, email string) (CredentialState, bool, error) {
	state, ok := s.credentials[email]
	return state, ok, s.storeErr
}

func (s *fakeAuthRepository) FindCredentialByUserID(_ context.Context, userID string) (CredentialState, bool, error) {
	for _, state := range s.credentials {
		if state.UserID == userID {
			return state, true, nil
		}
	}
	return CredentialState{}, false, s.storeErr
}

func (s *fakeAuthRepository) FindUserByEmail(_ context.Context, email string) (UserProfile, bool, error) {
	profile, ok := s.usersByEmail[email]
	return profile, ok, s.storeErr
}

func (s *fakeAuthRepository) IncrementLoginFailures(context.Context, string, int, time.Duration, time.Time) error {
	s.failureCount++
	return nil
}

func (s *fakeAuthRepository) ResetLoginFailures(context.Context, string, time.Time) error {
	s.failureCount = 0
	return nil
}

func (s *fakeAuthRepository) UpdatePasswordHash(_ context.Context, _ string, passwordHash string, _ time.Time) error {
	s.updatedHash = passwordHash
	return s.storeErr
}

func (s *fakeAuthRepository) SetEmailVerified(_ context.Context, userID string, _ time.Time) (UserProfile, error) {
	s.verifiedUsers = append(s.verifiedUsers, userID)
	return s.profile, s.storeErr
}

func (s *fakeAuthRepository) CreateChallenge(_ context.Context, challenge ChallengeRecord) error {
	s.challenges = append(s.challenges, challenge)
	return s.storeErr
}

func (s *fakeAuthRepository) ConsumeChallenge(_ context.Context, tokenHash []byte, _ string, _ time.Time) (string, error) {
	if s.consumeErr != nil {
		return "", s.consumeErr
	}
	s.consumedToken = tokenHash
	return s.consumedUser, nil
}

type fakeMailer struct {
	verificationEmail string
	verificationLink  string
	resetEmail        string
	resetLink         string
	err               error
}

func (m *fakeMailer) SendEmailVerification(_ context.Context, email, link string) error {
	m.verificationEmail, m.verificationLink = email, link
	return m.err
}

func (m *fakeMailer) SendPasswordReset(_ context.Context, email, link string) error {
	m.resetEmail, m.resetLink = email, link
	return m.err
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

func testAuthUsecase(t *testing.T) (*AuthUsecase, *fakeProvider, *fakeAuthRepository, *fakeAvatarStore, *fakeMailer, fixedClock) {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, time.August, 19, 10, 0, 0, 0, time.UTC)}
	provider := &fakeProvider{identity: Identity{
		Provider:      ProviderGoogle,
		Subject:       "google-user-1",
		Email:         "user@example.com",
		EmailVerified: true,
		DisplayName:   "Test User",
	}}
	repository := &fakeAuthRepository{profile: UserProfile{ID: "user-id", DisplayName: "Test User", Email: "user@example.com", EmailVerified: true}}
	avatars := &fakeAvatarStore{}
	mailer := &fakeMailer{}
	key := []byte("01234567890123456789012345678901")
	service, err := NewAuthUsecase(AuthDependencies{
		Provider:   provider,
		Repository: repository,
		Avatars:    avatars,
		Mailer:     mailer,
	}, clock, AuthConfig{
		TransactionEncryptionKey: key,
		SessionHMACKey:           key,
		SessionAbsoluteLifetime:  8 * time.Hour,
		SessionIdleLifetime:      30 * time.Minute,
		TransactionLifetime:      10 * time.Minute,
		AvatarMaxBytes:           5 << 20,
		EmailLinkBaseURL:         "http://localhost:3001",
	})
	if err != nil {
		t.Fatalf("NewAuthUsecase() error = %v", err)
	}
	return service, provider, repository, avatars, mailer, clock
}

func TestAuthUsecaseBeginGoogleLoginCreatesPKCETransaction(t *testing.T) {
	service, provider, repository, _, _, _ := testAuthUsecase(t)

	redirect, err := service.BeginGoogleLogin(context.Background())
	if err != nil {
		t.Fatalf("BeginGoogleLogin() error = %v", err)
	}
	if redirect == "" || len(repository.created) != 1 {
		t.Fatalf("redirect/transaction = %q/%d, want a redirect and one transaction", redirect, len(repository.created))
	}
	if provider.state == "" || provider.nonce == "" || provider.challenge == "" {
		t.Fatal("provider did not receive generated authorization parameters")
	}
	if len(repository.created[0].StateHash) == 0 || len(repository.created[0].CodeVerifierCiphertext) == 0 {
		t.Fatal("transaction contains empty protected values")
	}
	if string(repository.created[0].StateHash) == provider.state {
		t.Fatal("state was persisted in plaintext")
	}
}

func TestAuthUsecaseCompleteGoogleLoginCreatesLocalSession(t *testing.T) {
	service, provider, repository, _, _, clock := testAuthUsecase(t)
	if _, err := service.BeginGoogleLogin(context.Background()); err != nil {
		t.Fatalf("BeginGoogleLogin() error = %v", err)
	}
	verifier := randomVerifier(t)
	repository.consumed = LoginTransaction{
		StateHash:              hashValue(service.config.SessionHMACKey, provider.state),
		NonceHash:              hashValue(service.config.SessionHMACKey, provider.nonce),
		CodeVerifierCiphertext: testEncryptCallbackData(t, service.config.TransactionEncryptionKey, provider.nonce, verifier),
		ExpiresAt:              clock.now.Add(service.config.TransactionLifetime),
	}

	result, err := service.CompleteGoogleLogin(context.Background(), "authorization-code", provider.state)
	if err != nil {
		t.Fatalf("CompleteGoogleLogin() error = %v", err)
	}
	if provider.code != "authorization-code" || result.RawToken == "" {
		t.Fatal("callback did not exchange the code and create a local token")
	}
	if len(repository.session.TokenHash) == 0 || string(repository.session.TokenHash) == result.RawToken {
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
		{name: "unverified email", identity: Identity{Provider: ProviderGoogle, Subject: "google-user", Email: "user@example.com"}, want: ErrInvalidIdentity},
		{name: "wrong provider", identity: Identity{Provider: "auth0", Subject: "auth0|user", Email: "user@example.com", EmailVerified: true}, want: ErrInvalidIdentity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, provider, repository, _, _, clock := testAuthUsecase(t)
			repository.transactions = tt.transactionErr
			if tt.transactionErr == nil {
				provider.identity = tt.identity
				if _, err := service.BeginGoogleLogin(context.Background()); err != nil {
					t.Fatalf("BeginGoogleLogin() error = %v", err)
				}
				verifier := randomVerifier(t)
				repository.consumed = LoginTransaction{
					StateHash:              hashValue(service.config.SessionHMACKey, provider.state),
					NonceHash:              hashValue(service.config.SessionHMACKey, provider.nonce),
					CodeVerifierCiphertext: testEncryptCallbackData(t, service.config.TransactionEncryptionKey, provider.nonce, verifier),
					ExpiresAt:              clock.now.Add(service.config.TransactionLifetime),
				}
			}
			if _, err := service.CompleteGoogleLogin(context.Background(), "code", "state"); !errors.Is(err, tt.want) {
				t.Fatalf("CompleteGoogleLogin() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAuthUsecaseRegisterCreatesCredentialAndVerificationChallenge(t *testing.T) {
	service, _, repository, _, mailer, _ := testAuthUsecase(t)

	profile, err := service.Register(context.Background(), RegisterInput{Email: " New@Example.com ", Password: "super-secret-1", DisplayName: "New User"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if len(repository.registered) != 1 {
		t.Fatalf("registered records = %d, want 1", len(repository.registered))
	}
	record := repository.registered[0]
	if record.Email != "new@example.com" || record.UserID != profile.ID {
		t.Fatalf("record email/user = %q/%q", record.Email, record.UserID)
	}
	if record.Challenge.Purpose != ChallengeEmailVerification || len(record.Challenge.TokenHash) == 0 {
		t.Fatalf("challenge = %+v", record.Challenge)
	}
	if record.Challenge.UserID != record.UserID {
		t.Fatalf("challenge user = %q, want %q", record.Challenge.UserID, record.UserID)
	}
	if mailer.verificationEmail != "new@example.com" || !strings.Contains(mailer.verificationLink, "/auth/verify-email?token=") {
		t.Fatalf("verification delivery = %q / %q", mailer.verificationEmail, mailer.verificationLink)
	}
	if match, _, err := verifyPassword(record.PasswordHash, "super-secret-1"); err != nil || !match {
		t.Fatalf("stored hash does not verify: match=%v err=%v", match, err)
	}
}

func TestAuthUsecaseRegisterRejectsDuplicatesWeakPasswordsAndBadEmails(t *testing.T) {
	service, _, repository, _, _, _ := testAuthUsecase(t)
	repository.registerErr = ErrEmailTaken
	if _, err := service.Register(context.Background(), RegisterInput{Email: "user@example.com", Password: "super-secret-1"}); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("Register() duplicate error = %v, want %v", err, ErrEmailTaken)
	}
	repository.registerErr = nil
	if _, err := service.Register(context.Background(), RegisterInput{Email: "user@example.com", Password: "short"}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("Register() weak password error = %v, want %v", err, ErrWeakPassword)
	}
	if _, err := service.Register(context.Background(), RegisterInput{Email: "not-an-email", Password: "super-secret-1"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Register() bad email error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestAuthUsecasePasswordLoginLifecycle(t *testing.T) {
	service, _, repository, _, _, clock := testAuthUsecase(t)
	hash, err := hashPassword("correct-password-10")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	repository.credentials = map[string]CredentialState{
		"user@example.com": {UserID: "user-id", Email: "user@example.com", DisplayName: "Test User", PasswordHash: hash, EmailVerified: true, Active: true},
	}

	result, err := service.LoginWithPassword(context.Background(), " USER@Example.COM ", "correct-password-10")
	if err != nil {
		t.Fatalf("LoginWithPassword() error = %v", err)
	}
	if result.RawToken == "" || len(repository.session.TokenHash) == 0 {
		t.Fatal("login did not mint a session")
	}
	if !result.ExpiresAt.Equal(clock.now.Add(8 * time.Hour)) {
		t.Fatalf("session expiry = %v", result.ExpiresAt)
	}

	if _, err := service.LoginWithPassword(context.Background(), "user@example.com", "wrong-password-99"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repository.failureCount != 1 {
		t.Fatalf("failure count = %d, want 1", repository.failureCount)
	}

	if _, err := service.LoginWithPassword(context.Background(), "unknown@example.com", "whatever-pass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown email error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestAuthUsecasePasswordLoginRejectsUnverifiedAndLockedAccounts(t *testing.T) {
	service, _, repository, _, _, clock := testAuthUsecase(t)
	hash, err := hashPassword("correct-password-10")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	lockedUntil := clock.now.Add(10 * time.Minute)
	repository.credentials = map[string]CredentialState{
		"unverified@example.com": {UserID: "user-1", Email: "unverified@example.com", PasswordHash: hash, EmailVerified: false, Active: true},
		"locked@example.com":     {UserID: "user-2", Email: "locked@example.com", PasswordHash: hash, EmailVerified: true, Active: true, LockedUntil: &lockedUntil},
		"disabled@example.com":   {UserID: "user-3", Email: "disabled@example.com", PasswordHash: hash, EmailVerified: true, Active: false},
	}

	if _, err := service.LoginWithPassword(context.Background(), "unverified@example.com", "correct-password-10"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("unverified error = %v, want %v", err, ErrEmailNotVerified)
	}
	if _, err := service.LoginWithPassword(context.Background(), "locked@example.com", "correct-password-10"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, err := service.LoginWithPassword(context.Background(), "disabled@example.com", "correct-password-10"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled error = %v, want %v", err, ErrUserDisabled)
	}
}

func TestAuthUsecaseVerifyEmailConsumesChallengeAndMarksVerified(t *testing.T) {
	service, _, repository, _, _, _ := testAuthUsecase(t)
	repository.consumedUser = "user-id"

	profile, err := service.VerifyEmail(context.Background(), " verification-token ")
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if len(repository.consumedToken) == 0 || string(repository.consumedToken) == "verification-token" {
		t.Fatal("verification token was consumed in plaintext")
	}
	if len(repository.verifiedUsers) != 1 || repository.verifiedUsers[0] != "user-id" {
		t.Fatalf("verified users = %#v", repository.verifiedUsers)
	}
	if profile.ID != "user-id" {
		t.Fatalf("profile = %+v", profile)
	}

	repository.consumeErr = ErrInvalidChallenge
	if _, err := service.VerifyEmail(context.Background(), "used-token"); !errors.Is(err, ErrInvalidChallenge) {
		t.Fatalf("replay error = %v, want %v", err, ErrInvalidChallenge)
	}
}

func TestAuthUsecasePasswordResetFlow(t *testing.T) {
	service, _, repository, _, mailer, _ := testAuthUsecase(t)
	repository.usersByEmail = map[string]UserProfile{"user@example.com": {ID: "user-id"}}

	if err := service.RequestPasswordReset(context.Background(), "user@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if len(repository.challenges) != 1 || repository.challenges[0].Purpose != ChallengePasswordReset {
		t.Fatalf("challenges = %+v", repository.challenges)
	}
	if mailer.resetEmail != "user@example.com" || !strings.Contains(mailer.resetLink, "/auth/reset-password?token=") {
		t.Fatalf("reset delivery = %q / %q", mailer.resetEmail, mailer.resetLink)
	}

	// Unknown emails succeed silently and never send mail.
	if err := service.RequestPasswordReset(context.Background(), "unknown@example.com"); err != nil {
		t.Fatalf("RequestPasswordReset(unknown) error = %v", err)
	}
	if len(repository.challenges) != 1 || mailer.resetEmail != "user@example.com" {
		t.Fatal("unknown email created a challenge or sent mail")
	}

	repository.consumedUser = "user-id"
	if err := service.ResetPassword(context.Background(), "reset-token", "fresh-password-1"); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if repository.updatedHash == "" {
		t.Fatal("ResetPassword() did not store a new hash")
	}
	if match, _, err := verifyPassword(repository.updatedHash, "fresh-password-1"); err != nil || !match {
		t.Fatalf("reset hash does not verify: match=%v err=%v", match, err)
	}
	if len(repository.revokedUsers) != 1 || repository.revokedUsers[0] != "user-id" {
		t.Fatalf("revoked users = %#v", repository.revokedUsers)
	}
}

func TestAuthUsecaseChangePasswordRequiresCurrentPassword(t *testing.T) {
	service, _, repository, _, _, _ := testAuthUsecase(t)
	hash, err := hashPassword("current-password-1")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	repository.credentials = map[string]CredentialState{
		"user@example.com": {UserID: "user-id", PasswordHash: hash, EmailVerified: true, Active: true},
	}

	if err := service.ChangePassword(context.Background(), "user-id", "wrong-current-00", "fresh-password-1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("ChangePassword() wrong current error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := service.ChangePassword(context.Background(), "user-id", "current-password-1", "fresh-password-1"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if len(repository.revokedUsers) != 1 {
		t.Fatalf("revoked users = %#v", repository.revokedUsers)
	}
}

func TestPasswordHashingRoundTrip(t *testing.T) {
	encoded, err := hashPassword("round-trip-password")
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	match, rehash, err := verifyPassword(encoded, "round-trip-password")
	if err != nil || !match || rehash {
		t.Fatalf("verify = %v/%v/%v, want true/false/nil", match, rehash, err)
	}
	match, _, err = verifyPassword(encoded, "other-password-123")
	if err != nil || match {
		t.Fatalf("wrong password verify = %v/%v, want false/nil", match, err)
	}
	if _, _, err := verifyPassword("$bcrypt$v=1$nope", "x"); err == nil {
		t.Fatal("unsupported hash format accepted")
	}
}

func TestAuthUsecaseLogoutAndAuthenticateHashTokens(t *testing.T) {
	service, _, repository, _, _, clock := testAuthUsecase(t)
	repository.authenticated = AuthenticatedUser{User: repository.profile, SessionID: "session-id", LastUsedAt: clock.now}

	if err := service.Logout(context.Background(), "raw-session-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if !repository.revokedSession {
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
	service, _, repository, _, _, _ := testAuthUsecase(t)

	profile, err := service.UpdateProfile(context.Background(), "user-id", UpdateProfileInput{DisplayName: "  New Name  "})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if repository.updatedName != "New Name" || profile.DisplayName != "New Name" {
		t.Fatalf("updated/profile display name = %q/%q, want New Name", repository.updatedName, profile.DisplayName)
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

func TestAuthUsecaseEnablesDeveloperModeWithoutChangingDisplayName(t *testing.T) {
	service, _, repository, _, _, _ := testAuthUsecase(t)
	enabled := true

	profile, err := service.UpdateProfile(context.Background(), "user-id", UpdateProfileInput{DeveloperEnabled: &enabled})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if repository.developerEnabled == nil || !*repository.developerEnabled || !profile.DeveloperEnabled {
		t.Fatalf("developer mode = %v, profile = %+v", repository.developerEnabled, profile)
	}
	if repository.updatedName != "" {
		t.Fatalf("display name unexpectedly updated to %q", repository.updatedName)
	}
}

func TestAuthUsecaseReplacesAvatarAndRemovesPreviousObject(t *testing.T) {
	service, _, repository, avatars, _, _ := testAuthUsecase(t)
	repository.previousAvatarKey = "avatars/user-id/old.png"

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
	service, _, repository, avatars, _, _ := testAuthUsecase(t)
	repository.storeErr = errors.New("database unavailable")

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
	service, _, repository, avatars, _, _ := testAuthUsecase(t)
	repository.profile.AvatarObjectKey = "avatars/user-id/avatar.webp"
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
	if avatars.openedKey != repository.profile.AvatarObjectKey || file.ContentType != "image/webp" {
		t.Fatalf("opened key/content type = %q/%q", avatars.openedKey, file.ContentType)
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
