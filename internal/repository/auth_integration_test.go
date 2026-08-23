package repository

import (
	"context"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
)

const testPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2g"

func newAuthIntegrationStore(t *testing.T) *AuthRepository {
	t.Helper()
	return newIntegrationStore(t).auth
}

func TestCreateUserWithCredentialCreatesCompleteAccount(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	err := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       "00000000-0000-4000-8000-0000000000b1",
		Email:        "user@example.com",
		DisplayName:  "User",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    "00000000-0000-4000-8000-0000000000b1",
			TokenHash: []byte("verification-hash"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(24 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("CreateUserWithCredential() error = %v", err)
	}

	state, found, err := store.FindCredentialByEmail(ctx, "user@example.com")
	if err != nil || !found {
		t.Fatalf("FindCredentialByEmail() = %v, %v; want found", found, err)
	}
	if state.UserID != "00000000-0000-4000-8000-0000000000b1" || state.PasswordHash != testPasswordHash {
		t.Fatalf("credential state = %+v", state)
	}
	if state.EmailVerified || !state.Active {
		t.Fatalf("new account must be unverified and active: %+v", state)
	}

	duplicate := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       "00000000-0000-4000-8000-0000000000b2",
		Email:        "user@example.com",
		DisplayName:  "Clone",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    "00000000-0000-4000-8000-0000000000b2",
			TokenHash: []byte("verification-hash-2"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(24 * time.Hour),
		},
	})
	if duplicate != usecase.ErrEmailTaken {
		t.Fatalf("duplicate email error = %v, want %v", duplicate, usecase.ErrEmailTaken)
	}
}

func TestChallengeConsumptionIsSingleUseAndPurposeBound(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       "00000000-0000-4000-8000-0000000000c1",
		Email:        "challenge@example.com",
		DisplayName:  "Challenge",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    "00000000-0000-4000-8000-0000000000c1",
			TokenHash: []byte("challenge-hash"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(time.Hour),
		},
	}); err != nil {
		t.Fatalf("CreateUserWithCredential() error = %v", err)
	}

	if _, err := store.ConsumeChallenge(ctx, []byte("challenge-hash"), usecase.ChallengePasswordReset, now); err != usecase.ErrInvalidChallenge {
		t.Fatalf("wrong purpose consume error = %v, want %v", err, usecase.ErrInvalidChallenge)
	}
	userID, err := store.ConsumeChallenge(ctx, []byte("challenge-hash"), usecase.ChallengeEmailVerification, now)
	if err != nil || userID != "00000000-0000-4000-8000-0000000000c1" {
		t.Fatalf("ConsumeChallenge() = %q, %v", userID, err)
	}
	if _, err := store.ConsumeChallenge(ctx, []byte("challenge-hash"), usecase.ChallengeEmailVerification, now); err != usecase.ErrInvalidChallenge {
		t.Fatalf("replay consume error = %v, want %v", err, usecase.ErrInvalidChallenge)
	}
	if _, err := store.ConsumeChallenge(ctx, []byte("unknown-hash"), usecase.ChallengeEmailVerification, now); err != usecase.ErrInvalidChallenge {
		t.Fatalf("unknown token error = %v, want %v", err, usecase.ErrInvalidChallenge)
	}
}

func TestLoginFailureLockoutLifecycle(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := "00000000-0000-4000-8000-0000000000d1"

	if err := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       userID,
		Email:        "locked@example.com",
		DisplayName:  "Locked",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    userID,
			TokenHash: []byte("locked-hash"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(time.Hour),
		},
	}); err != nil {
		t.Fatalf("CreateUserWithCredential() error = %v", err)
	}

	for i := 0; i < 9; i++ {
		if err := store.IncrementLoginFailures(ctx, userID, 10, 15*time.Minute, now); err != nil {
			t.Fatalf("IncrementLoginFailures() error = %v", err)
		}
	}
	state, _, _ := store.FindCredentialByEmail(ctx, "locked@example.com")
	if state.FailedCount != 9 || state.LockedUntil != nil {
		t.Fatalf("below threshold state = %+v", state)
	}

	if err := store.IncrementLoginFailures(ctx, userID, 10, 15*time.Minute, now); err != nil {
		t.Fatalf("IncrementLoginFailures() error = %v", err)
	}
	state, _, _ = store.FindCredentialByEmail(ctx, "locked@example.com")
	if state.FailedCount != 10 || state.LockedUntil == nil || !state.LockedUntil.After(now) {
		t.Fatalf("locked state = %+v", state)
	}

	if err := store.ResetLoginFailures(ctx, userID, now); err != nil {
		t.Fatalf("ResetLoginFailures() error = %v", err)
	}
	state, _, _ = store.FindCredentialByEmail(ctx, "locked@example.com")
	if state.FailedCount != 0 || state.LockedUntil != nil {
		t.Fatalf("reset state = %+v", state)
	}
}

func TestUpdatePasswordHashCreatesCredentialForOAuthOnlyUser(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// A Google-linked user with no password credential yet.
	profile, err := store.UpsertIdentityAndCreateSession(ctx, usecase.Identity{
		Provider: usecase.ProviderGoogle, Subject: "google-subject-1",
		Email: "google@example.com", EmailVerified: true, DisplayName: "Google User",
	}, usecase.SessionRecord{TokenHash: []byte("session-hash"), ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("UpsertIdentityAndCreateSession() error = %v", err)
	}

	if _, found, _ := store.FindCredentialByUserID(ctx, profile.ID); found {
		t.Fatal("google-only user must not have a credential yet")
	}
	if err := store.UpdatePasswordHash(ctx, profile.ID, testPasswordHash, now); err != nil {
		t.Fatalf("UpdatePasswordHash() error = %v", err)
	}
	state, found, err := store.FindCredentialByUserID(ctx, profile.ID)
	if err != nil || !found || state.PasswordHash != testPasswordHash {
		t.Fatalf("credential after upsert = %+v, %v, %v", state, found, err)
	}
}

func TestGoogleIdentityLinksToVerifiedEmailAccount(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := "00000000-0000-4000-8000-0000000000f1"

	if err := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       userID,
		Email:        "shared@example.com",
		DisplayName:  "Email User",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    userID,
			TokenHash: []byte("shared-hash"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(time.Hour),
		},
	}); err != nil {
		t.Fatalf("CreateUserWithCredential() error = %v", err)
	}
	if _, err := store.SetEmailVerified(ctx, userID, now); err != nil {
		t.Fatalf("SetEmailVerified() error = %v", err)
	}

	linked, err := store.UpsertIdentityAndCreateSession(ctx, usecase.Identity{
		Provider: usecase.ProviderGoogle, Subject: "google-subject-2",
		Email: "shared@example.com", EmailVerified: true, DisplayName: "Google Name",
	}, usecase.SessionRecord{TokenHash: []byte("linked-session"), ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("UpsertIdentityAndCreateSession() error = %v", err)
	}
	if linked.ID != userID {
		t.Fatalf("google identity linked to user %q, want existing %q", linked.ID, userID)
	}

	// A second google identity with an unregistered email creates a new user.
	fresh, err := store.UpsertIdentityAndCreateSession(ctx, usecase.Identity{
		Provider: usecase.ProviderGoogle, Subject: "google-subject-3",
		Email: "fresh@example.com", EmailVerified: true, DisplayName: "Fresh User",
	}, usecase.SessionRecord{TokenHash: []byte("fresh-session"), ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("UpsertIdentityAndCreateSession(fresh) error = %v", err)
	}
	if fresh.ID == userID || fresh.Email != "fresh@example.com" {
		t.Fatalf("fresh google user = %+v", fresh)
	}
}

func TestRevokeSessionsForUserAndVerification(t *testing.T) {
	store := newAuthIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	userID := "00000000-0000-4000-8000-000000000101"

	if err := store.CreateUserWithCredential(ctx, usecase.RegisterRecord{
		UserID:       userID,
		Email:        "sessions@example.com",
		DisplayName:  "Sessions",
		PasswordHash: testPasswordHash,
		Challenge: usecase.ChallengeRecord{
			UserID:    userID,
			TokenHash: []byte("sessions-hash"),
			Purpose:   usecase.ChallengeEmailVerification,
			ExpiresAt: now.Add(time.Hour),
		},
	}); err != nil {
		t.Fatalf("CreateUserWithCredential() error = %v", err)
	}

	verified, err := store.SetEmailVerified(ctx, userID, now)
	if err != nil {
		t.Fatalf("SetEmailVerified() error = %v", err)
	}
	if !verified.EmailVerified {
		t.Fatalf("verified profile = %+v", verified)
	}

	if _, err := store.UpsertIdentityAndCreateSession(ctx, usecase.Identity{
		Provider: usecase.ProviderCredentials, Subject: "sessions@example.com",
		Email: "sessions@example.com", EmailVerified: true, DisplayName: "Sessions",
	}, usecase.SessionRecord{TokenHash: []byte("session-to-revoke"), ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("UpsertIdentityAndCreateSession() error = %v", err)
	}
	if err := store.RevokeSessionsForUser(ctx, userID, now); err != nil {
		t.Fatalf("RevokeSessionsForUser() error = %v", err)
	}
	if _, err := store.FindActiveSession(ctx, []byte("session-to-revoke"), now.Add(time.Minute)); err != usecase.ErrInvalidSession {
		t.Fatalf("revoked session error = %v, want %v", err, usecase.ErrInvalidSession)
	}
}
