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

type SEP38QuoteRepository struct {
	db *gorm.DB
}

func NewSEP38QuoteRepository(db *gorm.DB) *SEP38QuoteRepository {
	return &SEP38QuoteRepository{db: db}
}

func (r *SEP38QuoteRepository) Create(ctx context.Context, quote entity.SEP38Quote) error {
	if err := r.db.WithContext(ctx).Create(&quote).Error; err != nil {
		if isUniqueViolation(err) {
			return usecase.ErrSEP38QuoteConflict
		}
		return fmt.Errorf("creating SEP-38 quote: %w", err)
	}
	return nil
}

func (r *SEP38QuoteRepository) Find(ctx context.Context, walletAccount, quoteID string) (entity.SEP38Quote, error) {
	var quote entity.SEP38Quote
	if err := r.db.WithContext(ctx).Where("wallet_account = ? AND quote_id = ?", walletAccount, quoteID).First(&quote).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entity.SEP38Quote{}, usecase.ErrSEP38QuoteNotFound
		}
		return entity.SEP38Quote{}, fmt.Errorf("finding SEP-38 quote: %w", err)
	}
	return quote, nil
}

func (r *SEP38QuoteRepository) Consume(ctx context.Context, walletAccount, quoteID string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.SEP38Quote{}).
		Where("wallet_account = ? AND quote_id = ? AND consumed_at IS NULL AND expires_at > ?", walletAccount, quoteID, now.UTC()).
		Updates(map[string]any{"consumed_at": now.UTC(), "updated_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("consuming SEP-38 quote: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	quote, err := r.Find(ctx, walletAccount, quoteID)
	if err != nil {
		return err
	}
	if quote.ConsumedAt != nil {
		return usecase.ErrSEP38QuoteConsumed
	}
	if !now.UTC().Before(quote.ExpiresAt.UTC()) {
		return usecase.ErrSEP38QuoteExpired
	}
	return usecase.ErrSEP38QuoteConflict
}

var _ usecase.SEP38QuoteRepository = (*SEP38QuoteRepository)(nil)
