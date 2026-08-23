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
	"io"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

type AuthRepository interface {
	Create(ctx context.Context, transaction LoginTransaction) error
	Consume(ctx context.Context, stateHash []byte, now time.Time) (LoginTransaction, error)
	UpsertIdentityAndCreateSession(ctx context.Context, identity Identity, session SessionRecord) (UserProfile, error)
	RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error
	FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (AuthenticatedUser, error)
	UpdateDisplayName(ctx context.Context, userID, displayName string) (UserProfile, error)
	SetDeveloperMode(ctx context.Context, userID string, enabled bool) (UserProfile, error)
	FindProfile(ctx context.Context, userID string) (UserProfile, error)
	ReplaceAvatarObjectKey(ctx context.Context, userID, objectKey string) (UserProfile, string, error)
	ClearAvatarObjectKey(ctx context.Context, userID string) (UserProfile, string, error)
	RevokeSessionsForUser(ctx context.Context, userID string, now time.Time) error
	CreateUserWithCredential(ctx context.Context, record RegisterRecord) error
	FindCredentialByEmail(ctx context.Context, email string) (CredentialState, bool, error)
	FindCredentialByUserID(ctx context.Context, userID string) (CredentialState, bool, error)
	FindUserByEmail(ctx context.Context, email string) (UserProfile, bool, error)
	IncrementLoginFailures(ctx context.Context, userID string, maxFailures int, lockDuration time.Duration, now time.Time) error
	ResetLoginFailures(ctx context.Context, userID string, now time.Time) error
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string, now time.Time) error
	SetEmailVerified(ctx context.Context, userID string, now time.Time) (UserProfile, error)
	CreateChallenge(ctx context.Context, challenge ChallengeRecord) error
	ConsumeChallenge(ctx context.Context, tokenHash []byte, purpose string, now time.Time) (string, error)
}

type AvatarStore interface {
	Put(ctx context.Context, object AvatarObject) error
	Open(ctx context.Context, objectKey string) (AvatarFile, error)
	Delete(ctx context.Context, objectKey string) error
}

// EmailSender delivers authentication emails. The sandbox implementation
// logs the links; SMTP or a provider API can replace it behind this port.
type EmailSender interface {
	SendEmailVerification(ctx context.Context, email, link string) error
	SendPasswordReset(ctx context.Context, email, link string) error
}

const (
	ProviderGoogle      = "google"
	ProviderCredentials = "email"
)

const (
	emailVerificationLifetime = 24 * time.Hour
	passwordResetLifetime     = time.Hour
	maxFailedLogins           = 10
	loginLockDuration         = 15 * time.Minute
	minPasswordLength         = 10
	maxPasswordLength         = 128

	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonSaltLen = 16
	argonKeyLen  = 32
)

const (
	ChallengeEmailVerification = "email_verification"
	ChallengePasswordReset     = "password_reset"
)

var (
	ErrInvalidTransaction   = errors.New("invalid authentication transaction")
	ErrInvalidIdentity      = errors.New("invalid authenticated identity")
	ErrProvider             = errors.New("authentication provider unavailable")
	ErrGoogleNotConfigured  = errors.New("google sign-in is not configured")
	ErrInvalidSession       = errors.New("invalid session")
	ErrUserDisabled         = errors.New("user is disabled")
	ErrInvalidProfile       = errors.New("invalid profile")
	ErrInvalidAvatar        = errors.New("invalid avatar")
	ErrAvatarNotFound       = errors.New("avatar not found")
	ErrInvalidPasswordReset = errors.New("invalid password reset request")
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrEmailTaken           = errors.New("email is already registered")
	ErrEmailNotVerified     = errors.New("email is not verified")
	ErrWeakPassword         = errors.New("password must be between 10 and 128 characters")
	ErrInvalidChallenge     = errors.New("invalid or expired token")
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

// RegisterRecord carries everything the repository needs to create a new
// credentials account in one transaction.
type RegisterRecord struct {
	UserID       string
	Email        string
	DisplayName  string
	PasswordHash string
	Challenge    ChallengeRecord
}

// ChallengeRecord is a single-use hashed token with a purpose and expiry.
type ChallengeRecord struct {
	UserID    string
	TokenHash []byte
	Purpose   string
	ExpiresAt time.Time
}

// CredentialState is the login-time view of a user's password credential.
type CredentialState struct {
	UserID        string
	Email         string
	DisplayName   string
	PasswordHash  string
	EmailVerified bool
	Active        bool
	FailedCount   int
	LockedUntil   *time.Time
}

type UserProfile struct {
	ID               string `json:"id"`
	DisplayName      string `json:"display_name"`
	Email            string `json:"email"`
	EmailVerified    bool   `json:"email_verified"`
	DeveloperEnabled bool   `json:"developer_enabled"`
	AvatarURL        string `json:"avatar_url,omitempty"`
	AvatarObjectKey  string `json:"-"`
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

type AuthConfig struct {
	TransactionEncryptionKey []byte
	SessionHMACKey           []byte
	SessionAbsoluteLifetime  time.Duration
	SessionIdleLifetime      time.Duration
	TransactionLifetime      time.Duration
	AvatarMaxBytes           int64
	EmailLinkBaseURL         string
}

type AuthDependencies struct {
	// Provider is the Google OIDC adapter. It is optional: when nil, Google
	// sign-in endpoints report the service as not configured.
	Provider   Provider
	Repository AuthRepository
	Avatars    AvatarStore
	Mailer     EmailSender
}

type Provider interface {
	AuthorizationURL(ctx context.Context, state, nonce, codeChallenge string) (string, error)
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (Identity, error)
}

type UpdateProfileInput struct {
	DisplayName      string
	DeveloperEnabled *bool
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type AvatarUpload struct {
	Body        io.Reader
	Size        int64
	ContentType string
}

type AvatarObject struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

type AvatarFile struct {
	Body        io.ReadCloser
	Size        int64
	ContentType string
	ETag        string
}

const (
	randomValueBytes  = 32
	callbackSeparator = "\x00"
)

type AuthUsecase struct {
	provider   Provider
	repository AuthRepository
	avatars    AvatarStore
	mailer     EmailSender
	clock      Clock
	config     AuthConfig
	// dummyHash keeps unknown-email logins as slow as known-email logins.
	dummyHash string
}

func NewAuthUsecase(deps AuthDependencies, clock Clock, config AuthConfig) (*AuthUsecase, error) {
	if deps.Repository == nil || deps.Avatars == nil || deps.Mailer == nil {
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
	if config.AvatarMaxBytes <= 0 {
		return nil, errors.New("auth avatar maximum bytes must be positive")
	}
	if strings.TrimSpace(config.EmailLinkBaseURL) == "" {
		return nil, errors.New("auth email link base URL is required")
	}
	dummyHash, err := hashPassword("kailopay-timing-equalizer")
	if err != nil {
		return nil, fmt.Errorf("preparing credential timing equalizer: %w", err)
	}
	return &AuthUsecase{
		provider:   deps.Provider,
		repository: deps.Repository,
		avatars:    deps.Avatars,
		mailer:     deps.Mailer,
		clock:      clock,
		config:     config,
		dummyHash:  dummyHash,
	}, nil
}

// GoogleAvailable reports whether Google sign-in is configured.
func (s *AuthUsecase) GoogleAvailable() bool { return s.provider != nil }

func (s *AuthUsecase) BeginGoogleLogin(ctx context.Context) (string, error) {
	if s.provider == nil {
		return "", ErrGoogleNotConfigured
	}
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
	if err := s.repository.Create(ctx, transaction); err != nil {
		return "", fmt.Errorf("creating login transaction: %w", err)
	}
	redirectURL, err := s.provider.AuthorizationURL(ctx, state, nonce, pkceChallenge(verifier))
	if err != nil {
		return "", errors.Join(ErrProvider, err)
	}
	return redirectURL, nil
}

func (s *AuthUsecase) CompleteGoogleLogin(ctx context.Context, code, state string) (SessionResult, error) {
	if s.provider == nil {
		return SessionResult{}, ErrGoogleNotConfigured
	}
	if strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" {
		return SessionResult{}, ErrInvalidTransaction
	}
	now := s.clock.Now().UTC()
	transaction, err := s.repository.Consume(ctx, hashValue(s.config.SessionHMACKey, state), now)
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
	return s.createSession(ctx, identity, now)
}

// Register creates an unverified credentials account and issues a
// verification challenge. The verification link is delivered through the
// configured EmailSender.
func (s *AuthUsecase) Register(ctx context.Context, input RegisterInput) (UserProfile, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return UserProfile{}, err
	}
	if len(input.Password) < minPasswordLength || len(input.Password) > maxPasswordLength {
		return UserProfile{}, ErrWeakPassword
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = emailLocalPart(email)
	}
	if utf8.RuneCountInString(displayName) > 100 {
		return UserProfile{}, ErrInvalidProfile
	}
	now := s.clock.Now().UTC()
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return UserProfile{}, fmt.Errorf("hashing password: %w", err)
	}
	token, err := randomString(randomValueBytes)
	if err != nil {
		return UserProfile{}, fmt.Errorf("generating verification token: %w", err)
	}
	userID := newUUID()
	record := RegisterRecord{
		UserID:       userID,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		Challenge: ChallengeRecord{
			UserID:    userID,
			TokenHash: hashValue(s.config.SessionHMACKey, token),
			Purpose:   ChallengeEmailVerification,
			ExpiresAt: now.Add(emailVerificationLifetime),
		},
	}
	if err := s.repository.CreateUserWithCredential(ctx, record); err != nil {
		return UserProfile{}, err
	}
	if err := s.mailer.SendEmailVerification(ctx, email, s.verificationLink(token)); err != nil {
		return UserProfile{}, fmt.Errorf("delivering verification email: %w", err)
	}
	return UserProfile{ID: userID, DisplayName: displayName, Email: email}, nil
}

// LoginWithPassword verifies credentials and mints a session. Unknown emails
// run a dummy hash verification so response timing does not reveal whether
// the account exists.
func (s *AuthUsecase) LoginWithPassword(ctx context.Context, email, password string) (SessionResult, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return SessionResult{}, ErrInvalidCredentials
	}
	now := s.clock.Now().UTC()
	state, found, err := s.repository.FindCredentialByEmail(ctx, normalized)
	if err != nil {
		return SessionResult{}, fmt.Errorf("finding credential: %w", err)
	}
	if !found {
		_, _, _ = verifyPassword(s.dummyHash, password)
		return SessionResult{}, ErrInvalidCredentials
	}
	if state.LockedUntil != nil && state.LockedUntil.After(now) {
		return SessionResult{}, ErrInvalidCredentials
	}
	match, rehash, err := verifyPassword(state.PasswordHash, password)
	if err != nil {
		return SessionResult{}, fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		if incrementErr := s.repository.IncrementLoginFailures(ctx, state.UserID, maxFailedLogins, loginLockDuration, now); incrementErr != nil {
			return SessionResult{}, errors.Join(ErrInvalidCredentials, incrementErr)
		}
		return SessionResult{}, ErrInvalidCredentials
	}
	if !state.Active {
		return SessionResult{}, ErrUserDisabled
	}
	if !state.EmailVerified {
		return SessionResult{}, ErrEmailNotVerified
	}
	if rehash {
		if updateErr := s.repository.UpdatePasswordHash(ctx, state.UserID, mustHashPassword(password), now); updateErr != nil {
			return SessionResult{}, fmt.Errorf("refreshing password hash: %w", updateErr)
		}
	}
	identity := Identity{
		Provider:      ProviderCredentials,
		Subject:       normalized,
		Email:         normalized,
		EmailVerified: true,
		DisplayName:   state.DisplayName,
	}
	result, err := s.createSession(ctx, identity, now)
	if err != nil {
		return SessionResult{}, err
	}
	if resetErr := s.repository.ResetLoginFailures(ctx, state.UserID, now); resetErr != nil {
		return SessionResult{}, fmt.Errorf("resetting login failures: %w", resetErr)
	}
	return result, nil
}

// VerifyEmail consumes a verification challenge and marks the email verified.
func (s *AuthUsecase) VerifyEmail(ctx context.Context, token string) (UserProfile, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return UserProfile{}, ErrInvalidChallenge
	}
	userID, err := s.repository.ConsumeChallenge(ctx, hashValue(s.config.SessionHMACKey, token), ChallengeEmailVerification, s.clock.Now().UTC())
	if err != nil {
		return UserProfile{}, err
	}
	profile, err := s.repository.SetEmailVerified(ctx, userID, s.clock.Now().UTC())
	if err != nil {
		return UserProfile{}, err
	}
	return withAvatarURL(profile), nil
}

// ResendVerification re-issues a verification email. It reports success even
// when the email is unknown or already verified so responses do not reveal
// account existence.
func (s *AuthUsecase) ResendVerification(ctx context.Context, email string) error {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return ErrInvalidChallenge
	}
	state, found, err := s.repository.FindCredentialByEmail(ctx, normalized)
	if err != nil {
		return fmt.Errorf("finding credential: %w", err)
	}
	if !found || state.EmailVerified {
		return nil
	}
	token, err := randomString(randomValueBytes)
	if err != nil {
		return fmt.Errorf("generating verification token: %w", err)
	}
	now := s.clock.Now().UTC()
	if err := s.repository.CreateChallenge(ctx, ChallengeRecord{
		UserID:    state.UserID,
		TokenHash: hashValue(s.config.SessionHMACKey, token),
		Purpose:   ChallengeEmailVerification,
		ExpiresAt: now.Add(emailVerificationLifetime),
	}); err != nil {
		return fmt.Errorf("creating verification challenge: %w", err)
	}
	if err := s.mailer.SendEmailVerification(ctx, normalized, s.verificationLink(token)); err != nil {
		return fmt.Errorf("delivering verification email: %w", err)
	}
	return nil
}

// RequestPasswordReset issues a reset challenge when the email belongs to an
// account. It always reports success so responses do not reveal account
// existence.
func (s *AuthUsecase) RequestPasswordReset(ctx context.Context, email string) error {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return ErrInvalidPasswordReset
	}
	profile, found, err := s.repository.FindUserByEmail(ctx, normalized)
	if err != nil {
		return fmt.Errorf("finding reset account: %w", err)
	}
	if !found {
		return nil
	}
	token, err := randomString(randomValueBytes)
	if err != nil {
		return fmt.Errorf("generating reset token: %w", err)
	}
	now := s.clock.Now().UTC()
	if err := s.repository.CreateChallenge(ctx, ChallengeRecord{
		UserID:    profile.ID,
		TokenHash: hashValue(s.config.SessionHMACKey, token),
		Purpose:   ChallengePasswordReset,
		ExpiresAt: now.Add(passwordResetLifetime),
	}); err != nil {
		return fmt.Errorf("creating reset challenge: %w", err)
	}
	if err := s.mailer.SendPasswordReset(ctx, normalized, s.resetLink(token)); err != nil {
		return fmt.Errorf("delivering reset email: %w", err)
	}
	return nil
}

// ResetPassword consumes a reset challenge, stores a new password hash, and
// revokes every session for the account.
func (s *AuthUsecase) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < minPasswordLength || len(newPassword) > maxPasswordLength {
		return ErrWeakPassword
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrInvalidChallenge
	}
	now := s.clock.Now().UTC()
	userID, err := s.repository.ConsumeChallenge(ctx, hashValue(s.config.SessionHMACKey, token), ChallengePasswordReset, now)
	if err != nil {
		return err
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	if err := s.repository.UpdatePasswordHash(ctx, userID, passwordHash, now); err != nil {
		return err
	}
	if err := s.repository.RevokeSessionsForUser(ctx, userID, now); err != nil {
		return fmt.Errorf("revoking reset sessions: %w", err)
	}
	return nil
}

// ChangePassword replaces the password of the authenticated account and
// revokes all of its sessions, including the current one.
func (s *AuthUsecase) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < minPasswordLength || len(newPassword) > maxPasswordLength {
		return ErrWeakPassword
	}
	state, found, err := s.repository.FindCredentialByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("finding credential: %w", err)
	}
	if !found {
		return ErrInvalidCredentials
	}
	match, _, err := verifyPassword(state.PasswordHash, currentPassword)
	if err != nil {
		return fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		return ErrInvalidCredentials
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	now := s.clock.Now().UTC()
	if err := s.repository.UpdatePasswordHash(ctx, userID, passwordHash, now); err != nil {
		return err
	}
	if err := s.repository.RevokeSessionsForUser(ctx, userID, now); err != nil {
		return fmt.Errorf("revoking changed-password sessions: %w", err)
	}
	return nil
}

func (s *AuthUsecase) createSession(ctx context.Context, identity Identity, now time.Time) (SessionResult, error) {
	rawToken, err := randomString(randomValueBytes)
	if err != nil {
		return SessionResult{}, fmt.Errorf("generating session token: %w", err)
	}
	expiresAt := now.Add(s.config.SessionAbsoluteLifetime)
	profile, err := s.repository.UpsertIdentityAndCreateSession(ctx, identity, SessionRecord{
		TokenHash:  hashValue(s.config.SessionHMACKey, rawToken),
		ExpiresAt:  expiresAt,
		LastUsedAt: &now,
	})
	if err != nil {
		return SessionResult{}, fmt.Errorf("creating local session: %w", err)
	}
	return SessionResult{RawToken: rawToken, User: withAvatarURL(profile), ExpiresAt: expiresAt}, nil
}

func (s *AuthUsecase) Logout(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return nil
	}
	if err := s.repository.RevokeSession(ctx, hashValue(s.config.SessionHMACKey, rawToken), s.clock.Now().UTC()); err != nil {
		return fmt.Errorf("revoking local session: %w", err)
	}
	return nil
}

func (s *AuthUsecase) Authenticate(ctx context.Context, rawToken string) (AuthenticatedUser, error) {
	if strings.TrimSpace(rawToken) == "" {
		return AuthenticatedUser{}, ErrInvalidSession
	}
	user, err := s.repository.FindActiveSession(ctx, hashValue(s.config.SessionHMACKey, rawToken), s.clock.Now().UTC())
	if err != nil {
		return AuthenticatedUser{}, err
	}
	user.User = withAvatarURL(user.User)
	return user, nil
}

func (s *AuthUsecase) UpdateProfile(ctx context.Context, userID string, input UpdateProfileInput) (UserProfile, error) {
	displayName := strings.TrimSpace(input.DisplayName)
	if strings.TrimSpace(userID) == "" || (displayName == "" && input.DeveloperEnabled == nil) || utf8.RuneCountInString(displayName) > 100 {
		return UserProfile{}, ErrInvalidProfile
	}
	var profile UserProfile
	var err error
	if displayName != "" {
		profile, err = s.repository.UpdateDisplayName(ctx, userID, displayName)
		if err != nil {
			return UserProfile{}, fmt.Errorf("updating profile: %w", err)
		}
	}
	if input.DeveloperEnabled != nil {
		profile, err = s.repository.SetDeveloperMode(ctx, userID, *input.DeveloperEnabled)
		if err != nil {
			return UserProfile{}, fmt.Errorf("updating developer mode: %w", err)
		}
	}
	return withAvatarURL(profile), nil
}

func (s *AuthUsecase) UpdateAvatar(ctx context.Context, userID string, upload AvatarUpload) (UserProfile, error) {
	extension, ok := avatarExtension(upload.ContentType)
	if strings.TrimSpace(userID) == "" || upload.Body == nil || upload.Size <= 0 ||
		upload.Size > s.config.AvatarMaxBytes || !ok {
		return UserProfile{}, ErrInvalidAvatar
	}
	objectID := newUUID()
	if objectID == "" {
		return UserProfile{}, errors.New("generating avatar object id")
	}
	objectKey := fmt.Sprintf("avatars/%s/%s.%s", userID, objectID, extension)
	if err := s.avatars.Put(ctx, AvatarObject{
		Key:         objectKey,
		Body:        upload.Body,
		Size:        upload.Size,
		ContentType: upload.ContentType,
	}); err != nil {
		return UserProfile{}, fmt.Errorf("storing avatar: %w", err)
	}
	profile, previousKey, err := s.repository.ReplaceAvatarObjectKey(ctx, userID, objectKey)
	if err != nil {
		cleanupErr := s.avatars.Delete(ctx, objectKey)
		return UserProfile{}, errors.Join(fmt.Errorf("updating avatar profile: %w", err), cleanupErr)
	}
	if previousKey != "" && previousKey != objectKey {
		if err := s.avatars.Delete(ctx, previousKey); err != nil {
			return UserProfile{}, fmt.Errorf("removing previous avatar: %w", err)
		}
	}
	return withAvatarURL(profile), nil
}

func (s *AuthUsecase) OpenAvatar(ctx context.Context, userID string) (AvatarFile, error) {
	if strings.TrimSpace(userID) == "" {
		return AvatarFile{}, ErrAvatarNotFound
	}
	profile, err := s.repository.FindProfile(ctx, userID)
	if err != nil {
		return AvatarFile{}, fmt.Errorf("finding avatar profile: %w", err)
	}
	if profile.AvatarObjectKey == "" {
		return AvatarFile{}, ErrAvatarNotFound
	}
	file, err := s.avatars.Open(ctx, profile.AvatarObjectKey)
	if err != nil {
		return AvatarFile{}, fmt.Errorf("opening avatar: %w", err)
	}
	return file, nil
}

func (s *AuthUsecase) DeleteAvatar(ctx context.Context, userID string) (UserProfile, error) {
	if strings.TrimSpace(userID) == "" {
		return UserProfile{}, ErrInvalidProfile
	}
	profile, objectKey, err := s.repository.ClearAvatarObjectKey(ctx, userID)
	if err != nil {
		return UserProfile{}, fmt.Errorf("clearing avatar profile: %w", err)
	}
	if objectKey != "" {
		if err := s.avatars.Delete(ctx, objectKey); err != nil {
			return UserProfile{}, fmt.Errorf("removing avatar: %w", err)
		}
	}
	return withAvatarURL(profile), nil
}

func (s *AuthUsecase) verificationLink(token string) string {
	return strings.TrimRight(s.config.EmailLinkBaseURL, "/") + "/auth/verify-email?token=" + token
}

func (s *AuthUsecase) resetLink(token string) string {
	return strings.TrimRight(s.config.EmailLinkBaseURL, "/") + "/auth/reset-password?token=" + token
}

func avatarExtension(contentType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return "jpg", true
	case "image/png":
		return "png", true
	case "image/webp":
		return "webp", true
	default:
		return "", false
	}
}

func withAvatarURL(profile UserProfile) UserProfile {
	if profile.AvatarObjectKey != "" {
		profile.AvatarURL = "/auth/me/avatar"
	} else {
		profile.AvatarURL = ""
	}
	return profile
}

func validateIdentity(identity Identity) error {
	if identity.Provider != ProviderGoogle || strings.TrimSpace(identity.Subject) == "" ||
		strings.TrimSpace(identity.Email) == "" || !identity.EmailVerified {
		return ErrInvalidIdentity
	}
	return nil
}

func normalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized || len(normalized) > 254 {
		return "", ErrInvalidCredentials
	}
	return normalized, nil
}

func emailLocalPart(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return email
	}
	return local
}

// hashPassword produces the encoded Argon2id string stored in PostgreSQL:
// $argon2id$v=<version>$m=<memory>,t=<time>,p=<threads>$<salt>$<hash>.
func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := cryptorand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

func mustHashPassword(password string) string {
	encoded, err := hashPassword(password)
	if err != nil {
		return ""
	}
	return encoded
}

// verifyPassword checks a password against an encoded Argon2id hash using
// the parameters stored in the hash. rehash reports whether the stored
// parameters differ from the current policy and the hash should be rewritten.
func verifyPassword(encoded, password string) (match bool, rehash bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, false, errors.New("unsupported password hash format")
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil {
		return false, false, errors.New("invalid password hash version")
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, false, errors.New("invalid password hash parameters")
	}
	memory, err := strconv.Atoi(strings.TrimPrefix(params[0], "m="))
	if err != nil {
		return false, false, errors.New("invalid password hash memory")
	}
	rounds, err := strconv.Atoi(strings.TrimPrefix(params[1], "t="))
	if err != nil {
		return false, false, errors.New("invalid password hash time")
	}
	threads, err := strconv.Atoi(strings.TrimPrefix(params[2], "p="))
	if err != nil {
		return false, false, errors.New("invalid password hash threads")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, false, errors.New("invalid password hash salt")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, false, errors.New("invalid password hash digest")
	}
	got := argon2.IDKey([]byte(password), salt, uint32(rounds), uint32(memory), uint8(threads), uint32(len(want)))
	match = subtle.ConstantTimeCompare(got, want) == 1
	rehash = match && (version != argon2.Version || memory != argonMemory || rounds != argonTime || threads != argonThreads)
	return match, rehash, nil
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
