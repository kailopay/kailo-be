package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

// SEP24InteractiveRepository persists the pre-order browser hand-off. The
// browser token is queried by hash only; the raw token is never written to
// the database.
type SEP24InteractiveRepository struct {
	db *gorm.DB
}

func NewSEP24InteractiveRepository(db *gorm.DB) *SEP24InteractiveRepository {
	return &SEP24InteractiveRepository{db: db}
}

func (r *SEP24InteractiveRepository) Create(ctx context.Context, session entity.SEP24InteractiveSession) error {
	if err := r.db.WithContext(ctx).Create(&session).Error; err != nil {
		if isUniqueViolation(err) {
			return usecase.ErrSEP24InteractiveConflict
		}
		return fmt.Errorf("creating SEP-24 interactive session: %w", err)
	}
	return nil
}

func (r *SEP24InteractiveRepository) FindByTransactionID(ctx context.Context, transactionID string) (entity.SEP24InteractiveSession, error) {
	var session entity.SEP24InteractiveSession
	if err := r.db.WithContext(ctx).Where("transaction_id = ?", transactionID).First(&session).Error; err != nil {
		return sessionError(err, "finding SEP-24 interactive session")
	}
	return session, nil
}

func (r *SEP24InteractiveRepository) FindByBrowserTokenHash(ctx context.Context, transactionID string, tokenHash []byte) (entity.SEP24InteractiveSession, error) {
	var session entity.SEP24InteractiveSession
	if err := r.db.WithContext(ctx).
		Where("transaction_id = ? AND browser_token_hash = ?", transactionID, tokenHash).
		First(&session).Error; err != nil {
		return sessionError(err, "finding SEP-24 browser session")
	}
	return session, nil
}

func (r *SEP24InteractiveRepository) LinkUser(ctx context.Context, transactionID, walletAccount, userID, sessionID, kycStatus string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.SEP24InteractiveSession{}).
		Where("transaction_id = ? AND wallet_account = ? AND (linked_user_id IS NULL OR linked_user_id = ?)", transactionID, walletAccount, userID).
		Updates(map[string]any{
			"linked_user_id":    userID,
			"linked_session_id": sessionID,
			"kyc_status":        kycStatus,
			"updated_at":        now.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("linking SEP-24 interactive session: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	session, err := r.FindByTransactionID(ctx, transactionID)
	if err != nil {
		return err
	}
	if session.WalletAccount != walletAccount || session.LinkedUserID == nil || *session.LinkedUserID != userID {
		return usecase.ErrSEP24InteractiveUserMismatch
	}
	return nil
}

func (r *SEP24InteractiveRepository) Complete(ctx context.Context, transactionID, userID, orderID string, completedAt time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.SEP24InteractiveSession{}).
		Where("transaction_id = ? AND linked_user_id = ? AND order_id IS NULL", transactionID, userID).
		Updates(map[string]any{
			"order_id":     orderID,
			"completed_at": completedAt.UTC(),
			"kyc_status":   string(usecase.KYCStatusApproved),
			"updated_at":   completedAt.UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("completing SEP-24 interactive session: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	session, err := r.FindByTransactionID(ctx, transactionID)
	if err != nil {
		return err
	}
	if session.OrderID != nil && *session.OrderID != "" {
		return usecase.ErrSEP24InteractiveCompleted
	}
	if session.LinkedUserID == nil || *session.LinkedUserID != userID {
		return usecase.ErrSEP24InteractiveUserMismatch
	}
	return usecase.ErrSEP24InteractiveConflict
}

func sessionError(err error, operation string) (entity.SEP24InteractiveSession, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entity.SEP24InteractiveSession{}, usecase.ErrSEP24InteractiveNotFound
	}
	return entity.SEP24InteractiveSession{}, fmt.Errorf("%s: %w", operation, err)
}

var _ usecase.SEP24InteractiveRepository = (*SEP24InteractiveRepository)(nil)
