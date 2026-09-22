package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

const sandboxRevenueDisclosure = "Sandbox figures are simulated estimates; no real fiat revenue or payout is settled."

type DeveloperDashboardRepository struct {
	db *gorm.DB
}

func NewDeveloperDashboardRepository(db *gorm.DB) *DeveloperDashboardRepository {
	return &DeveloperDashboardRepository{db: db}
}

type developerCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeDeveloperCursor(cursor developerCursor) (string, error) {
	if cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return "", errors.New("invalid developer cursor")
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encoding developer cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeDeveloperCursor(value string) (developerCursor, error) {
	if strings.TrimSpace(value) == "" {
		return developerCursor{}, errors.New("invalid developer cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return developerCursor{}, errors.New("invalid developer cursor")
	}
	var cursor developerCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" {
		return developerCursor{}, errors.New("invalid developer cursor")
	}
	return cursor, nil
}

func analyticsBucketExpression(bucket string) (string, bool) {
	switch bucket {
	case "hour":
		return "date_trunc('hour', o.created_at)", true
	case "day":
		return "date_trunc('day', o.created_at)", true
	case "week":
		return "date_trunc('week', o.created_at)", true
	default:
		return "", false
	}
}

func analyticsWebhookBucketExpression(bucket string) (string, bool) {
	switch bucket {
	case "hour":
		return "date_trunc('hour', we.created_at)", true
	case "day":
		return "date_trunc('day', we.created_at)", true
	case "week":
		return "date_trunc('week', we.created_at)", true
	default:
		return "", false
	}
}

func (r *DeveloperDashboardRepository) ownedOrders(ctx context.Context, query usecase.DeveloperQuery) *gorm.DB {
	result := r.db.WithContext(ctx).
		Table("orders AS o").
		Joins("LEFT JOIN api_clients AS c ON c.id = o.client_id").
		Where("(o.created_by_user_id = ? OR c.owner_user_id = ?)", query.OwnerUserID, query.OwnerUserID)
	return applyDeveloperOrderFilters(result, query)
}

func applyDeveloperOrderFilters(query *gorm.DB, filter usecase.DeveloperQuery) *gorm.DB {
	query = query.Where("o.created_at >= ? AND o.created_at < ?", filter.From, filter.To)
	if filter.ClientID != "" {
		query = query.Where("o.client_id = ?", filter.ClientID)
	}
	if filter.Currency == usecase.IDRCurrency {
		query = query.Where("o.currency = ?", usecase.IDRCurrency)
	} else if filter.Currency == usecase.NativeXLMAssetCode {
		query = query.Where("o.asset_code = ?", usecase.NativeXLMAssetCode)
	}
	if filter.Direction != "" {
		query = query.Where("o.direction = ?", filter.Direction)
	}
	if filter.Status != "" {
		query = query.Where("o.status = ?", filter.Status)
	}
	if filter.PaymentMethod != "" {
		query = query.Where("o.payment_method = ?", filter.PaymentMethod)
	}
	return query
}

type developerOverviewRow struct {
	TotalOrders              int64   `gorm:"column:total_orders"`
	ActiveOrders             int64   `gorm:"column:active_orders"`
	CompletedOrders          int64   `gorm:"column:completed_orders"`
	FailedOrders             int64   `gorm:"column:failed_orders"`
	BuyOrders                int64   `gorm:"column:buy_orders"`
	SellOrders               int64   `gorm:"column:sell_orders"`
	GrossIDRMinor            int64   `gorm:"column:gross_idr_minor"`
	AssetVolumeStroops       int64   `gorm:"column:asset_volume_stroops"`
	FeeIDRMinor              int64   `gorm:"column:fee_idr_minor"`
	NetIDRMinor              int64   `gorm:"column:net_idr_minor"`
	AverageCompletionSeconds float64 `gorm:"column:average_completion_seconds"`
}

func (r *DeveloperDashboardRepository) GetOverview(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperOverview, error) {
	var row developerOverviewRow
	err := r.ownedOrders(ctx, query).
		Joins("LEFT JOIN order_financials AS f ON f.order_id = o.id").
		Select(`
			COUNT(DISTINCT o.id) AS total_orders,
			COALESCE(SUM(CASE WHEN o.status NOT IN ('completed', 'expired', 'payment_failed', 'stellar_failed', 'asset_invalid', 'retirement_failed', 'withdrawal_failed', 'cancelled') THEN 1 ELSE 0 END), 0)::bigint AS active_orders,
			COALESCE(SUM(CASE WHEN o.status = 'completed' THEN 1 ELSE 0 END), 0)::bigint AS completed_orders,
			COALESCE(SUM(CASE WHEN o.status IN ('expired', 'payment_failed', 'stellar_failed', 'asset_invalid', 'retirement_failed', 'withdrawal_failed', 'cancelled') THEN 1 ELSE 0 END), 0)::bigint AS failed_orders,
			COALESCE(SUM(CASE WHEN o.direction = 'onramp' THEN 1 ELSE 0 END), 0)::bigint AS buy_orders,
			COALESCE(SUM(CASE WHEN o.direction = 'offramp' THEN 1 ELSE 0 END), 0)::bigint AS sell_orders,
			COALESCE(SUM(f.gross_amount_minor), 0)::bigint AS gross_idr_minor,
			COALESCE(SUM(f.asset_amount_stroops), 0)::bigint AS asset_volume_stroops,
			COALESCE(SUM(f.fee_amount_minor), 0)::bigint AS fee_idr_minor,
			COALESCE(SUM(f.net_amount_minor), 0)::bigint AS net_idr_minor,
			COALESCE(AVG(CASE WHEN o.completed_at IS NOT NULL THEN EXTRACT(EPOCH FROM (o.completed_at - o.created_at)) END), 0) AS average_completion_seconds`).
		Scan(&row).Error
	if err != nil {
		return usecase.DeveloperOverview{}, fmt.Errorf("aggregating developer overview: %w", err)
	}

	pending, exhausted, err := r.webhookDeliveryCounts(ctx, query)
	if err != nil {
		return usecase.DeveloperOverview{}, err
	}
	ordersPage, err := r.ListOrders(ctx, usecase.DeveloperQuery{
		OwnerUserID: query.OwnerUserID,
		ClientID:    query.ClientID,
		From:        query.From,
		To:          query.To,
		Currency:    query.Currency,
		Direction:   query.Direction,
		Status:      query.Status,
		Limit:       query.Limit,
	})
	if err != nil {
		return usecase.DeveloperOverview{}, fmt.Errorf("listing recent developer orders: %w", err)
	}
	denominator := row.CompletedOrders + row.FailedOrders
	successRate := 0.0
	if denominator > 0 {
		successRate = float64(row.CompletedOrders) / float64(denominator) * 100
	}
	return usecase.DeveloperOverview{
		Environment:                "test",
		Network:                    usecase.StellarTestnetNetwork,
		From:                       query.From,
		To:                         query.To,
		TotalOrders:                row.TotalOrders,
		ActiveOrders:               row.ActiveOrders,
		CompletedOrders:            row.CompletedOrders,
		FailedOrders:               row.FailedOrders,
		BuyOrders:                  row.BuyOrders,
		SellOrders:                 row.SellOrders,
		GrossIDRMinor:              row.GrossIDRMinor,
		AssetVolumeStroops:         row.AssetVolumeStroops,
		FeeIDRMinor:                row.FeeIDRMinor,
		NetIDRMinor:                row.NetIDRMinor,
		SuccessRate:                successRate,
		AverageCompletionSeconds:   row.AverageCompletionSeconds,
		PendingWebhookDeliveries:   pending,
		ExhaustedWebhookDeliveries: exhausted,
		RecentOrders:               ordersPage.Items,
	}, nil
}

type developerWebhookCountRow struct {
	Pending   int64 `gorm:"column:pending"`
	Exhausted int64 `gorm:"column:exhausted"`
}

func (r *DeveloperDashboardRepository) webhookDeliveryCounts(ctx context.Context, query usecase.DeveloperQuery) (int64, int64, error) {
	var row developerWebhookCountRow
	result := r.db.WithContext(ctx).
		Table("webhook_attempts AS wa").
		Joins("JOIN webhook_events AS we ON we.id = wa.event_id").
		Joins("JOIN orders AS o ON o.id = we.order_id").
		Joins("LEFT JOIN api_clients AS c ON c.id = o.client_id").
		Where("(o.created_by_user_id = ? OR c.owner_user_id = ?)", query.OwnerUserID, query.OwnerUserID)
	result = applyDeveloperOrderFilters(result, query)
	if err := result.Select(`
		COALESCE(SUM(CASE WHEN wa.status IN ('pending', 'retry_scheduled', 'in_flight') THEN 1 ELSE 0 END), 0)::bigint AS pending,
		COALESCE(SUM(CASE WHEN wa.status = 'exhausted' THEN 1 ELSE 0 END), 0)::bigint AS exhausted`).Scan(&row).Error; err != nil {
		return 0, 0, fmt.Errorf("aggregating webhook delivery counts: %w", err)
	}
	return row.Pending, row.Exhausted, nil
}

type developerAnalyticsRow struct {
	BucketStart              time.Time `gorm:"column:bucket_start"`
	Orders                   int64     `gorm:"column:orders"`
	BuyOrders                int64     `gorm:"column:buy_orders"`
	SellOrders               int64     `gorm:"column:sell_orders"`
	GrossIDRMinor            int64     `gorm:"column:gross_idr_minor"`
	AssetVolumeStroops       int64     `gorm:"column:asset_volume_stroops"`
	FeeIDRMinor              int64     `gorm:"column:fee_idr_minor"`
	NetIDRMinor              int64     `gorm:"column:net_idr_minor"`
	CompletedOrders          int64     `gorm:"column:completed_orders"`
	FailedOrders             int64     `gorm:"column:failed_orders"`
	AverageCompletionSeconds float64   `gorm:"column:average_completion_seconds"`
}

func (r *DeveloperDashboardRepository) GetAnalytics(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperAnalytics, error) {
	expression, ok := analyticsBucketExpression(query.Bucket)
	if !ok {
		return usecase.DeveloperAnalytics{}, usecase.ErrInvalidDeveloperQuery
	}
	var rows []developerAnalyticsRow
	if err := r.ownedOrders(ctx, query).
		Joins("LEFT JOIN order_financials AS f ON f.order_id = o.id").
		Select(fmt.Sprintf(`
			%s AS bucket_start,
			COUNT(DISTINCT o.id)::bigint AS orders,
			COALESCE(SUM(CASE WHEN o.direction = 'onramp' THEN 1 ELSE 0 END), 0)::bigint AS buy_orders,
			COALESCE(SUM(CASE WHEN o.direction = 'offramp' THEN 1 ELSE 0 END), 0)::bigint AS sell_orders,
			COALESCE(SUM(f.gross_amount_minor), 0)::bigint AS gross_idr_minor,
			COALESCE(SUM(f.asset_amount_stroops), 0)::bigint AS asset_volume_stroops,
			COALESCE(SUM(f.fee_amount_minor), 0)::bigint AS fee_idr_minor,
			COALESCE(SUM(f.net_amount_minor), 0)::bigint AS net_idr_minor,
			COALESCE(SUM(CASE WHEN o.status = 'completed' THEN 1 ELSE 0 END), 0)::bigint AS completed_orders,
			COALESCE(SUM(CASE WHEN o.status IN ('expired', 'payment_failed', 'stellar_failed', 'asset_invalid', 'retirement_failed', 'withdrawal_failed', 'cancelled') THEN 1 ELSE 0 END), 0)::bigint AS failed_orders,
			COALESCE(AVG(CASE WHEN o.completed_at IS NOT NULL THEN EXTRACT(EPOCH FROM (o.completed_at - o.created_at)) END), 0) AS average_completion_seconds`, expression)).
		Group("bucket_start").Order("bucket_start").Find(&rows).Error; err != nil {
		return usecase.DeveloperAnalytics{}, fmt.Errorf("aggregating developer analytics: %w", err)
	}
	paymentMethods, err := r.analyticsPaymentMethods(ctx, query, expression)
	if err != nil {
		return usecase.DeveloperAnalytics{}, err
	}
	webhookStats, err := r.analyticsWebhookStats(ctx, query)
	if err != nil {
		return usecase.DeveloperAnalytics{}, err
	}
	buckets := make([]usecase.DeveloperAnalyticsBucket, 0, len(rows))
	duration := analyticsBucketDuration(query.Bucket)
	for _, row := range rows {
		key := row.BucketStart.UTC().Format(time.RFC3339Nano)
		bucket := usecase.DeveloperAnalyticsBucket{
			Start:                    row.BucketStart.UTC(),
			End:                      row.BucketStart.UTC().Add(duration),
			Orders:                   row.Orders,
			BuyOrders:                row.BuyOrders,
			SellOrders:               row.SellOrders,
			GrossIDRMinor:            row.GrossIDRMinor,
			AssetVolumeStroops:       row.AssetVolumeStroops,
			FeeIDRMinor:              row.FeeIDRMinor,
			NetIDRMinor:              row.NetIDRMinor,
			CompletedOrders:          row.CompletedOrders,
			FailedOrders:             row.FailedOrders,
			AverageCompletionSeconds: row.AverageCompletionSeconds,
			PaymentMethods:           paymentMethods[key],
			WebhookDelivered:         webhookStats[key].Delivered,
			WebhookRetried:           webhookStats[key].Retried,
			WebhookExhausted:         webhookStats[key].Exhausted,
		}
		if bucket.PaymentMethods == nil {
			bucket.PaymentMethods = map[string]int64{}
		}
		buckets = append(buckets, bucket)
	}
	return usecase.DeveloperAnalytics{Environment: "test", Network: usecase.StellarTestnetNetwork, From: query.From, To: query.To, Bucket: query.Bucket, Buckets: buckets}, nil
}

func analyticsBucketDuration(bucket string) time.Duration {
	switch bucket {
	case "hour":
		return time.Hour
	case "week":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

type analyticsPaymentMethodRow struct {
	BucketStart   time.Time `gorm:"column:bucket_start"`
	PaymentMethod string    `gorm:"column:payment_method"`
	Count         int64     `gorm:"column:count"`
}

func (r *DeveloperDashboardRepository) analyticsPaymentMethods(ctx context.Context, query usecase.DeveloperQuery, expression string) (map[string]map[string]int64, error) {
	var rows []analyticsPaymentMethodRow
	if err := r.ownedOrders(ctx, query).Select(fmt.Sprintf("%s AS bucket_start, COALESCE(o.payment_method, 'unknown') AS payment_method, COUNT(DISTINCT o.id)::bigint AS count", expression)).
		Group("bucket_start, payment_method").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregating payment methods: %w", err)
	}
	result := make(map[string]map[string]int64, len(rows))
	for _, row := range rows {
		key := row.BucketStart.UTC().Format(time.RFC3339Nano)
		if result[key] == nil {
			result[key] = map[string]int64{}
		}
		result[key][row.PaymentMethod] = row.Count
	}
	return result, nil
}

type developerWebhookStats struct {
	Delivered int64
	Retried   int64
	Exhausted int64
}

func (r *DeveloperDashboardRepository) analyticsWebhookStats(ctx context.Context, query usecase.DeveloperQuery) (map[string]developerWebhookStats, error) {
	expression, ok := analyticsWebhookBucketExpression(query.Bucket)
	if !ok {
		return nil, usecase.ErrInvalidDeveloperQuery
	}
	var rows []struct {
		BucketStart time.Time `gorm:"column:bucket_start"`
		Delivered   int64     `gorm:"column:delivered"`
		Retried     int64     `gorm:"column:retried"`
		Exhausted   int64     `gorm:"column:exhausted"`
	}
	queryDB := r.db.WithContext(ctx).
		Table("webhook_attempts AS wa").
		Joins("JOIN webhook_events AS we ON we.id = wa.event_id").
		Joins("JOIN orders AS o ON o.id = we.order_id").
		Joins("LEFT JOIN api_clients AS c ON c.id = o.client_id").
		Where("(o.created_by_user_id = ? OR c.owner_user_id = ?)", query.OwnerUserID, query.OwnerUserID)
	queryDB = applyDeveloperOrderFilters(queryDB, query)
	if err := queryDB.Select(fmt.Sprintf(`
		%s AS bucket_start,
		COALESCE(SUM(CASE WHEN wa.status = 'delivered' THEN 1 ELSE 0 END), 0)::bigint AS delivered,
		COALESCE(SUM(CASE WHEN wa.status IN ('retry_scheduled', 'in_flight') THEN 1 ELSE 0 END), 0)::bigint AS retried,
		COALESCE(SUM(CASE WHEN wa.status = 'exhausted' THEN 1 ELSE 0 END), 0)::bigint AS exhausted`, expression)).
		Group("bucket_start").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregating webhook analytics: %w", err)
	}
	result := make(map[string]developerWebhookStats, len(rows))
	for _, row := range rows {
		result[row.BucketStart.UTC().Format(time.RFC3339Nano)] = developerWebhookStats{Delivered: row.Delivered, Retried: row.Retried, Exhausted: row.Exhausted}
	}
	return result, nil
}

type developerRevenueSummaryRow struct {
	GrossIDRMinor         int64 `gorm:"column:gross_idr_minor"`
	FeeIDRMinor           int64 `gorm:"column:fee_idr_minor"`
	PlatformRevenueMinor  int64 `gorm:"column:platform_revenue_minor"`
	DeveloperRevenueMinor int64 `gorm:"column:developer_revenue_minor"`
	NetIDRMinor           int64 `gorm:"column:net_idr_minor"`
	EntryCount            int64 `gorm:"column:entry_count"`
}

func (r *DeveloperDashboardRepository) GetRevenueSummary(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperRevenueSummary, error) {
	var row developerRevenueSummaryRow
	if err := r.ownedOrders(ctx, query).
		Joins("JOIN order_financials AS f ON f.order_id = o.id").
		Select(`
			COALESCE(SUM(f.gross_amount_minor), 0)::bigint AS gross_idr_minor,
			COALESCE(SUM(f.fee_amount_minor), 0)::bigint AS fee_idr_minor,
			COALESCE(SUM(f.platform_revenue_minor), 0)::bigint AS platform_revenue_minor,
			COALESCE(SUM(f.developer_revenue_minor), 0)::bigint AS developer_revenue_minor,
			COALESCE(SUM(f.net_amount_minor), 0)::bigint AS net_idr_minor,
			COUNT(DISTINCT f.order_id)::bigint AS entry_count`).Scan(&row).Error; err != nil {
		return usecase.DeveloperRevenueSummary{}, fmt.Errorf("aggregating developer revenue: %w", err)
	}
	return usecase.DeveloperRevenueSummary{
		Environment:           "test",
		Network:               usecase.StellarTestnetNetwork,
		From:                  query.From,
		To:                    query.To,
		GrossIDRMinor:         row.GrossIDRMinor,
		FeeIDRMinor:           row.FeeIDRMinor,
		PlatformRevenueMinor:  row.PlatformRevenueMinor,
		DeveloperRevenueMinor: row.DeveloperRevenueMinor,
		NetIDRMinor:           row.NetIDRMinor,
		EntryCount:            row.EntryCount,
		Simulated:             true,
		Disclosure:            sandboxRevenueDisclosure,
	}, nil
}

type developerRevenueEntryRow struct {
	OrderID               string    `gorm:"column:order_id"`
	ClientID              string    `gorm:"column:client_id"`
	Direction             string    `gorm:"column:direction"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	GrossIDRMinor         int64     `gorm:"column:gross_idr_minor"`
	FeeIDRMinor           int64     `gorm:"column:fee_idr_minor"`
	PlatformRevenueMinor  int64     `gorm:"column:platform_revenue_minor"`
	DeveloperRevenueMinor int64     `gorm:"column:developer_revenue_minor"`
	NetIDRMinor           int64     `gorm:"column:net_idr_minor"`
	AssetAmount           string    `gorm:"column:asset_amount"`
	AssetAmountStroops    int64     `gorm:"column:asset_amount_stroops"`
	FeeCurrency           string    `gorm:"column:fee_currency"`
	FeePolicyVersion      string    `gorm:"column:fee_policy_version"`
	Source                string    `gorm:"column:source"`
	Simulated             bool      `gorm:"column:simulated"`
}

func (r *DeveloperDashboardRepository) ListRevenueEntries(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperRevenueEntry], error) {
	var cursor developerCursor
	if query.Cursor != "" {
		var err error
		cursor, err = decodeDeveloperCursor(query.Cursor)
		if err != nil {
			return usecase.DeveloperPage[usecase.DeveloperRevenueEntry]{}, usecase.ErrInvalidDeveloperQuery
		}
	}
	queryDB := r.ownedOrders(ctx, query).Joins("JOIN order_financials AS f ON f.order_id = o.id")
	if query.Cursor != "" {
		queryDB = queryDB.Where("(f.created_at < ? OR (f.created_at = ? AND f.order_id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	var rows []developerRevenueEntryRow
	if err := queryDB.Select(`
		f.order_id::text AS order_id,
		COALESCE(f.client_id::text, '') AS client_id,
		f.direction AS direction,
		f.created_at AS created_at,
		f.gross_amount_minor AS gross_idr_minor,
		f.fee_amount_minor AS fee_idr_minor,
		f.platform_revenue_minor AS platform_revenue_minor,
		f.developer_revenue_minor AS developer_revenue_minor,
		f.net_amount_minor AS net_idr_minor,
		f.asset_amount AS asset_amount,
		f.asset_amount_stroops AS asset_amount_stroops,
		f.fee_currency AS fee_currency,
		f.fee_policy_version AS fee_policy_version,
		f.source AS source,
		f.simulated AS simulated`).
		Order("f.created_at DESC, f.order_id DESC").Limit(query.Limit + 1).Find(&rows).Error; err != nil {
		return usecase.DeveloperPage[usecase.DeveloperRevenueEntry]{}, fmt.Errorf("listing developer revenue entries: %w", err)
	}
	page := usecase.DeveloperPage[usecase.DeveloperRevenueEntry]{Items: make([]usecase.DeveloperRevenueEntry, 0, min(len(rows), query.Limit))}
	if len(rows) > query.Limit {
		last := rows[query.Limit-1]
		nextCursor, err := encodeDeveloperCursor(developerCursor{CreatedAt: last.CreatedAt, ID: last.OrderID})
		if err != nil {
			return usecase.DeveloperPage[usecase.DeveloperRevenueEntry]{}, err
		}
		page.NextCursor = nextCursor
		rows = rows[:query.Limit]
	}
	for _, row := range rows {
		page.Items = append(page.Items, revenueEntryFromRow(row))
	}
	return page, nil
}

func revenueEntryFromRow(row developerRevenueEntryRow) usecase.DeveloperRevenueEntry {
	return usecase.DeveloperRevenueEntry{
		OrderID:               row.OrderID,
		ClientID:              row.ClientID,
		Direction:             row.Direction,
		CreatedAt:             row.CreatedAt.UTC(),
		GrossIDRMinor:         row.GrossIDRMinor,
		FeeIDRMinor:           row.FeeIDRMinor,
		PlatformRevenueMinor:  row.PlatformRevenueMinor,
		DeveloperRevenueMinor: row.DeveloperRevenueMinor,
		NetIDRMinor:           row.NetIDRMinor,
		AssetAmount:           row.AssetAmount,
		AssetAmountStroops:    row.AssetAmountStroops,
		FeeCurrency:           row.FeeCurrency,
		FeePolicyVersion:      row.FeePolicyVersion,
		Source:                row.Source,
		Simulated:             row.Simulated,
	}
}

type developerOrderRow struct {
	ID                     string     `gorm:"column:id"`
	ClientID               string     `gorm:"column:client_id"`
	ClientName             string     `gorm:"column:client_name"`
	Environment            string     `gorm:"column:environment"`
	Direction              string     `gorm:"column:direction"`
	Status                 string     `gorm:"column:status"`
	Currency               string     `gorm:"column:currency"`
	FiatAmountMinor        int64      `gorm:"column:fiat_amount_minor"`
	AssetAmount            string     `gorm:"column:asset_amount"`
	AssetAmountStroops     int64      `gorm:"column:asset_amount_stroops"`
	PaymentMethod          string     `gorm:"column:payment_method"`
	GatewayProvider        string     `gorm:"column:gateway_provider"`
	GatewayReference       string     `gorm:"column:gateway_reference"`
	StellarIntentID        string     `gorm:"column:stellar_intent_id"`
	StellarTransactionHash string     `gorm:"column:stellar_transaction_hash"`
	SEP24TransactionID     string     `gorm:"column:sep24_transaction_id"`
	QuoteID                string     `gorm:"column:quote_id"`
	WalletAccount          string     `gorm:"column:wallet_account"`
	PayoutReference        string     `gorm:"column:payout_reference"`
	PayoutSimulated        bool       `gorm:"column:payout_simulated"`
	LatestEventType        string     `gorm:"column:latest_event_type"`
	LatestTransitionAt     *time.Time `gorm:"column:latest_transition_at"`
	CreatedAt              time.Time  `gorm:"column:created_at"`
	UpdatedAt              time.Time  `gorm:"column:updated_at"`
	CompletedAt            *time.Time `gorm:"column:completed_at"`
	FailureCode            string     `gorm:"column:failure_code"`
	FinancialOrderID       string     `gorm:"column:financial_order_id"`
	GrossIDRMinor          int64      `gorm:"column:gross_idr_minor"`
	FeeIDRMinor            int64      `gorm:"column:fee_idr_minor"`
	PlatformRevenueMinor   int64      `gorm:"column:platform_revenue_minor"`
	DeveloperRevenueMinor  int64      `gorm:"column:developer_revenue_minor"`
	NetIDRMinor            int64      `gorm:"column:net_idr_minor"`
	FinancialAssetAmount   string     `gorm:"column:financial_asset_amount"`
	FinancialAssetStroops  int64      `gorm:"column:financial_asset_stroops"`
	FeeCurrency            string     `gorm:"column:fee_currency"`
	FeePolicyVersion       string     `gorm:"column:fee_policy_version"`
	FinancialSource        string     `gorm:"column:financial_source"`
	FinancialSimulated     bool       `gorm:"column:financial_simulated"`
}

func (r *DeveloperDashboardRepository) ListOrders(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperOrder], error) {
	var cursor developerCursor
	if query.Cursor != "" {
		var err error
		cursor, err = decodeDeveloperCursor(query.Cursor)
		if err != nil {
			return usecase.DeveloperPage[usecase.DeveloperOrder]{}, usecase.ErrInvalidDeveloperQuery
		}
	}
	queryDB := r.ownedOrders(ctx, query).
		Joins("LEFT JOIN order_financials AS f ON f.order_id = o.id").
		Joins("LEFT JOIN offramp_payouts AS p ON p.order_id = o.id")
	if query.Cursor != "" {
		queryDB = queryDB.Where("(o.created_at < ? OR (o.created_at = ? AND o.id < ?))", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	var rows []developerOrderRow
	if err := queryDB.Select(`
		o.id AS id,
		COALESCE(o.client_id::text, '') AS client_id,
		COALESCE(c.name, '') AS client_name,
		COALESCE(c.environment, 'test') AS environment,
		o.direction AS direction,
		o.status AS status,
		o.currency AS currency,
		o.fiat_amount_minor AS fiat_amount_minor,
		o.asset_amount AS asset_amount,
		o.asset_amount_stroops AS asset_amount_stroops,
		COALESCE(o.payment_method, '') AS payment_method,
		COALESCE(o.gateway_provider, '') AS gateway_provider,
		COALESCE((SELECT pc.provider_checkout_id FROM payment_checkouts pc WHERE pc.order_id = o.id ORDER BY pc.created_at DESC LIMIT 1), '') AS gateway_reference,
		COALESCE((SELECT st.intent_id FROM stellar_transactions st WHERE st.order_id = o.id ORDER BY st.created_at DESC LIMIT 1), '') AS stellar_intent_id,
		COALESCE((SELECT st.transaction_hash FROM stellar_transactions st WHERE st.order_id = o.id AND st.transaction_hash IS NOT NULL ORDER BY st.created_at DESC LIMIT 1), '') AS stellar_transaction_hash,
		COALESCE((SELECT s24.transaction_id FROM sep24_transactions s24 WHERE s24.order_id = o.id ORDER BY s24.created_at DESC LIMIT 1), '') AS sep24_transaction_id,
		COALESCE(o.quote_id, '') AS quote_id,
		COALESCE(o.wallet_account, '') AS wallet_account,
		COALESCE(p.reference_id, '') AS payout_reference,
		(p.id IS NOT NULL) AS payout_simulated,
		COALESCE((SELECT oe.event_type FROM order_events oe WHERE oe.order_id = o.id ORDER BY oe.aggregate_version DESC LIMIT 1), '') AS latest_event_type,
		(SELECT oe.created_at FROM order_events oe WHERE oe.order_id = o.id ORDER BY oe.aggregate_version DESC LIMIT 1) AS latest_transition_at,
		o.created_at AS created_at,
		o.updated_at AS updated_at,
		o.completed_at AS completed_at,
		COALESCE(o.failure_code, '') AS failure_code,
		COALESCE(f.order_id::text, '') AS financial_order_id,
		COALESCE(f.gross_amount_minor, 0)::bigint AS gross_idr_minor,
		COALESCE(f.fee_amount_minor, 0)::bigint AS fee_idr_minor,
		COALESCE(f.platform_revenue_minor, 0)::bigint AS platform_revenue_minor,
		COALESCE(f.developer_revenue_minor, 0)::bigint AS developer_revenue_minor,
		COALESCE(f.net_amount_minor, 0)::bigint AS net_idr_minor,
		COALESCE(f.asset_amount, '') AS financial_asset_amount,
		COALESCE(f.asset_amount_stroops, 0)::bigint AS financial_asset_stroops,
		COALESCE(f.fee_currency, '') AS fee_currency,
		COALESCE(f.fee_policy_version, '') AS fee_policy_version,
		COALESCE(f.source, '') AS financial_source,
		COALESCE(f.simulated, false) AS financial_simulated`).
		Order("o.created_at DESC, o.id DESC").Limit(query.Limit + 1).Find(&rows).Error; err != nil {
		return usecase.DeveloperPage[usecase.DeveloperOrder]{}, fmt.Errorf("listing developer orders: %w", err)
	}
	page := usecase.DeveloperPage[usecase.DeveloperOrder]{Items: make([]usecase.DeveloperOrder, 0, min(len(rows), query.Limit))}
	if len(rows) > query.Limit {
		last := rows[query.Limit-1]
		nextCursor, err := encodeDeveloperCursor(developerCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return usecase.DeveloperPage[usecase.DeveloperOrder]{}, err
		}
		page.NextCursor = nextCursor
		rows = rows[:query.Limit]
	}
	for _, row := range rows {
		page.Items = append(page.Items, developerOrderFromRow(row))
	}
	return page, nil
}

func developerOrderFromRow(row developerOrderRow) usecase.DeveloperOrder {
	order := usecase.DeveloperOrder{
		ID:                     row.ID,
		ClientID:               row.ClientID,
		ClientName:             row.ClientName,
		Environment:            row.Environment,
		Direction:              row.Direction,
		Status:                 row.Status,
		Currency:               row.Currency,
		FiatAmountMinor:        row.FiatAmountMinor,
		AssetAmount:            row.AssetAmount,
		AssetAmountStroops:     row.AssetAmountStroops,
		PaymentMethod:          row.PaymentMethod,
		GatewayProvider:        row.GatewayProvider,
		GatewayReference:       row.GatewayReference,
		StellarIntentID:        row.StellarIntentID,
		StellarTransactionHash: row.StellarTransactionHash,
		SEP24TransactionID:     row.SEP24TransactionID,
		QuoteID:                row.QuoteID,
		WalletAccount:          row.WalletAccount,
		PayoutReference:        row.PayoutReference,
		PayoutSimulated:        row.PayoutSimulated,
		LatestEventType:        row.LatestEventType,
		CreatedAt:              row.CreatedAt.UTC(),
		UpdatedAt:              row.UpdatedAt.UTC(),
		CompletedAt:            row.CompletedAt,
		FailureCode:            row.FailureCode,
	}
	if row.LatestTransitionAt != nil {
		order.LatestTransitionAt = row.LatestTransitionAt.UTC()
	}
	if row.PayoutReference != "" {
		order.PayoutDisclosure = usecase.SandboxPayoutDisclosure
	}
	if row.FinancialOrderID != "" {
		order.Financial = &usecase.DeveloperRevenueEntry{
			OrderID:               row.FinancialOrderID,
			ClientID:              row.ClientID,
			Direction:             row.Direction,
			CreatedAt:             row.CreatedAt.UTC(),
			GrossIDRMinor:         row.GrossIDRMinor,
			FeeIDRMinor:           row.FeeIDRMinor,
			PlatformRevenueMinor:  row.PlatformRevenueMinor,
			DeveloperRevenueMinor: row.DeveloperRevenueMinor,
			NetIDRMinor:           row.NetIDRMinor,
			AssetAmount:           row.FinancialAssetAmount,
			AssetAmountStroops:    row.FinancialAssetStroops,
			FeeCurrency:           row.FeeCurrency,
			FeePolicyVersion:      row.FeePolicyVersion,
			Source:                row.FinancialSource,
			Simulated:             row.FinancialSimulated,
		}
	}
	return order
}
