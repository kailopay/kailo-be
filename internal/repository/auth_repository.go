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
			identityRow = entity.UserIdentity{
				ID:          newUUID(),
				UserID:      user.ID,
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

func (r *AuthRepository) RevokeSessionsForIdentity(ctx context.Context, provider, subject string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity entity.UserIdentity
		if err := tx.Where("provider = ? AND subject = ?", provider, subject).First(&identity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("finding password-reset identity: %w", err)
		}
		revokedAt := now.UTC()
		if err := tx.Model(&entity.RetailSession{}).
			Where("user_id = ? AND revoked_at IS NULL", identity.UserID).
			Updates(map[string]any{"revoked_at": revokedAt, "updated_at": revokedAt}).Error; err != nil {
			return fmt.Errorf("revoking password-reset sessions: %w", err)
		}
		return nil
	})
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

var _ auth.TransactionStore = (*AuthRepository)(nil)
var _ auth.UserSessionStore = (*AuthRepository)(nil)
var _ auth.ProfileStore = (*AuthRepository)(nil)
var _ auth.IdentitySessionRevoker = (*AuthRepository)(nil)
