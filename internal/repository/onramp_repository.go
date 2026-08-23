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

const onrampCreateOperation = "onramp.create"

type OnrampRepository struct {
	db              *gorm.DB
	tx              txManager
	treasuryAccount string
	network         string
	operatingBuffer entity.Stroops
}

func NewOnrampRepository(db *gorm.DB, treasuryAccount, network string, operatingBuffer entity.Stroops) *OnrampRepository {
	return &OnrampRepository{db: db, tx: newTxManager(db), treasuryAccount: treasuryAccount, network: network, operatingBuffer: operatingBuffer}
}

func (r *OnrampRepository) FindReplay(ctx context.Context, clientID, keyHash, requestHash string) (usecase.OrderView, bool, error) {
	var record entity.IdempotencyRecord
	err := r.db.WithContext(ctx).Where("client_id = ? AND operation = ? AND key_hash = ?", clientID, onrampCreateOperation, keyHash).First(&record).Error
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
		return usecase.OrderView{}, false, fmt.Errorf("idempotency record has no order: %w", usecase.ErrCheckoutUnknown)
	}
	view, err := r.Get(ctx, clientID, *record.CreatedResourceID)
	if err != nil {
		return usecase.OrderView{}, false, err
	}
	return view, true, nil
}

func (r *OnrampRepository) ReserveAndCreate(ctx context.Context, record usecase.CreateRecord, observedBalance entity.Stroops) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		treasury, err := r.lockTreasury(ctx, tx, observedBalance, record.CreatedAt)
		if err != nil {
			return err
		}
		available := entity.Stroops(treasury.ObservedBalanceStroops - treasury.ReservedStroops - treasury.OperatingBufferStroops)
		if available < record.Quote.AssetAmount {
			return usecase.ErrInsufficientLiquidity
		}

		order := entity.OrderRecord{
			ID: record.OrderID, ClientID: &record.ClientID, Direction: "onramp", Status: string(entity.OrderStatusCreated), Version: 1,
			Currency: "IDR", FiatAmountMinor: int64(record.Quote.FiatAmount), AssetCode: "XLM", AssetIssuer: "",
			Network: r.network, AssetAmount: record.Quote.AssetAmount.String(), AssetAmountStroops: int64(record.Quote.AssetAmount),
			QuoteProvider: "coinmarketcap", QuoteSourceAt: record.Quote.SourceAt, QuoteRate: record.Quote.Rate,
			QuoteAdjustedRate: record.Quote.AdjustedRate, QuoteSpreadBPS: record.Quote.SpreadBPS, QuoteExpiresAt: record.Quote.ExpiresAt,
			PaymentMethod: string(record.PaymentMethod), GatewayProvider: "xendit", StellarSource: &r.treasuryAccount,
			StellarDestination: &record.Destination, CreatedAt: record.CreatedAt, UpdatedAt: record.CreatedAt,
		}
		if record.Memo != "" {
			order.StellarMemo = &record.Memo
		}
		if err := tx.Create(&order).Error; err != nil {
			return fmt.Errorf("creating order: %w", err)
		}
		reservationID, err := platform.NewID()
		if err != nil {
			return err
		}
		reservation := entity.TreasuryReservation{ID: reservationID, TreasuryID: treasury.ID, OrderID: order.ID,
			AmountStroops: int64(record.Quote.AssetAmount), Status: "reserved", ExpiresAt: record.Quote.ExpiresAt,
			CreatedAt: record.CreatedAt, UpdatedAt: record.CreatedAt}
		if err := tx.Create(&reservation).Error; err != nil {
			return fmt.Errorf("creating treasury reservation: %w", err)
		}
		if err := tx.Model(&entity.TreasuryAccount{}).Where("id = ?", treasury.ID).
			Update("reserved_stroops", gorm.Expr("reserved_stroops + ?", record.Quote.AssetAmount)).Error; err != nil {
			return fmt.Errorf("reserving treasury balance: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, 1, "order.created", "", string(entity.OrderStatusCreated), record.CreatedAt); err != nil {
			return err
		}
		idempotencyID, err := platform.NewID()
		if err != nil {
			return err
		}
		idempotency := entity.IdempotencyRecord{ID: idempotencyID, ClientID: record.ClientID, Operation: onrampCreateOperation,
			KeyHash: record.IdempotencyKeyHash, RequestHash: record.RequestHash, CreatedResourceID: &order.ID,
			State: "processing", ExpiresAt: record.CreatedAt.Add(24 * time.Hour), CreatedAt: record.CreatedAt, UpdatedAt: record.CreatedAt}
		if err := tx.Create(&idempotency).Error; err != nil {
			return fmt.Errorf("creating idempotency record: %w", err)
		}
		return nil
	})
}

func (r *OnrampRepository) AttachCheckout(ctx context.Context, orderID string, checkout usecase.Checkout) (usecase.OrderView, error) {
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking order: %w", err)
		}
		if order.Status != string(entity.OrderStatusCreated) {
			return entity.ErrInvalidOrderState
		}
		now := time.Now().UTC()
		checkoutID, err := platform.NewID()
		if err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]string{"presentation_type": checkout.PresentationType, "presentation_value": checkout.PresentationValue})
		presentation := checkout.PresentationValue
		row := entity.PaymentCheckout{ID: checkoutID, OrderID: orderID, Provider: "xendit", ProviderCheckoutID: checkout.ProviderID,
			Method: string(checkout.Method), Currency: "IDR", AmountMinor: order.FiatAmountMinor, Status: checkout.Status,
			PresentationReference: &presentation, ExpiresAt: checkout.ExpiresAt, Metadata: metadata, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("creating payment checkout: %w", err)
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).
			Updates(map[string]any{"status": entity.OrderStatusPaymentPending, "version": order.Version + 1, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("moving order to payment pending: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "checkout.created", order.Status, string(entity.OrderStatusPaymentPending), now); err != nil {
			return err
		}
		return tx.Model(&entity.IdempotencyRecord{}).Where("created_resource_id = ? AND operation = ?", order.ID, onrampCreateOperation).
			Updates(map[string]any{"state": "completed", "response_status": 201, "updated_at": now}).Error
	})
	if err != nil {
		return usecase.OrderView{}, err
	}
	var order entity.OrderRecord
	if err := r.db.WithContext(ctx).Where("id = ?", orderID).First(&order).Error; err != nil {
		return usecase.OrderView{}, err
	}
	return r.Get(ctx, *order.ClientID, orderID)
}

func (r *OnrampRepository) FailCheckout(ctx context.Context, orderID, reason string) error {
	return r.releaseFailedOrder(ctx, orderID, entity.OrderStatusPaymentFailed, "checkout_failed", reason)
}

func (r *OnrampRepository) MarkCheckoutUnknown(ctx context.Context, orderID, reason string) error {
	return r.db.WithContext(ctx).Model(&entity.OrderRecord{}).Where("id = ? AND status = ?", orderID, entity.OrderStatusCreated).
		Updates(map[string]any{"failure_code": "checkout_unknown", "failure_stage": "payment", "failure_retryable": true, "updated_at": time.Now().UTC()}).Error
}

func (r *OnrampRepository) RecordCallbackReceipt(ctx context.Context, receipt usecase.CallbackReceipt) (bool, error) {
	var existing entity.GatewayEvent
	err := r.db.WithContext(ctx).Where("provider = ? AND provider_event_id = ?", "xendit", receipt.EventID).First(&existing).Error
	if err == nil {
		if existing.PayloadHash != receipt.PayloadHash {
			return false, usecase.ErrPaymentMismatch
		}
		return existing.ProcessedAt != nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("finding callback receipt: %w", err)
	}
	id, err := platform.NewID()
	if err != nil {
		return false, err
	}
	providerReference := receipt.PaymentRequestID
	row := entity.GatewayEvent{ID: id, Provider: "xendit", ProviderEventID: receipt.EventID, EventType: receipt.EventType,
		CheckoutReference: &providerReference, PayloadHash: receipt.PayloadHash, SignatureVerified: true,
		MatchingResult: "pending", ReceivedAt: time.Now().UTC(), ProcessingStatus: "received"}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return false, fmt.Errorf("creating callback receipt: %w", err)
	}
	return false, nil
}

func (r *OnrampRepository) ExpectedPayment(ctx context.Context, providerID string) (usecase.ExpectedPayment, error) {
	var checkout entity.PaymentCheckout
	if err := r.db.WithContext(ctx).Where("provider = ? AND provider_checkout_id = ?", "xendit", providerID).First(&checkout).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.ExpectedPayment{}, usecase.ErrOrderNotFound
		}
		return usecase.ExpectedPayment{}, fmt.Errorf("finding expected checkout: %w", err)
	}
	var order entity.OrderRecord
	if err := r.db.WithContext(ctx).Where("id = ?", checkout.OrderID).First(&order).Error; err != nil {
		return usecase.ExpectedPayment{}, fmt.Errorf("finding expected order: %w", err)
	}
	channel := "QRIS"
	if checkout.Method == string(usecase.PaymentMethodBRIVA) {
		channel = "BRI_VIRTUAL_ACCOUNT"
	}
	return usecase.ExpectedPayment{OrderID: order.ID, ProviderID: checkout.ProviderCheckoutID, Amount: entity.IDR(checkout.AmountMinor),
		Currency: checkout.Currency, Channel: channel, AssetAmount: entity.Stroops(order.AssetAmountStroops)}, nil
}

func (r *OnrampRepository) ConfirmPaymentAndEnqueue(ctx context.Context, confirmation usecase.PaymentConfirmation) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", confirmation.Expected.OrderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking paid order: %w", err)
		}
		if order.Status == string(entity.OrderStatusStellarProcessing) || order.Status == string(entity.OrderStatusCompleted) {
			return nil
		}
		if order.Status != string(entity.OrderStatusPaymentPending) {
			return entity.ErrInvalidOrderState
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).Updates(map[string]any{
			"status": entity.OrderStatusStellarProcessing, "version": order.Version + 2, "updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("moving paid order to settlement: %w", err)
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+1, "payment.confirmed", order.Status, string(entity.OrderStatusPaymentConfirmed), now); err != nil {
			return err
		}
		if err := appendOrderEvent(tx, order.ID, order.Version+2, "stellar.transfer_requested", string(entity.OrderStatusPaymentConfirmed), string(entity.OrderStatusStellarProcessing), now); err != nil {
			return err
		}
		intentRowID, err := platform.NewID()
		if err != nil {
			return err
		}
		intentID := "stellar-onramp-" + order.ID
		stellar := entity.StellarTransaction{ID: intentRowID, OrderID: order.ID, IntentID: intentID, Purpose: "transfer", Network: order.Network,
			AssetCode: "XLM", Amount: order.AssetAmount, Source: order.StellarSource, Destination: order.StellarDestination,
			Memo: order.StellarMemo, Status: "pending", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&stellar).Error; err != nil {
			return fmt.Errorf("creating settlement intent: %w", err)
		}
		outboxID, err := platform.NewID()
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"order_id": order.ID, "intent_id": intentID})
		outbox := entity.OutboxMessage{ID: outboxID, Topic: "stellar.settle_onramp", AggregateType: "order", AggregateID: order.ID,
			Payload: payload, CreatedAt: now, AvailableAt: now}
		if err := tx.Create(&outbox).Error; err != nil {
			return fmt.Errorf("creating settlement outbox message: %w", err)
		}
		return tx.Model(&entity.GatewayEvent{}).Where("provider = ? AND provider_event_id = ?", "xendit", confirmation.EventID).
			Updates(map[string]any{"order_reference": order.ID, "matching_result": "matched", "processing_status": "processing"}).Error
	})
}

func (r *OnrampRepository) CompleteCallback(ctx context.Context, eventID, result string) error {
	now := time.Now().UTC()
	status := "processed"
	if result != "processed" {
		status = "rejected"
	}
	return r.db.WithContext(ctx).Model(&entity.GatewayEvent{}).Where("provider = ? AND provider_event_id = ?", "xendit", eventID).
		Updates(map[string]any{"matching_result": result, "processing_status": status, "processed_at": now}).Error
}

func (r *OnrampRepository) Get(ctx context.Context, clientID, orderID string) (usecase.OrderView, error) {
	var order entity.OrderRecord
	result := r.db.WithContext(ctx).Raw(`
		SELECT
			id,
			client_id,
			created_by_user_id,
			retail_session_id,
			direction,
			status,
			version,
			currency,
			fiat_amount_minor,
			asset_code,
			asset_issuer,
			network,
			asset_amount,
			asset_amount_stroops,
			quote_provider,
			quote_source_at,
			quote_rate,
			quote_adjusted_rate,
			quote_spread_bps,
			quote_expires_at,
			payment_method,
			gateway_provider,
			stellar_source,
			stellar_destination,
			stellar_memo,
			withdrawal_destination,
			expires_at,
			failure_code,
			failure_stage,
			failure_retryable,
			created_at,
			updated_at,
			completed_at
		FROM orders
		WHERE id = ? AND client_id = ?
	`, orderID, clientID).Scan(&order)
	if result.Error != nil {
		return usecase.OrderView{}, fmt.Errorf("finding order: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return usecase.OrderView{}, usecase.ErrOrderNotFound
	}
	return r.orderView(ctx, order)
}

func (r *OnrampRepository) List(ctx context.Context, clientID string, limit int, cursor string) ([]usecase.OrderView, string, error) {
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
		view, err := r.orderView(ctx, row)
		if err != nil {
			return nil, "", err
		}
		views = append(views, view)
	}
	return views, next, nil
}

func (r *OnrampRepository) lockTreasury(ctx context.Context, tx *gorm.DB, observed entity.Stroops, now time.Time) (entity.TreasuryAccount, error) {
	var treasury entity.TreasuryAccount
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("network = ? AND public_account = ?", r.network, r.treasuryAccount).First(&treasury).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		id, idErr := platform.NewID()
		if idErr != nil {
			return entity.TreasuryAccount{}, idErr
		}
		treasury = entity.TreasuryAccount{ID: id, Network: r.network, PublicAccount: r.treasuryAccount,
			ObservedBalanceStroops: int64(observed), OperatingBufferStroops: int64(r.operatingBuffer), LastReconciledAt: now,
			CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&treasury).Error; err != nil {
			return entity.TreasuryAccount{}, fmt.Errorf("creating treasury account: %w", err)
		}
		return treasury, nil
	}
	if err != nil {
		return entity.TreasuryAccount{}, fmt.Errorf("locking treasury account: %w", err)
	}
	treasury.ObservedBalanceStroops = int64(observed)
	treasury.LastReconciledAt = now
	treasury.UpdatedAt = now
	if err := tx.Save(&treasury).Error; err != nil {
		return entity.TreasuryAccount{}, fmt.Errorf("reconciling treasury account: %w", err)
	}
	return treasury, nil
}

func (r *OnrampRepository) releaseFailedOrder(ctx context.Context, orderID string, status entity.OrderStatus, code, reason string) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var order entity.OrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&order).Error; err != nil {
			return fmt.Errorf("locking failed order: %w", err)
		}
		if order.Status != string(entity.OrderStatusCreated) {
			return entity.ErrInvalidOrderState
		}
		var reservation entity.TreasuryReservation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_id = ? AND status = ?", orderID, "reserved").First(&reservation).Error; err != nil {
			return fmt.Errorf("locking treasury reservation: %w", err)
		}
		now := time.Now().UTC()
		if err := tx.Model(&entity.TreasuryReservation{}).Where("id = ?", reservation.ID).
			Updates(map[string]any{"status": "released", "released_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.TreasuryAccount{}).Where("id = ?", reservation.TreasuryID).
			Update("reserved_stroops", gorm.Expr("reserved_stroops - ?", reservation.AmountStroops)).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.OrderRecord{}).Where("id = ? AND version = ?", order.ID, order.Version).Updates(map[string]any{
			"status": status, "failure_code": code, "failure_stage": "payment", "failure_retryable": false,
			"updated_at": now, "version": order.Version + 1,
		}).Error; err != nil {
			return fmt.Errorf("failing order: %w", err)
		}
		return appendOrderEvent(tx, order.ID, order.Version+1, "checkout.failed", order.Status, string(status), now)
	})
}

func appendOrderEvent(tx *gorm.DB, orderID string, version int, eventType, previous, next string, now time.Time) error {
	id, err := platform.NewID()
	if err != nil {
		return err
	}
	var previousPtr, nextPtr *string
	if previous != "" {
		previousPtr = &previous
	}
	if next != "" {
		nextPtr = &next
	}
	event := entity.OrderEvent{ID: id, OrderID: orderID, AggregateVersion: version, EventType: eventType,
		PreviousStatus: previousPtr, NewStatus: nextPtr, Source: "api", CorrelationID: orderID, Metadata: []byte(`{}`), CreatedAt: now}
	if err := tx.Create(&event).Error; err != nil {
		return fmt.Errorf("appending order event: %w", err)
	}
	return nil
}

func (r *OnrampRepository) orderView(ctx context.Context, order entity.OrderRecord) (usecase.OrderView, error) {
	view := usecase.OrderView{ID: order.ID, Status: entity.OrderStatus(order.Status), FiatAmountMinor: entity.IDR(order.FiatAmountMinor),
		AssetAmount: entity.Stroops(order.AssetAmountStroops), QuoteRate: order.QuoteRate, QuoteAdjustedRate: order.QuoteAdjustedRate,
		QuoteSpreadBPS: order.QuoteSpreadBPS, QuoteSourceAt: order.QuoteSourceAt, QuoteExpiresAt: order.QuoteExpiresAt,
		PaymentMethod: entity.PaymentMethod(order.PaymentMethod), CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt}
	if order.StellarDestination != nil {
		view.StellarDestination = *order.StellarDestination
	}
	if order.StellarMemo != nil {
		view.StellarMemo = *order.StellarMemo
	}
	if order.FailureCode != nil {
		view.FailureCode = *order.FailureCode
	}
	var checkout entity.PaymentCheckout
	if err := r.db.WithContext(ctx).Where("order_id = ?", order.ID).Order("created_at DESC").First(&checkout).Error; err == nil {
		presentation := ""
		presentationType := ""
		if checkout.PresentationReference != nil {
			presentation = *checkout.PresentationReference
		}
		var metadata struct {
			PresentationType string `json:"presentation_type"`
		}
		if json.Unmarshal(checkout.Metadata, &metadata) == nil {
			presentationType = metadata.PresentationType
		}
		view.Checkout = &usecase.Checkout{ProviderID: checkout.ProviderCheckoutID, Method: entity.PaymentMethod(checkout.Method), Status: checkout.Status,
			PresentationType: presentationType, PresentationValue: presentation, ExpiresAt: checkout.ExpiresAt}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding checkout: %w", err)
	}
	var stellar entity.StellarTransaction
	if err := r.db.WithContext(ctx).Where("order_id = ? AND purpose = ?", order.ID, "transfer").First(&stellar).Error; err == nil && stellar.TransactionHash != nil {
		view.StellarTransactionHash = *stellar.TransactionHash
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.OrderView{}, fmt.Errorf("finding stellar transaction: %w", err)
	}
	return view, nil
}
