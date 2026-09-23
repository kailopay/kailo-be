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

var errInvalidDepositPayment = errors.New("invalid off-ramp deposit payment")

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

func (r *OfframpRepository) FindOfframpReplay(ctx context.Context, principal usecase.OrderPrincipal, keyHash, requestHash string) (usecase.OrderView, bool, error) {
	var record entity.IdempotencyRecord
	query, err := applyIdempotencyOwnership(r.db.WithContext(ctx), principal)
	if err != nil {
		return usecase.OrderView{}, false, err
	}
	err = query.Where("operation = ? AND key_hash = ?", offrampCreateOperation, keyHash).First(&record).Error
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
	view, err := r.Get(ctx, principal, *record.CreatedResourceID)
	if err != nil {
		return usecase.OrderView{}, false, err
	}
	return view, true, nil
}

// CreateOfframp persists the order in asset_pending with its deterministic
// deposit instructions and idempotency record in one transaction. Deposits
// add to treasury inventory, so no reservation is made.
func (r *OfframpRepository) CreateOfframp(ctx context.Context, record usecase.OfframpCreateRecord) error {
	ownership, err := orderOwnershipFor(record.Principal)
	if err != nil {
		return err
	}
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		if err := validateRetailSessionOwnership(ctx, tx, record.Principal); err != nil {
			return err
		}
		now := record.CreatedAt
		// Two events are appended below, so the aggregate ends at version 2,
		// keeping the row version in lockstep with order_events.
		order := entity.OrderRecord{
			ID: record.OrderID, ClientID: ownership.ClientID, CreatedByUserID: ownership.CreatedByUserID,
			RetailSessionID: ownership.RetailSessionID, WalletAccount: strPtrIfNotEmpty(record.WalletAccount), QuoteID: strPtrIfNotEmpty(record.QuoteID), Direction: "offramp",
			Status: string(entity.OrderStatusAssetPending), Version: 2,
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
		if err := createSandboxOrderFinancial(tx, order); err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, 1, "order.created", "", string(entity.OrderStatusCreated), now); err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, 2, "deposit.instructions_issued", string(entity.OrderStatusCreated), string(entity.OrderStatusAssetPending), now); err != nil {
			return err
		}
		_ = order
		idempotencyID, err := platform.NewID()
		if err != nil {
			return err
		}
		idempotency := entity.IdempotencyRecord{ID: idempotencyID, ClientID: ownership.ClientID, RetailUserID: ownership.RetailUserID, Operation: offrampCreateOperation,
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
		Where("direction = ? AND status = ? AND stellar_source = ? AND (expires_at IS NULL OR expires_at > ?)",
			"offramp", entity.OrderStatusAssetPending, depositAccount, time.Now().UTC()).
		Order("created_at").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("finding deposit candidates: %w", err)
	}
	views := make([]usecase.OrderView, 0, len(rows))
	for _, row := range rows {
		view, err := r.orderView(ctx, row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// ExpirePendingDeposits marks expired off-ramp orders before the deposit
// scanner can accept a late payment. Row locks and the aggregate version
// condition make overlapping worker passes safe.
func (r *OfframpRepository) ExpirePendingDeposits(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	now = now.UTC()
	expired := 0
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var orders []entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("direction = ? AND status = ? AND expires_at IS NOT NULL AND expires_at <= ?",
				"offramp", entity.OrderStatusAssetPending, now).
			Order("expires_at").Limit(limit).Find(&orders).Error; err != nil {
			return fmt.Errorf("finding expired off-ramp orders: %w", err)
		}
		for _, order := range orders {
			result := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ? AND status = ?",
				order.ID, order.Version, entity.OrderStatusAssetPending).
				Updates(map[string]any{"status": entity.OrderStatusExpired, "version": order.Version + 1, "updated_at": now})
			if result.Error != nil {
				return fmt.Errorf("expiring off-ramp order: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("expiring off-ramp order %s: %w", order.ID, entity.ErrInvalidOrderState)
			}
			if err := appendOrderEvent(tx, order.ID, order.Version+1, "order.expired", order.Status,
				string(entity.OrderStatusExpired), now); err != nil {
				return err
			}
			expired++
		}
		return nil
	})
	return expired, err
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
		if err := validateObservedDeposit(order, payment); err != nil {
			return err
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

func validateObservedDeposit(order entity.OrderRecord, payment usecase.ObservedPayment) error {
	if order.Direction != "offramp" || order.StellarSource == nil || order.StellarMemo == nil ||
		payment.TransactionHash == "" || payment.To != *order.StellarSource ||
		payment.Amount != entity.Stroops(order.AssetAmountStroops) || payment.Memo != *order.StellarMemo ||
		payment.LedgerAt.IsZero() {
		return errInvalidDepositPayment
	}
	return nil
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

// RequeueUnsubmittedRetirements recovers exhausted retirement jobs only when
// their order is still processing and the pending intent has no hash.
func (r *OfframpRepository) RequeueUnsubmittedRetirements(ctx context.Context, now time.Time, maxAttempts int) (int64, error) {
	if maxAttempts <= 0 {
		return 0, errors.New("maximum outbox attempts must be positive")
	}

	now = now.UTC()
	result := r.db.WithContext(ctx).Model(&entity.OutboxMessage{}).
		Where(`topic = ? AND processed_at IS NULL AND attempts >= ?
			AND (lease_until IS NULL OR lease_until <= ?)
			AND EXISTS (
				SELECT 1 FROM stellar_transactions
				WHERE stellar_transactions.order_id = outbox_messages.aggregate_id
					AND stellar_transactions.purpose = 'retirement'
					AND stellar_transactions.status = 'pending'
					AND stellar_transactions.transaction_hash IS NULL
					AND stellar_transactions.ledger_at IS NULL
			)
			AND EXISTS (
				SELECT 1 FROM orders
				WHERE orders.id = outbox_messages.aggregate_id
					AND orders.direction = 'offramp'
					AND orders.status = ?
			)`,
			"stellar.retire_offramp", maxAttempts, now, entity.OrderStatusRetirementProcessing).
		Updates(map[string]any{
			"attempts":     0,
			"available_at": now,
			"lease_owner":  nil,
			"lease_until":  nil,
			"last_error":   nil,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("requeueing unsubmitted retirement jobs: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// SimulateRetirement records a sandbox-only retirement without inventing a
// transaction hash or ledger time, then queues the payout simulation atomically.
func (r *OfframpRepository) SimulateRetirement(ctx context.Context, intentID string, now time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var stellar entity.StellarTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("intent_id = ? AND purpose = ?", intentID, "retirement").First(&stellar).Error; err != nil {
			return fmt.Errorf("locking retirement intent: %w", err)
		}

		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", stellar.OrderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking retirement order: %w", err)
		}
		if stellar.Status == "simulated" {
			if stellar.TransactionHash == nil && stellar.LedgerAt == nil &&
				(entity.OrderStatus(order.Status) == entity.OrderStatusWithdrawalProcessing ||
					entity.OrderStatus(order.Status) == entity.OrderStatusCompleted) {
				return nil
			}
			return entity.ErrInvalidOrderState
		}
		if stellar.Status != "pending" || stellar.TransactionHash != nil || stellar.LedgerAt != nil {
			return errors.New("retirement intent cannot be simulated after submission")
		}
		if order.Direction != "offramp" || entity.OrderStatus(order.Status) != entity.OrderStatusRetirementProcessing {
			return entity.ErrInvalidOrderState
		}

		now = now.UTC()
		stellarUpdate := tx.Model(&entity.StellarTransaction{}).
			Where("id = ? AND status = ? AND transaction_hash IS NULL AND ledger_at IS NULL", stellar.ID, "pending").
			Updates(map[string]any{"status": "simulated", "last_error": nil, "updated_at": now})
		if stellarUpdate.Error != nil {
			return fmt.Errorf("recording simulated retirement: %w", stellarUpdate.Error)
		}
		if stellarUpdate.RowsAffected != 1 {
			return entity.ErrInvalidOrderState
		}

		orderUpdate := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ? AND status = ?",
			order.ID, order.Version, entity.OrderStatusRetirementProcessing).
			Updates(map[string]any{"status": entity.OrderStatusWithdrawalProcessing, "version": order.Version + 1, "updated_at": now})
		if orderUpdate.Error != nil {
			return fmt.Errorf("moving simulated retirement order to withdrawal processing: %w", orderUpdate.Error)
		}
		if orderUpdate.RowsAffected != 1 {
			return entity.ErrInvalidOrderState
		}
		if err := appendOrderEventFromSource(tx, order.ID, order.Version+1, "retirement.simulated", order.Status,
			string(entity.OrderStatusWithdrawalProcessing), "worker", now); err != nil {
			return err
		}
		if err := tx.Model(&entity.OutboxMessage{}).
			Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.retire_offramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error; err != nil {
			return fmt.Errorf("completing simulated retirement job: %w", err)
		}
		return enqueueSandboxPayout(tx, order.ID, now)
	})
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
		if err := tx.Model(&entity.SEP24Transaction{}).Where("order_id = ?", stellar.OrderID).
			Updates(map[string]any{"stellar_transaction_id": hash}).Error; err != nil {
			return fmt.Errorf("recording SEP-24 retirement transaction: %w", err)
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
		if err := appendOrderEventFromSource(tx, order.ID, order.Version+1, "retirement.confirmed", order.Status,
			string(entity.OrderStatusWithdrawalProcessing), "worker", now); err != nil {
			return err
		}
		if err := tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.retire_offramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error; err != nil {
			return err
		}
		return enqueueSandboxPayout(tx, order.ID, now)
	})
}

func enqueueSandboxPayout(tx *gorm.DB, orderID string, now time.Time) error {
	payoutOutboxID, err := platform.NewID()
	if err != nil {
		return fmt.Errorf("generating payout outbox id: %w", err)
	}
	payoutPayload, err := json.Marshal(map[string]string{"order_id": orderID})
	if err != nil {
		return fmt.Errorf("encoding payout outbox payload: %w", err)
	}
	if err := tx.Create(&entity.OutboxMessage{
		ID: payoutOutboxID, Topic: usecase.SandboxPayoutTopic, AggregateType: "order", AggregateID: orderID,
		Payload: payoutPayload, CreatedAt: now, AvailableAt: now,
	}).Error; err != nil {
		return fmt.Errorf("creating payout outbox message: %w", err)
	}
	return nil
}

// CompleteSandboxPayout atomically records the deterministic simulated payout,
// advances the order, and finishes the payout outbox message. Re-delivery is
// safe because both the order and payout rows are unique per order.
func (r *OfframpRepository) CompleteSandboxPayout(ctx context.Context, orderID, outboxID, reference string, now time.Time) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking payout order: %w", err)
		}
		if entity.OrderStatus(order.Status) != entity.OrderStatusWithdrawalProcessing && entity.OrderStatus(order.Status) != entity.OrderStatusCompleted {
			return entity.ErrInvalidOrderState
		}

		var payout entity.OfframpPayout
		payoutErr := tx.Where("order_id = ?", order.ID).First(&payout).Error
		if errors.Is(payoutErr, gorm.ErrRecordNotFound) {
			payoutID, err := platform.NewID()
			if err != nil {
				return fmt.Errorf("generating payout id: %w", err)
			}
			completedAt := now.UTC()
			payout = entity.OfframpPayout{ID: payoutID, OrderID: order.ID, Method: "sandbox_bank_transfer",
				AmountMinor: order.FiatAmountMinor, ReferenceID: reference, State: "completed",
				CompletedAt: &completedAt, CreatedAt: completedAt, UpdatedAt: completedAt}
			if err := tx.Create(&payout).Error; err != nil && !isUniqueViolation(err) {
				return fmt.Errorf("creating sandbox payout: %w", err)
			}
			if err := tx.Where("order_id = ?", order.ID).First(&payout).Error; err != nil {
				return fmt.Errorf("loading sandbox payout: %w", err)
			}
		} else if payoutErr != nil {
			return fmt.Errorf("loading sandbox payout: %w", payoutErr)
		}

		if entity.OrderStatus(order.Status) == entity.OrderStatusWithdrawalProcessing {
			if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
				Updates(map[string]any{"status": entity.OrderStatusCompleted, "version": order.Version + 1,
					"completed_at": now.UTC(), "updated_at": now.UTC()}).Error; err != nil {
				return fmt.Errorf("completing payout order: %w", err)
			}
			if err := appendOrderEventFromSource(tx, order.ID, order.Version+1, "payout.simulated", order.Status,
				string(entity.OrderStatusCompleted), "worker", now.UTC()); err != nil {
				return err
			}
		}
		if err := tx.Model(&entity.SEP24Transaction{}).Where("order_id = ?", order.ID).
			Updates(map[string]any{"external_transaction_id": payout.ReferenceID}).Error; err != nil {
			return fmt.Errorf("recording SEP-24 payout reference: %w", err)
		}
		return tx.Model(&entity.OutboxMessage{}).
			Where("id = ? AND topic = ? AND processed_at IS NULL", outboxID, usecase.SandboxPayoutTopic).
			Updates(map[string]any{"processed_at": now.UTC(), "lease_owner": nil, "lease_until": nil, "last_error": nil}).Error
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
		if err := appendOrderEventFromSource(tx, order.ID, order.Version+1, "retirement.failed", order.Status,
			string(entity.OrderStatusRetirementFailed), "worker", now); err != nil {
			return err
		}
		return tx.Model(&entity.OutboxMessage{}).Where("topic = ? AND aggregate_id = ? AND processed_at IS NULL", "stellar.retire_offramp", order.ID).
			Updates(map[string]any{"processed_at": now, "lease_owner": nil, "lease_until": nil, "last_error": safeError}).Error
	})
}

func (r *OfframpRepository) Get(ctx context.Context, principal usecase.OrderPrincipal, orderID string) (usecase.OrderView, error) {
	var order entity.OrderRecord
	query, err := applyOrderOwnership(r.db.WithContext(ctx).Where("id = ?", orderID), principal)
	if err != nil {
		return usecase.OrderView{}, err
	}
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.OrderView{}, usecase.ErrOrderNotFound
		}
		return usecase.OrderView{}, fmt.Errorf("finding order: %w", err)
	}
	return r.orderView(ctx, order)
}

func (r *OfframpRepository) List(ctx context.Context, principal usecase.OrderPrincipal, limit int, cursor string) ([]usecase.OrderView, string, error) {
	query, err := applyOrderOwnership(r.db.WithContext(ctx), principal)
	if err != nil {
		return nil, "", err
	}
	query = query.Order("created_at DESC, id DESC").Limit(limit + 1)
	if cursor != "" {
		var cursorOrder entity.OrderRecord
		cursorQuery, err := applyOrderOwnership(r.db.WithContext(ctx).Select("created_at", "id").Where("id = ?", cursor), principal)
		if err != nil {
			return nil, "", err
		}
		if err := cursorQuery.First(&cursorOrder).Error; err != nil {
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
		view, err := r.orderView(ctx, row)
		if err != nil {
			return nil, "", err
		}
		views = append(views, view)
	}
	return views, next, nil
}

func (r *OfframpRepository) orderView(ctx context.Context, order entity.OrderRecord) (usecase.OrderView, error) {
	view := baseOrderView(order)
	var payout entity.OfframpPayout
	if err := r.db.WithContext(ctx).Where("order_id = ?", order.ID).First(&payout).Error; err == nil {
		simulation := true
		view.Payout = &usecase.PayoutView{Reference: payout.ReferenceID, Method: payout.Method,
			AmountMinor: payout.AmountMinor, State: payout.State, Simulated: &simulation,
			Disclosure: usecase.SandboxPayoutDisclosureForRetirementStatus("")}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding payout: %w", err)
	}
	var retirement entity.StellarTransaction
	if err := r.db.WithContext(ctx).Where("order_id = ? AND purpose = ?", order.ID, "retirement").First(&retirement).Error; err == nil {
		applyOfframpRetirementEvidence(&view, retirement)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding retirement transaction: %w", err)
	}
	var deposit entity.StellarTransaction
	if err := r.db.WithContext(ctx).Where("order_id = ? AND purpose = ?", order.ID, "deposit").First(&deposit).Error; err == nil && deposit.TransactionHash != nil {
		view.DepositTransactionHash = *deposit.TransactionHash
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding deposit transaction: %w", err)
	}
	return view, nil
}

func applyOfframpRetirementEvidence(view *usecase.OrderView, retirement entity.StellarTransaction) {
	status := retirement.Status
	switch retirement.Status {
	case "simulated":
		if retirement.TransactionHash != nil || retirement.LedgerAt != nil {
			status = ""
		}
	case "confirmed":
		if retirement.TransactionHash == nil || retirement.LedgerAt == nil {
			status = ""
		} else {
			view.StellarTransactionHash = *retirement.TransactionHash
		}
	default:
		status = ""
	}
	if view.Payout != nil {
		view.Payout.Disclosure = usecase.SandboxPayoutDisclosureForRetirementStatus(status)
	}
}

func strPtr(value string) *string { return &value }

func intPtr(value int) *int { return &value }

var _ usecase.OfframpRepository = (*OfframpRepository)(nil)
