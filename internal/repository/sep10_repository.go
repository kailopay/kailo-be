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

type SEP10Repository struct {
	db *gorm.DB
}

func NewSEP10Repository(db *gorm.DB) *SEP10Repository {
	return &SEP10Repository{db: db}
}

func (r *SEP10Repository) Create(ctx context.Context, challenge entity.SEP10Challenge) error {
	if err := r.db.WithContext(ctx).Create(&challenge).Error; err != nil {
		return fmt.Errorf("creating SEP-10 challenge: %w", err)
	}
	return nil
}

func (r *SEP10Repository) FindByHash(ctx context.Context, challengeHash []byte) (entity.SEP10Challenge, error) {
	var challenge entity.SEP10Challenge
	if err := r.db.WithContext(ctx).Where("challenge_hash = ?", challengeHash).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return entity.SEP10Challenge{}, usecase.ErrSEP10ChallengeNotFound
		}
		return entity.SEP10Challenge{}, fmt.Errorf("finding SEP-10 challenge: %w", err)
	}
	return challenge, nil
}

func (r *SEP10Repository) Consume(ctx context.Context, challengeID string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.SEP10Challenge{}).
		Where("id = ? AND consumed_at IS NULL", challengeID).
		Updates(map[string]any{"consumed_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("consuming SEP-10 challenge: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var challenge entity.SEP10Challenge
	if err := r.db.WithContext(ctx).Select("id").Where("id = ?", challengeID).First(&challenge).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.ErrSEP10ChallengeNotFound
	}
	return usecase.ErrSEP10ChallengeConsumed
}

var _ usecase.SEP10ChallengeRepository = (*SEP10Repository)(nil)
