package repository

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/febry3/kailopay-be/internal/entity"
	"strings"
	"time"

	auth "github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const activeUserStatus = "active"

type AuthRepository struct {
	db           *gorm.DB
	idleLifetime time.Duration
}

func NewAuthRepository(db *gorm.DB, idleLifetime time.Duration) *AuthRepository {
	return &AuthRepository{db: db, idleLifetime: idleLifetime}
}

func (r *AuthRepository) Create(ctx context.Context, transaction auth.LoginTransaction) error {
	row := entity.AuthTransaction{
		ID:                     transaction.ID,
		StateHash:              append([]byte(nil), transaction.StateHash...),
		NonceHash:              append([]byte(nil), transaction.NonceHash...),
		CodeVerifierCiphertext: append([]byte(nil), transaction.CodeVerifierCiphertext...),
		ExpiresAt:              transaction.ExpiresAt.UTC(),
		ConsumedAt:             transaction.ConsumedAt,
		CreatedAt:              time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating auth transaction: %w", err)
	}
	return nil
}

func (r *AuthRepository) Consume(ctx context.Context, stateHash []byte, now time.Time) (auth.LoginTransaction, error) {
	var result auth.LoginTransaction
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.AuthTransaction
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("state_hash = ? AND consumed_at IS NULL AND expires_at > ?", stateHash, now.UTC()).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.ErrInvalidTransaction
		}
		if err != nil {
			return fmt.Errorf("finding auth transaction: %w", err)
		}
		consumedAt := now.UTC()
		updated := tx.Model(&entity.AuthTransaction{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", row.ID, now.UTC()).
			Updates(map[string]any{"consumed_at": consumedAt}).RowsAffected
		if updated != 1 {
			return auth.ErrInvalidTransaction
		}
		result = auth.LoginTransaction{
			ID:                     row.ID,
			StateHash:              append([]byte(nil), row.StateHash...),
			NonceHash:              append([]byte(nil), row.NonceHash...),
			CodeVerifierCiphertext: append([]byte(nil), row.CodeVerifierCiphertext...),
			ExpiresAt:              row.ExpiresAt,
			ConsumedAt:             &consumedAt,
		}
		return nil
	})
	if err != nil {
		return auth.LoginTransaction{}, err
	}
	return result, nil
}

func (r *AuthRepository) UpsertIdentityAndCreateSession(ctx context.Context, identity auth.Identity, session auth.SessionRecord) (auth.UserProfile, error) {
	var profile auth.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityRow entity.UserIdentity
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("provider = ? AND subject = ?", identity.Provider, identity.Subject).
			First(&identityRow).Error
		now := time.Now().UTC()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A verified email links a new OAuth identity to the existing
			// account instead of creating a second user.
			linkedUserID := ""
			if strings.TrimSpace(identity.Email) != "" {
				var existing entity.User
				linkErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("email = ? AND email_verified_at IS NOT NULL AND status = ?", identity.Email, activeUserStatus).
					First(&existing).Error
				if linkErr != nil && !errors.Is(linkErr, gorm.ErrRecordNotFound) {
					return fmt.Errorf("finding linking user: %w", linkErr)
				}
				if linkErr == nil {
					linkedUserID = existing.ID
				}
			}
			if linkedUserID == "" {
				userID := newUUID()
				if userID == "" {
					return errors.New("generating user id")
				}
				var email *string
				if strings.TrimSpace(identity.Email) != "" {
					value := strings.TrimSpace(identity.Email)
					email = &value
				}
				var verifiedAt *time.Time
				if identity.EmailVerified {
					verifiedAt = &now
				}
				user := entity.User{
					ID:              userID,
					Status:          activeUserStatus,
					DisplayName:     identity.DisplayName,
					Email:           email,
					EmailVerifiedAt: verifiedAt,
					CreatedAt:       now,
					UpdatedAt:       now,
				}
				if err := tx.Create(&user).Error; err != nil {
					return fmt.Errorf("creating user: %w", err)
				}
				linkedUserID = user.ID
			}
			identityRow = entity.UserIdentity{
				ID:          newUUID(),
				UserID:      linkedUserID,
				Provider:    identity.Provider,
				Subject:     identity.Subject,
				CreatedAt:   now,
				LastLoginAt: &now,
			}
			if identityRow.ID == "" {
				return errors.New("generating identity id")
			}
			if err := tx.Create(&identityRow).Error; err != nil {
				return fmt.Errorf("creating user identity: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("finding user identity: %w", err)
		} else {
			if err := tx.Model(&entity.UserIdentity{}).Where("id = ?", identityRow.ID).Updates(map[string]any{"last_login_at": now}).Error; err != nil {
				return fmt.Errorf("updating user identity: %w", err)
			}
		}

		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", identityRow.UserID).First(&user).Error; err != nil {
			return fmt.Errorf("finding user: %w", err)
		}
		if user.Status != activeUserStatus {
			return auth.ErrUserDisabled
		}
		if strings.TrimSpace(identity.DisplayName) != "" && identity.DisplayName != user.DisplayName {
			user.DisplayName = identity.DisplayName
		}
		if strings.TrimSpace(identity.Email) != "" {
			value := strings.TrimSpace(identity.Email)
			user.Email = &value
		}
		if identity.EmailVerified && user.EmailVerifiedAt == nil {
			user.EmailVerifiedAt = &now
		}
		user.UpdatedAt = now
		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("updating user profile: %w", err)
		}

		row := entity.RetailSession{
			ID:         newUUID(),
			UserID:     user.ID,
			TokenHash:  append([]byte(nil), session.TokenHash...),
			ExpiresAt:  session.ExpiresAt.UTC(),
			LastUsedAt: session.LastUsedAt,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if row.ID == "" {
			return errors.New("generating session id")
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("creating retail session: %w", err)
		}
		profile = userProfile(user)
		return nil
	})
	if err != nil {
		return auth.UserProfile{}, err
	}
	return profile, nil
}

func (r *AuthRepository) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	revokedAt := now.UTC()
	if err := r.db.WithContext(ctx).Model(&entity.RetailSession{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Updates(map[string]any{"revoked_at": revokedAt, "updated_at": revokedAt}).Error; err != nil {
		return fmt.Errorf("revoking retail session: %w", err)
	}
	return nil
}

func (r *AuthRepository) FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (auth.AuthenticatedUser, error) {
	var session entity.RetailSession
	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.AuthenticatedUser{}, auth.ErrInvalidSession
		}
		return auth.AuthenticatedUser{}, fmt.Errorf("finding retail session: %w", err)
	}
	now = now.UTC()
	lastActivity := session.CreatedAt
	if session.LastUsedAt != nil {
		lastActivity = *session.LastUsedAt
	}
	if r.idleLifetime <= 0 {
		return auth.AuthenticatedUser{}, errors.New("auth session idle lifetime is not configured")
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) || !lastActivity.Add(r.idleLifetime).After(now) {
		return auth.AuthenticatedUser{}, auth.ErrInvalidSession
	}
	var user entity.User
	if err := r.db.WithContext(ctx).Where("id = ?", session.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.AuthenticatedUser{}, auth.ErrInvalidSession
		}
		return auth.AuthenticatedUser{}, fmt.Errorf("finding session user: %w", err)
	}
	if user.Status != activeUserStatus {
		return auth.AuthenticatedUser{}, auth.ErrUserDisabled
	}
	lastUsedAt := now
	if session.LastUsedAt != nil {
		lastUsedAt = *session.LastUsedAt
		if now.Sub(lastUsedAt) >= 5*time.Minute {
			if err := r.db.WithContext(ctx).Model(&entity.RetailSession{}).Where("id = ?", session.ID).Updates(map[string]any{"last_used_at": now, "updated_at": now}).Error; err != nil {
				return auth.AuthenticatedUser{}, fmt.Errorf("updating retail session activity: %w", err)
			}
			lastUsedAt = now
		}
	}
	return auth.AuthenticatedUser{User: userProfile(user), SessionID: session.ID, LastUsedAt: lastUsedAt}, nil
}

func (r *AuthRepository) UpdateDisplayName(ctx context.Context, userID, displayName string) (auth.UserProfile, error) {
	var profile auth.UserProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", userID, activeUserStatus).
			First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return auth.ErrInvalidSession
			}
			return fmt.Errorf("finding profile user: %w", err)
		}
		user.DisplayName = displayName
		user.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("updating profile user: %w", err)
		}
		profile = userProfile(user)
		return nil
	})
	if err != nil {
		return auth.UserProfile{}, err
	}
	return profile, nil
}

func (r *AuthRepository) SetDeveloperMode(ctx context.Context, userID string, enabled bool) (auth.UserProfile, error) {
	now := time.Now().UTC()
	updates := map[string]any{"updated_at": now}
	if enabled {
		updates["developer_enabled_at"] = now
	} else {
		updates["developer_enabled_at"] = nil
	}
	result := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ? AND status = ?", userID, activeUserStatus).
		Updates(updates)
	if result.Error != nil {
		return auth.UserProfile{}, fmt.Errorf("setting developer mode: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return auth.UserProfile{}, auth.ErrInvalidSession
	}
	return r.FindProfile(ctx, userID)
}

func (r *AuthRepository) FindProfile(ctx context.Context, userID string) (auth.UserProfile, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).
		Where("id = ? AND status = ?", userID, activeUserStatus).
		First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.UserProfile{}, auth.ErrInvalidSession
		}
		return auth.UserProfile{}, fmt.Errorf("finding profile: %w", err)
	}
	return userProfile(user), nil
}

func (r *AuthRepository) ReplaceAvatarObjectKey(ctx context.Context, userID, objectKey string) (auth.UserProfile, string, error) {
	return r.updateAvatarObjectKey(ctx, userID, &objectKey)
}

func (r *AuthRepository) ClearAvatarObjectKey(ctx context.Context, userID string) (auth.UserProfile, string, error) {
	return r.updateAvatarObjectKey(ctx, userID, nil)
}

func (r *AuthRepository) updateAvatarObjectKey(ctx context.Context, userID string, objectKey *string) (auth.UserProfile, string, error) {
	var profile auth.UserProfile
	var previousKey string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user entity.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", userID, activeUserStatus).
			First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return auth.ErrInvalidSession
			}
			return fmt.Errorf("finding avatar profile: %w", err)
		}
		if user.AvatarObjectKey != nil {
			previousKey = *user.AvatarObjectKey
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.User{}).Where("id = ?", user.ID).Updates(map[string]any{
			"avatar_object_key": objectKey,
			"updated_at":        now,
		}).Error; err != nil {
			return fmt.Errorf("updating avatar object key: %w", err)
		}
		user.AvatarObjectKey = objectKey
		user.UpdatedAt = now
		profile = userProfile(user)
		return nil
	})
	if err != nil {
		return auth.UserProfile{}, "", err
	}
	return profile, previousKey, nil
}

func (r *AuthRepository) RevokeSessionsForUser(ctx context.Context, userID string, now time.Time) error {
	revokedAt := now.UTC()
	if err := r.db.WithContext(ctx).Model(&entity.RetailSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{"revoked_at": revokedAt, "updated_at": revokedAt}).Error; err != nil {
		return fmt.Errorf("revoking user sessions: %w", err)
	}
	return nil
}

// CreateUserWithCredential creates the user, its password credential, the
// credentials identity, and the pending email-verification challenge in one
// transaction. A duplicate email returns auth.ErrEmailTaken.
func (r *AuthRepository) CreateUserWithCredential(ctx context.Context, record auth.RegisterRecord) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		user := entity.User{
			ID:          record.UserID,
			Status:      activeUserStatus,
			DisplayName: record.DisplayName,
			Email:       &record.Email,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating user: %w", err)
		}
		credential := entity.AuthCredential{
			ID:                newUUID(),
			UserID:            user.ID,
			PasswordHash:      record.PasswordHash,
			PasswordChangedAt: now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if credential.ID == "" {
			return errors.New("generating credential id")
		}
		if err := tx.Create(&credential).Error; err != nil {
			return fmt.Errorf("creating credential: %w", err)
		}
		identity := entity.UserIdentity{
			ID:          newUUID(),
			UserID:      user.ID,
			Provider:    auth.ProviderCredentials,
			Subject:     record.Email,
			CreatedAt:   now,
			LastLoginAt: nil,
		}
		if identity.ID == "" {
			return errors.New("generating identity id")
		}
		if err := tx.Create(&identity).Error; err != nil {
			return fmt.Errorf("creating credentials identity: %w", err)
		}
		challenge := entity.AuthChallenge{
			ID:        newUUID(),
			UserID:    record.Challenge.UserID,
			TokenHash: append([]byte(nil), record.Challenge.TokenHash...),
			Purpose:   record.Challenge.Purpose,
			ExpiresAt: record.Challenge.ExpiresAt.UTC(),
			CreatedAt: now,
		}
		if challenge.ID == "" {
			return errors.New("generating challenge id")
		}
		if err := tx.Create(&challenge).Error; err != nil {
			return fmt.Errorf("creating verification challenge: %w", err)
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return auth.ErrEmailTaken
		}
		return err
	}
	return nil
}

func (r *AuthRepository) FindCredentialByEmail(ctx context.Context, email string) (auth.CredentialState, bool, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.CredentialState{}, false, nil
		}
		return auth.CredentialState{}, false, fmt.Errorf("finding credential user: %w", err)
	}
	return r.findCredentialForUser(ctx, user)
}

func (r *AuthRepository) FindCredentialByUserID(ctx context.Context, userID string) (auth.CredentialState, bool, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.CredentialState{}, false, nil
		}
		return auth.CredentialState{}, false, fmt.Errorf("finding credential user: %w", err)
	}
	return r.findCredentialForUser(ctx, user)
}

func (r *AuthRepository) findCredentialForUser(ctx context.Context, user entity.User) (auth.CredentialState, bool, error) {
	var credential entity.AuthCredential
	if err := r.db.WithContext(ctx).Where("user_id = ?", user.ID).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.CredentialState{}, false, nil
		}
		return auth.CredentialState{}, false, fmt.Errorf("finding credential: %w", err)
	}
	state := auth.CredentialState{
		UserID:        user.ID,
		Email:         dereference(user.Email),
		DisplayName:   user.DisplayName,
		PasswordHash:  credential.PasswordHash,
		EmailVerified: user.EmailVerifiedAt != nil,
		Active:        user.Status == activeUserStatus,
		FailedCount:   credential.FailedLoginCount,
		LockedUntil:   credential.LockedUntil,
	}
	return state, true, nil
}

func (r *AuthRepository) FindUserByEmail(ctx context.Context, email string) (auth.UserProfile, bool, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).Where("email = ? AND status = ?", email, activeUserStatus).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.UserProfile{}, false, nil
		}
		return auth.UserProfile{}, false, fmt.Errorf("finding user by email: %w", err)
	}
	return userProfile(user), true, nil
}

func (r *AuthRepository) IncrementLoginFailures(ctx context.Context, userID string, maxFailures int, lockDuration time.Duration, now time.Time) error {
	lockedUntil := gorm.Expr(
		"CASE WHEN failed_login_count + 1 >= ? THEN ? ELSE locked_until END",
		maxFailures, now.UTC().Add(lockDuration),
	)
	if err := r.db.WithContext(ctx).Model(&entity.AuthCredential{}).Where("user_id = ?", userID).Updates(map[string]any{
		"failed_login_count": gorm.Expr("failed_login_count + 1"),
		"locked_until":       lockedUntil,
		"updated_at":         now.UTC(),
	}).Error; err != nil {
		return fmt.Errorf("recording login failure: %w", err)
	}
	return nil
}

func (r *AuthRepository) ResetLoginFailures(ctx context.Context, userID string, now time.Time) error {
	if err := r.db.WithContext(ctx).Model(&entity.AuthCredential{}).Where("user_id = ?", userID).Updates(map[string]any{
		"failed_login_count": 0,
		"locked_until":       nil,
		"updated_at":         now.UTC(),
	}).Error; err != nil {
		return fmt.Errorf("resetting login failures: %w", err)
	}
	return nil
}

// UpdatePasswordHash writes a new password hash, creating the credential row
// when the account previously had none (for example a Google-only account
// that requested a password reset).
func (r *AuthRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.AuthCredential{}).Where("user_id = ?", userID).Updates(map[string]any{
			"password_hash":       passwordHash,
			"password_changed_at": now.UTC(),
			"failed_login_count":  0,
			"locked_until":        nil,
			"updated_at":          now.UTC(),
		})
		if result.Error != nil {
			return fmt.Errorf("updating password: %w", result.Error)
		}
		if result.RowsAffected > 0 {
			return nil
		}
		credential := entity.AuthCredential{
			ID:                newUUID(),
			UserID:            userID,
			PasswordHash:      passwordHash,
			PasswordChangedAt: now.UTC(),
			CreatedAt:         now.UTC(),
			UpdatedAt:         now.UTC(),
		}
		if credential.ID == "" {
			return errors.New("generating credential id")
		}
		if err := tx.Create(&credential).Error; err != nil {
			if isUniqueViolation(err) {
				return nil
			}
			return fmt.Errorf("creating credential: %w", err)
		}
		return nil
	})
}

func (r *AuthRepository) SetEmailVerified(ctx context.Context, userID string, now time.Time) (auth.UserProfile, error) {
	result := r.db.WithContext(ctx).Model(&entity.User{}).
		Where("id = ? AND status = ? AND email_verified_at IS NULL", userID, activeUserStatus).
		Updates(map[string]any{"email_verified_at": now.UTC(), "updated_at": now.UTC()})
	if result.Error != nil {
		return auth.UserProfile{}, fmt.Errorf("verifying email: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Either already verified or the user does not exist; both resolve to
		// the same profile outcome below.
	}
	return r.FindProfile(ctx, userID)
}

func (r *AuthRepository) CreateChallenge(ctx context.Context, challenge auth.ChallengeRecord) error {
	row := entity.AuthChallenge{
		ID:        newUUID(),
		UserID:    challenge.UserID,
		TokenHash: append([]byte(nil), challenge.TokenHash...),
		Purpose:   challenge.Purpose,
		ExpiresAt: challenge.ExpiresAt.UTC(),
		CreatedAt: time.Now().UTC(),
	}
	if row.ID == "" {
		return errors.New("generating challenge id")
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating auth challenge: %w", err)
	}
	return nil
}

func (r *AuthRepository) ConsumeChallenge(ctx context.Context, tokenHash []byte, purpose string, now time.Time) (string, error) {
	var userID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row entity.AuthChallenge
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", tokenHash, purpose, now.UTC()).
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.ErrInvalidChallenge
		}
		if err != nil {
			return fmt.Errorf("finding auth challenge: %w", err)
		}
		updated := tx.Model(&entity.AuthChallenge{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", row.ID, now.UTC()).
			Updates(map[string]any{"consumed_at": now.UTC()}).RowsAffected
		if updated != 1 {
			return auth.ErrInvalidChallenge
		}
		userID = row.UserID
		return nil
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func userProfile(user entity.User) auth.UserProfile {
	profile := auth.UserProfile{ID: user.ID, DisplayName: user.DisplayName, DeveloperEnabled: user.DeveloperEnabledAt != nil}
	if user.Email != nil {
		profile.Email = *user.Email
	}
	if user.AvatarObjectKey != nil {
		profile.AvatarObjectKey = *user.AvatarObjectKey
	}
	profile.EmailVerified = user.EmailVerifiedAt != nil
	return profile
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var _ auth.AuthRepository = (*AuthRepository)(nil)
