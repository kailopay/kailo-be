package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var emailOutboxTopics = []string{
	usecase.EmailVerificationTopic,
	usecase.PasswordResetTopic,
}

type EmailRepository struct {
	db          *gorm.DB
	tx          txManager
	maxAttempts int
}

func NewEmailRepository(db *gorm.DB, maxAttempts int) *EmailRepository {
	return &EmailRepository{db: db, tx: newTxManager(db), maxAttempts: maxAttempts}
}

func (r *EmailRepository) LeaseEmail(ctx context.Context, workerID string, now time.Time, duration time.Duration) (usecase.EmailOutboxJob, error) {
	var job usecase.EmailOutboxJob
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var row entity.OutboxMessage
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("topic IN ? AND processed_at IS NULL AND available_at <= ? AND attempts < ? AND (lease_until IS NULL OR lease_until < ?)",
				emailOutboxTopics, now.UTC(), r.maxAttempts, now.UTC()).
			Order("available_at, created_at").First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.ErrNoJob
		}
		if err != nil {
			return fmt.Errorf("selecting email outbox message: %w", err)
		}
		leaseUntil := now.UTC().Add(duration)
		if err := tx.Model(&entity.OutboxMessage{}).Where("id = ?", row.ID).Updates(map[string]any{
			"lease_owner": workerID,
			"lease_until": leaseUntil,
			"attempts":    gorm.Expr("attempts + 1"),
		}).Error; err != nil {
			return fmt.Errorf("leasing email outbox message: %w", err)
		}
		job = usecase.EmailOutboxJob{
			ID:       row.ID,
			Topic:    row.Topic,
			Payload:  append([]byte(nil), row.Payload...),
			Attempts: row.Attempts + 1,
		}
		return nil
	})
	return job, err
}

func (r *EmailRepository) CompleteEmail(ctx context.Context, id string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.OutboxMessage{}).
		Where("id = ? AND topic IN ? AND processed_at IS NULL", id, emailOutboxTopics).
		Updates(map[string]any{
			"processed_at": now.UTC(),
			"lease_owner":  nil,
			"lease_until":  nil,
			"last_error":   nil,
		})
	if result.Error != nil {
		return fmt.Errorf("completing email outbox message: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("email outbox message was not active")
	}
	return nil
}

func (r *EmailRepository) RetryEmail(ctx context.Context, id string, availableAt time.Time, safeError string) error {
	result := r.db.WithContext(ctx).Model(&entity.OutboxMessage{}).
		Where("id = ? AND topic IN ? AND processed_at IS NULL", id, emailOutboxTopics).
		Updates(map[string]any{
			"available_at": availableAt.UTC(),
			"lease_owner":  nil,
			"lease_until":  nil,
			"last_error":   safeError,
		})
	if result.Error != nil {
		return fmt.Errorf("rescheduling email outbox message: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("email outbox message was not active")
	}
	return nil
}

func (r *EmailRepository) FailEmail(ctx context.Context, id string, now time.Time, safeError string) error {
	result := r.db.WithContext(ctx).Model(&entity.OutboxMessage{}).
		Where("id = ? AND topic IN ? AND processed_at IS NULL", id, emailOutboxTopics).
		Updates(map[string]any{
			"processed_at": now.UTC(),
			"lease_owner":  nil,
			"lease_until":  nil,
			"last_error":   safeError,
		})
	if result.Error != nil {
		return fmt.Errorf("failing email outbox message: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("email outbox message was not active")
	}
	return nil
}

var _ usecase.EmailOutboxRepository = (*EmailRepository)(nil)
