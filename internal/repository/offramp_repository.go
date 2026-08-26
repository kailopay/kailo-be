package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const offrampCreateOperation = "offramp.create"

// OfframpRepository implements the off-ramp persistence port.
type OfframpRepository struct {
	db             *gorm.DB
	tx             txManager
	depositAccount string
	network        string
}

func NewOfframpRepository(db *gorm.DB, depositAccount, network string) *OfframpRepository {
	return &OfframpRepository{db: db, tx: newTxManager(db), depositAccount: depositAccount, network: network}
}

func (r *OfframpRepository) FindOfframpReplay(ctx context.Context, clientID, keyHash, requestHash string) (usecase.OrderView, bool, error) {
	var record entity.IdempotencyRecord
	err := r.db.WithContext(ctx).Where("client_id = ? AND operation = ? AND key_hash = ?", clientID, offrampCreateOperation, keyHash).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, false, nil
	}
	if err != nil {
		return usecase.OrderView{}, false, fmt.Errorf("finding idempotency record: %w", err)
	}
	if record.RequestHash != requestHash {
		return usecase.OrderView{}, false, usecase.ErrIdempotencyConflict
	}
	if record.CreatedResourceID == nil {
		return usecase.OrderView{}, false, fmt.Errorf("idempotency record has no order: %w", usecase.ErrInvalidWithdrawal)
	}
	view, err := r.Get(ctx, clientID, *record.CreatedResourceID)
	if err != nil {
		return usecase.OrderView{}, false, err
	}
	return view, true, nil
}

// CreateOfframp persists the order in asset_pending with its deterministic
// deposit instructions and idempotency record in one transaction. Deposits
// add to treasury inventory, so no reservation is made.
func (r *OfframpRepository) CreateOfframp(ctx context.Context, record usecase.OfframpCreateRecord) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		now := record.CreatedAt
		order := entity.OrderRecord{
			ID: record.OrderID, ClientID: &record.ClientID, Direction: "offramp",
			Status: string(entity.OrderStatusAssetPending), Version: 1,
			Currency: "IDR", FiatAmountMinor: int64(record.Quote.FiatAmount),
			AssetCode: "XLM", AssetIssuer: "", Network: r.network,
			AssetAmount: record.AssetAmount.String(), AssetAmountStroops: int64(record.AssetAmount),
			QuoteProvider: "coinmarketcap", QuoteSourceAt: record.Quote.SourceAt,
			QuoteRate: record.Quote.Rate, QuoteAdjustedRate: record.Quote.AdjustedRate,
			QuoteSpreadBPS: record.Quote.SpreadBPS, QuoteExpiresAt: record.Quote.ExpiresAt,
			PaymentMethod: nil, GatewayProvider: nil,
			StellarSource: &r.depositAccount, StellarMemo: &record.Memo,
			WithdrawalMethod:      strPtr(string(entity.WithdrawalMethodSandboxTransfer)),
			WithdrawalDestination: record.DestinationToken,
			ExpiresAt:             &record.ExpiresAt,
			CreatedAt:             now, UpdatedAt: now,
		}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("creating order: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, 1, "order.created", "", string(entity.OrderStatusCreated), now); err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, 2, "deposit.instructions_issued", string(entity.OrderStatusCreated), string(entity.OrderStatusAssetPending), now); err != nil {
			return err
		}
		idempotencyID, err := platform.NewID()
		if err != nil {
			return err
		}
		idempotency := entity.IdempotencyRecord{ID: idempotencyID, ClientID: record.ClientID, Operation: offrampCreateOperation,
			KeyHash: record.IdempotencyKeyHash, RequestHash: record.RequestHash, CreatedResourceID: &order.ID,
			State: "completed", ResponseStatus: intPtr(201), ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&idempotency).Error; err != nil {
			return fmt.Errorf("creating idempotency record: %w", err)
		}
		return nil
	})
}

// FindDepositCandidates returns asset_pending orders for the deposit account
// whose window has not expired, oldest first.
func (r *OfframpRepository) FindDepositCandidates(ctx context.Context, depositAccount string, limit int) ([]usecase.OrderView, error) {
	var rows []entity.OrderRecord
	err := r.db.WithContext(ctx).
		Where("direction = ? AND status = ? AND stellar_source = ?", "offramp", entity.OrderStatusAssetPending, depositAccount).
		Order("created_at").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("finding deposit candidates: %w", err)
	}
	views := make([]usecase.OrderView, 0, len(rows))
	for _, row := range rows {
		view, err := r.orderView(row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// RecordAssetReceived verifies the observed payment against the order, then
// atomically records the deposit evidence and queues retirement. It is safe
// to call repeatedly: an already-received order short-circuits.
func (r *OfframpRepository) RecordAssetReceived(ctx context.Context, orderID string, payment usecase.ObservedPayment) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking off-ramp order: %w", err)
		}
		switch entity.OrderStatus(order.Status) {
		case entity.OrderStatusAssetReceived, entity.OrderStatusRetirementProcessing,
			entity.OrderStatusWithdrawalProcessing, entity.OrderStatusCompleted:
			return nil // already processed by a previous scan
		case entity.OrderStatusAssetPending:
		default:
			return entity.ErrInvalidOrderState
		}

		now := time.Now().UTC()
		intentRowID, err := platform.NewID()
		if err != nil {
			return err
		}
		deposit := entity.StellarTransaction{ID: intentRowID, OrderID: order.ID,
			IntentID: "stellar-offramp-deposit-" + payment.TransactionHash[:min(24, len(payment.TransactionHash))],
			Purpose:  "deposit", Network: order.Network, AssetCode: "XLM", Amount: order.AssetAmount,
			Source: strPtr(payment.From), Destination: order.StellarSource,
			Memo: order.StellarMemo, TransactionHash: strPtr(payment.TransactionHash),
			Status: "confirmed", LedgerAt: &payment.LedgerAt, AttemptCount: 1,
			CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&deposit).Error; err != nil {
			return fmt.Errorf("recording deposit transaction: %w", err)
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusAssetReceived, "version": order.Version + 1, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("marking asset received: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "asset.received", order.Status, string(entity.OrderStatusAssetReceived), now); err != nil {
			return err
		}
		// Queue retirement in the same commit so the deposit fact can never
		// exist without its follow-up work.
		retirementID, err := platform.NewID()
		if err != nil {
			return err
		}
		retirement := entity.StellarTransaction{ID: retirementID, OrderID: order.ID,
			IntentID: "stellar-retire-" + order.ID, Purpose: "retirement", Network: order.Network,
			AssetCode: "XLM", Amount: order.AssetAmount, Source: order.StellarSource,
			Destination: strPtr(usecase.BurnAddress), Memo: order.StellarMemo,
			Status: "pending", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&retirement).Error; err != nil {
			return fmt.Errorf("creating retirement intent: %w", err)
		}
		outboxID, err := platform.NewID()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"intent_id": retirement.IntentID})
		outbox := entity.OutboxMessage{ID: outboxID, Topic: "stellar.retire_offramp", AggregateType: "order",
			AggregateID: order.ID, Payload: payload, CreatedAt: now, AvailableAt: now}
		if err := tx.Create(&outbox).Error; err != nil {
			return fmt.Errorf("creating retirement outbox message: %w", err)
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version+1).
			Updates(map[string]any{"status": entity.OrderStatusRetirementProcessing, "version": order.Version + 2, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("moving order to retirement processing: %w", err)
		}
		return appendOrderEvent(tx, order.ID, order.Version+2, "retirement.requested", string(entity.OrderStatusAssetReceived), string(entity.OrderStatusRetirementProcessing), now)
	})
}

// RecordAssetInvalid marks a correlated but mismatched deposit as invalid.
func (r *OfframpRepository) RecordAssetInvalid(ctx context.Context, orderID, hash, safeReason string) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", orderID, entity.OrderStatusAssetPending).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return entity.ErrInvalidOrderState
			}
			return fmt.Errorf("locking invalid-deposit order: %w", err)
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusAssetInvalid, "failure_code": "deposit_mismatch",
				"failure_stage": "deposit", "failure_retryable": false, "version": order.Version + 1, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("marking asset invalid: %w", err)
		}
		return appendOrderEvent(tx, order.ID, order.Version+1, "asset.invalid", order.Status, string(entity.OrderStatusAssetInvalid), now)
	})
}

func (r *OfframpRepository) LoadRetirement(ctx context.Context, intentID string) (usecase.RetirementIntent, error) {
	var row entity.StellarTransaction
	if err := r.db.WithContext(ctx).Where("intent_id = ? AND purpose = ?", intentID, "retirement").First(&row).Error; err != nil {
		return usecase.RetirementIntent{}, fmt.Errorf("finding retirement intent: %w", err)
	}
	amount, err := parseStroops(row.Amount)
	if err != nil {
		return usecase.RetirementIntent{}, err
	}
	intent := usecase.RetirementIntent{IntentID: row.IntentID, OrderID: row.OrderID,
		BurnTarget: usecase.BurnAddress, Amount: amount}
	if row.Source != nil {
		intent.Source = *row.Source
	}
	if row.Memo != nil {
		intent.Memo = *row.Memo
	}
	if row.TransactionHash != nil {
		intent.TransactionHash = *row.TransactionHash
	}
	return intent, nil
}

// SaveRetirementHash persists the deterministic burn hash before submission.
func (r *OfframpRepository) SaveRetirementHash(ctx context.Context, intentID, hash string, now time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.StellarTransaction{}).Where("intent_id = ?", intentID).
		Updates(map[string]any{"transaction_hash": hash, "status": "submitted", "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now}).Error
}

// ConfirmRetirement records confirmation, advances the order to withdrawal
// processing, and finishes the outbox job atomically.
func (r *OfframpRepository) ConfirmRetirement(ctx context.Context, intentID, hash string, ledgerAt time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var stellar entity.StellarTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_id = ?", intentID).First(&stellar).Error; err != nil {
			return err
		}
		if stellar.Status == "confirmed" {
			return nil
		}
		if stellar.TransactionHash == nil || *stellar.TransactionHash != hash {
			return errors.New("retirement hash mismatch")
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.StellarTransaction{}).Where("id = ?", stellar.ID).Updates(map[string]any{
			"status": "confirmed", "ledger_at": ledgerAt.UTC(), "updated_at": now}).Error; err != nil {
			return err
		}
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", stellar.OrderID).First(&order).Error; err != nil {
			return err
		}
		if entity.OrderStatus(order.Status) != entity.OrderStatusRetirementProcessing {
			return entity.ErrInvalidOrderState
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusWithdrawalProcessing, "version": order.Version + 1, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("moving order to withdrawal processing: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "retirement.confirmed", order.Status, string(entity.OrderStatusWithdrawalProcessing), now); err != nil {
			return err
		}
		return tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.retire_offramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error
	})
}

func (r *OfframpRepository) FailRetirement(ctx context.Context, intentID, safeError string) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var stellar entity.StellarTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("intent_id = ?", intentID).First(&stellar).Error; err != nil {
			return err
		}
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", stellar.OrderID).First(&order).Error; err != nil {
			return err
		}
		if entity.OrderStatus(order.Status) != entity.OrderStatusRetirementProcessing {
			return entity.ErrInvalidOrderState
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.StellarTransaction{}).Where("id = ?", stellar.ID).Updates(map[string]any{
			"status": "failed", "last_error": safeError, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusRetirementFailed, "failure_code": "retirement_failed",
				"failure_stage": "stellar", "failure_retryable": false, "version": order.Version + 1, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "retirement.failed", order.Status, string(entity.OrderStatusRetirementFailed), now); err != nil {
			return err
		}
		return tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.retire_offramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": safeError}).Error
	})
}

// CompleteSimulatedPayout records the deterministic sandbox payout and
// completes the order (ADR-003). The reference wording must always be
// presented as simulation.
func (r *OfframpRepository) CompleteSimulatedPayout(ctx context.Context, orderID string, amountMinor int64, now time.Time) (string, error) {
	reference := "payout_" + orderID
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		payout := entity.OfframpPayout{ID: reference, OrderID: orderID,
			Method: string(entity.WithdrawalMethodSandboxTransfer), AmountMinor: amountMinor,
			ReferenceID: reference, State: "completed", CompletedAt: &now, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&payout).Error; err != nil {
			return fmt.Errorf("recording simulated payout: %w", err)
		}
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			return err
		}
		if entity.OrderStatus(order.Status) != entity.OrderStatusWithdrawalProcessing {
			return entity.ErrInvalidOrderState
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusCompleted, "version": order.Version + 1,
				"completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return appendOrderEvent(tx, order.ID, order.Version+1, "payout.simulated", order.Status, string(entity.OrderStatusCompleted), now)
	})
	if err != nil {
		return "", err
	}
	return reference, nil
}

func (r *OfframpRepository) Get(ctx context.Context, clientID, orderID string) (usecase.OrderView, error) {
	var order entity.OrderRecord
	result := r.db.WithContext(ctx).Raw(`
		SELECT * FROM orders WHERE id = ? AND client_id = ?
	`, orderID, clientID).Scan(&order)
	if result.Error != nil {
		return usecase.OrderView{}, fmt.Errorf("finding order: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return usecase.OrderView{}, usecase.ErrOrderNotFound
	}
	return r.orderView(order)
}

func (r *OfframpRepository) List(ctx context.Context, clientID string, limit int, cursor string) ([]usecase.OrderView, string, error) {
	query := r.db.WithContext(ctx).Where("client_id = ?", clientID).Order("created_at DESC, id DESC").Limit(limit + 1)
	if cursor != "" {
		var cursorOrder entity.OrderRecord
		if err := r.db.WithContext(ctx).Select("created_at", "id").Where("id = ? AND client_id = ?", cursor, clientID).First(&cursorOrder).Error; err != nil {
			return nil, "", usecase.ErrOrderNotFound
		}
		query = query.Where("(created_at, id) < (?, ?)", cursorOrder.CreatedAt, cursorOrder.ID)
	}
	rows := make([]entity.OrderRecord, 0, limit+1)
	if err := query.Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("listing orders: %w", err)
	}
	next := ""
	if len(rows) > limit {
		next = rows[limit-1].ID
		rows = rows[:limit]
	}
	views := make([]usecase.OrderView, 0, len(rows))
	for _, row := range rows {
		view, err := r.orderView(row)
		if err != nil {
			return nil, "", err
		}
		views = append(views, view)
	}
	return views, next, nil
}

func (r *OfframpRepository) orderView(order entity.OrderRecord) (usecase.OrderView, error) {
	view := baseOrderView(order)
	var payout entity.OfframpPayout
	if err := r.db.WithContext(context.Background()).Where("order_id = ?", order.ID).First(&payout).Error; err == nil {
		simulation := true
		view.Payout = &usecase.PayoutView{Reference: payout.ReferenceID, Method: payout.Method,
			AmountMinor: payout.AmountMinor, State: payout.State, Simulated: &simulation}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding payout: %w", err)
	}
	var deposit entity.StellarTransaction
	if err := r.db.WithContext(context.Background()).Where("order_id = ? AND purpose = ?", order.ID, "deposit").First(&deposit).Error; err == nil && deposit.TransactionHash != nil {
		view.DepositTransactionHash = *deposit.TransactionHash
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding deposit transaction: %w", err)
	}
	return view, nil
}

func strPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

var _ usecase.OfframpRepository = (*OfframpRepository)(nil)
