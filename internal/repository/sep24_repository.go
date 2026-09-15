package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SEP24Repository persists the mapping between protocol transaction IDs and
// orders. It uses the order ownership predicates as a second authorization
// boundary on reads.
type SEP24Repository struct {
	db *gorm.DB
}

func NewSEP24Repository(db *gorm.DB) *SEP24Repository {
	return &SEP24Repository{db: db}
}

func (r *SEP24Repository) Create(ctx context.Context, record usecase.Sep24TransactionRecord) error {
	if err := record.Principal.Validate(); err != nil {
		return usecase.ErrSEP24TransactionConflict
	}
	if strings.TrimSpace(record.TransactionID) == "" || strings.TrimSpace(record.OrderID) == "" {
		return usecase.ErrSEP24TransactionConflict
	}
	direction := sep24OrderDirection(record.Kind)
	if direction == "" {
		return usecase.ErrSEP24TransactionConflict
	}

	now := time.Now().UTC()
	return newTxManager(r.db).do(ctx, func(tx *gorm.DB) error {
		orderQuery, err := applyOrderOwnership(tx.WithContext(ctx).Where("id = ? AND direction = ?", record.OrderID, direction), record.Principal)
		if err != nil {
			return usecase.ErrSEP24TransactionConflict
		}
		var order entity.OrderRecord
		if err := orderQuery.First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrSEP24TransactionConflict
			}
			return fmt.Errorf("finding sep-24 order: %w", err)
		}

		id, err := platform.NewID()
		if err != nil {
			return fmt.Errorf("generating sep-24 mapping id: %w", err)
		}
		mapping := entity.SEP24Transaction{ID: id, TransactionID: record.TransactionID, OrderID: record.OrderID, Kind: record.Kind, CreatedAt: now}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&mapping)
		if result.Error != nil {
			return fmt.Errorf("creating sep-24 mapping: %w", result.Error)
		}
		if result.RowsAffected > 0 {
			return nil
		}

		var existing entity.SEP24Transaction
		if err := tx.Where("transaction_id = ?", record.TransactionID).First(&existing).Error; err != nil {
			return fmt.Errorf("finding existing sep-24 mapping: %w", err)
		}
		if existing.OrderID != record.OrderID || existing.Kind != record.Kind {
			return usecase.ErrSEP24TransactionConflict
		}
		return nil
	})
}

func sep24OrderDirection(kind string) string {
	switch kind {
	case usecase.Sep24KindDeposit:
		return "onramp"
	case usecase.Sep24KindWithdraw:
		return "offramp"
	default:
		return ""
	}
}

func (r *SEP24Repository) Find(ctx context.Context, principal usecase.OrderPrincipal, transactionID string) (usecase.Sep24TransactionRecord, error) {
	if err := principal.Validate(); err != nil {
		return usecase.Sep24TransactionRecord{}, usecase.ErrSEP24TransactionNotFound
	}
	transactionID = strings.TrimSpace(transactionID)
	if transactionID == "" || len(transactionID) > 255 {
		return usecase.Sep24TransactionRecord{}, usecase.ErrSEP24TransactionNotFound
	}

	query := r.db.WithContext(ctx).Model(&entity.SEP24Transaction{}).
		Joins("JOIN orders ON orders.id = sep24_transactions.order_id").
		Where("sep24_transactions.transaction_id = ?", transactionID)
	if principal.IsAPIClient() {
		query = query.Where("orders.client_id = ? AND orders.retail_session_id IS NULL", principal.ClientID)
	} else {
		query = query.Where("orders.client_id IS NULL AND orders.retail_session_id IS NOT NULL AND orders.created_by_user_id = ?", principal.OwnerUserID)
	}

	var mapping entity.SEP24Transaction
	if err := query.First(&mapping).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.Sep24TransactionRecord{}, usecase.ErrSEP24TransactionNotFound
		}
		return usecase.Sep24TransactionRecord{}, fmt.Errorf("finding sep-24 mapping: %w", err)
	}
	return usecase.Sep24TransactionRecord{
		Principal:     principal,
		TransactionID: mapping.TransactionID,
		OrderID:       mapping.OrderID,
		Kind:          mapping.Kind,
	}, nil
}
