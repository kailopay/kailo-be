package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettlementRepository struct {
	db          *gorm.DB
	tx          txManager
	maxAttempts int
}

func NewSettlementRepository(db *gorm.DB, maxAttempts int) *SettlementRepository {
	return &SettlementRepository{db: db, tx: newTxManager(db), maxAttempts: maxAttempts}
}

func (r *SettlementRepository) Lease(ctx context.Context, workerID string, now time.Time, duration time.Duration) (usecase.Job, error) {
	var job usecase.Job
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var row entity.OutboxMessage
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("topic = ? AND processed_at IS NULL AND available_at <= ? AND attempts < ? AND (lease_until IS NULL OR lease_until < ?)",
				"stellar.settle_onramp", now, r.maxAttempts, now).
			Order("available_at, created_at").First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.ErrNoJob
		}
		if err != nil {
			return fmt.Errorf("selecting settlement outbox message: %w", err)
		}
		leaseUntil := now.Add(duration)
		if err := tx.Model(&entity.OutboxMessage{}).Where("id = ?", row.ID).Updates(map[string]any{
			"lease_owner": workerID, "lease_until": leaseUntil, "attempts": gorm.Expr("attempts + 1"),
		}).Error; err != nil {
			return fmt.Errorf("leasing settlement outbox message: %w", err)
		}
		var payload struct {
			IntentID string `json:"intent_id"`
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil || payload.IntentID == "" {
			return errors.New("invalid settlement outbox payload")
		}
		job = usecase.Job{OutboxID: row.ID, IntentID: payload.IntentID}
		return nil
	})
	return job, err
}

func (r *SettlementRepository) LoadIntent(ctx context.Context, intentID string) (usecase.Intent, error) {
	var row entity.StellarTransaction
	if err := r.db.WithContext(ctx).Where("intent_id = ? AND purpose = ?", intentID, "transfer").First(&row).Error; err != nil {
		return usecase.Intent{}, fmt.Errorf("finding settlement intent: %w", err)
	}
	amount, err := parseStroops(row.Amount)
	if err != nil {
		return usecase.Intent{}, err
	}
	transfer := usecase.Transfer{OrderID: row.OrderID, Amount: amount}
	if row.Source != nil {
		transfer.Source = *row.Source
	}
	if row.Destination != nil {
		transfer.Destination = *row.Destination
	}
	if row.Memo != nil {
		transfer.Memo = *row.Memo
	}
	intent := usecase.Intent{IntentID: row.IntentID, Transfer: transfer}
	if row.TransactionHash != nil {
		intent.TransactionHash = *row.TransactionHash
	}
	return intent, nil
}

func (r *SettlementRepository) MarkSubmitted(ctx context.Context, intentID, hash string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.StellarTransaction{}).Where("intent_id = ?", intentID).Updates(map[string]any{
		"transaction_hash": hash, "status": "submitted", "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
	}).Error
}

func (r *SettlementRepository) Confirm(ctx context.Context, intentID, hash string, ledgerAt time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var stellar entity.StellarTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_id = ?", intentID).First(&stellar).Error; err != nil {
			return err
		}
		if stellar.Status == "confirmed" {
			return nil
		}
		if stellar.TransactionHash == nil || *stellar.TransactionHash != hash {
			return errors.New("settlement hash mismatch")
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.StellarTransaction{}).Where("id = ?", stellar.ID).Updates(map[string]any{
			"status": "confirmed", "ledger_at": ledgerAt, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		var reservation entity.TreasuryReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ?", stellar.OrderID).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == "reserved" {
			if err := tx.Model(&entity.TreasuryReservation{}).Where("id = ?", reservation.ID).Updates(map[string]any{
				"status": "consumed", "consumed_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&entity.TreasuryAccount{}).Where("id = ?", reservation.TreasuryID).
				Update("reserved_stroops", gorm.Expr("reserved_stroops - ?", reservation.AmountStroops)).Error; err != nil {
				return err
			}
		}
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", stellar.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != string(entity.OrderStatusStellarProcessing) {
			return entity.ErrInvalidOrderState
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ?", order.ID).Updates(map[string]any{
			"status": entity.OrderStatusCompleted, "version": order.Version + 1, "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := appendSettlementEvent(tx, order, now); err != nil {
			return err
		}
		return tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.settle_onramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error
	})
}

func (r *SettlementRepository) MarkUnknown(ctx context.Context, intentID, safeError string) error {
	return r.db.WithContext(ctx).Model(&entity.StellarTransaction{}).Where("intent_id = ?", intentID).
		Updates(map[string]any{"status": "unknown", "last_error": safeError, "updated_at": time.Now().UTC()}).Error
}

func (r *SettlementRepository) FailPermanent(ctx context.Context, intentID, safeError string) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var stellar entity.StellarTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_id = ?", intentID).First(&stellar).Error; err != nil {
			return err
		}
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", stellar.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status != string(entity.OrderStatusStellarProcessing) {
			return entity.ErrInvalidOrderState
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.StellarTransaction{}).Where("id = ?", stellar.ID).Updates(map[string]any{
			"status": "failed", "last_error": safeError, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).Updates(map[string]any{
			"status": entity.OrderStatusStellarFailed, "failure_code": "stellar_permanent_failure", "failure_stage": "stellar",
			"failure_retryable": false, "version": order.Version + 1, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "stellar.failed", order.Status, string(entity.OrderStatusStellarFailed), now); err != nil {
			return err
		}
		return tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.settle_onramp", stellar.OrderID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": safeError}).Error
	})
}

func (r *SettlementRepository) RetryLater(ctx context.Context, intentID string, availableAt time.Time, safeError string) error {
	var stellar entity.StellarTransaction
	if err := r.db.WithContext(ctx).Select("order_id").Where("intent_id = ?", intentID).First(&stellar).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.settle_onramp", stellar.OrderID).
		Updates(map[string]any{"available_at": availableAt, "lease_owner": nil, "lease_until": nil, "last_error": safeError}).Error
}

// ResetSubmitted clears a persisted-but-rejected transaction hash so the next
// attempt rebuilds from the current account sequence. It must only be used for
// Horizon responses that prove the transaction was never applied.
func (r *SettlementRepository) ResetSubmitted(ctx context.Context, intentID, safeError string) error {
	return r.db.WithContext(ctx).Model(&entity.StellarTransaction{}).Where("intent_id = ?", intentID).
		Updates(map[string]any{"transaction_hash": nil, "status": "pending", "last_error": safeError, "updated_at": time.Now().UTC()}).Error
}

// ReleaseExpiredReservations releases inventory held by reservations whose
// expiry has passed. Only unpaid orders still awaiting payment expire; orders
// held for operator resolution, such as an unknown checkout outcome, keep
// their inventory reserved by design.
func (r *SettlementRepository) ReleaseExpiredReservations(ctx context.Context, now time.Time, limit int) (int, error) {
	released := 0
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var reservations []entity.TreasuryReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND expires_at < ?", "reserved", now).
			Order("expires_at").Limit(limit).Find(&reservations).Error; err != nil {
			return fmt.Errorf("finding expired treasury reservations: %w", err)
		}
		for _, reservation := range reservations {
			var order entity.OrderRecord
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", reservation.OrderID).First(&order).Error; err != nil {
				return fmt.Errorf("locking expired order: %w", err)
			}
			if order.Status != string(entity.OrderStatusPaymentPending) {
				continue
			}
			if err := tx.Model(&entity.TreasuryReservation{}).Where("id = ?", reservation.ID).
				Updates(map[string]any{"status": "released", "released_at": now, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("releasing treasury reservation: %w", err)
			}
			if err := tx.Model(&entity.TreasuryAccount{}).Where("id = ?", reservation.TreasuryID).
				Update("reserved_stroops", gorm.Expr("reserved_stroops - ?", reservation.AmountStroops)).Error; err != nil {
				return fmt.Errorf("restoring treasury balance: %w", err)
			}
			if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).Updates(map[string]any{
				"status": entity.OrderStatusExpired, "version": order.Version + 1, "updated_at": now,
			}).Error; err != nil {
				return fmt.Errorf("expiring order: %w", err)
			}
			if err := appendOrderEvent(tx, order.ID, order.Version+1, "order.expired", order.Status, string(entity.OrderStatusExpired), now); err != nil {
				return err
			}
			released++
		}
		return nil
	})
	return released, err
}

func parseStroops(amount string) (entity.Stroops, error) {
	value, ok := new(big.Rat).SetString(amount)
	if !ok || value.Sign() <= 0 {
		return 0, errors.New("invalid settlement amount")
	}
	value.Mul(value, new(big.Rat).SetInt64(int64(entity.StroopsPerXLM)))
	if !value.IsInt() || !value.Num().IsInt64() {
		return 0, errors.New("settlement amount exceeds stroop precision")
	}
	return entity.Stroops(value.Num().Int64()), nil
}

func appendSettlementEvent(tx *gorm.DB, order entity.OrderRecord, now time.Time) error {
	id, err := platform.NewID()
	if err != nil {
		return err
	}
	previous, next := order.Status, string(entity.OrderStatusCompleted)
	event := entity.OrderEvent{ID: id, OrderID: order.ID, AggregateVersion: order.Version + 1, EventType: "stellar.transfer_confirmed",
		PreviousStatus: &previous, NewStatus: &next, Source: "worker", CorrelationID: order.ID, Metadata: []byte(`{}`), CreatedAt: now}
	return tx.Create(&event).Error
}
