package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type DeveloperDashboardService interface {
	Overview(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperOverview, error)
	Analytics(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperAnalytics, error)
	RevenueSummary(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperRevenueSummary, error)
	RevenueEntries(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperRevenueEntry], error)
	Orders(ctx context.Context, query usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperOrder], error)
}

type DeveloperDashboardHandler struct {
	service DeveloperDashboardService
	logger  *slog.Logger
}

func NewDeveloperDashboardHandler(service DeveloperDashboardService, logger *slog.Logger) *DeveloperDashboardHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DeveloperDashboardHandler{service: service, logger: logger}
}

func (h *DeveloperDashboardHandler) Overview(c *gin.Context) {
	query, ok := h.query(c)
	if !ok {
		return
	}
	result, err := h.service.Overview(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, "reading developer overview", err)
		return
	}
	c.JSON(http.StatusOK, developerOverviewResponseFrom(result))
}

func (h *DeveloperDashboardHandler) Analytics(c *gin.Context) {
	query, ok := h.query(c)
	if !ok {
		return
	}
	result, err := h.service.Analytics(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, "reading developer analytics", err)
		return
	}
	c.JSON(http.StatusOK, developerAnalyticsResponseFrom(result))
}

func (h *DeveloperDashboardHandler) RevenueSummary(c *gin.Context) {
	query, ok := h.query(c)
	if !ok {
		return
	}
	result, err := h.service.RevenueSummary(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, "reading developer revenue summary", err)
		return
	}
	c.JSON(http.StatusOK, developerRevenueSummaryResponseFrom(result))
}

func (h *DeveloperDashboardHandler) RevenueEntries(c *gin.Context) {
	query, ok := h.query(c)
	if !ok {
		return
	}
	page, err := h.service.RevenueEntries(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, "listing developer revenue entries", err)
		return
	}
	entries := make([]developerRevenueEntryResponse, 0, len(page.Items))
	for _, entry := range page.Items {
		entries = append(entries, developerRevenueEntryResponseFrom(entry))
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "paging_id": page.NextCursor})
}

func (h *DeveloperDashboardHandler) Orders(c *gin.Context) {
	query, ok := h.query(c)
	if !ok {
		return
	}
	page, err := h.service.Orders(c.Request.Context(), query)
	if err != nil {
		h.writeError(c, "listing developer orders", err)
		return
	}
	orders := make([]developerOrderResponse, 0, len(page.Items))
	for _, order := range page.Items {
		orders = append(orders, developerOrderResponseFrom(order))
	}
	c.JSON(http.StatusOK, gin.H{"orders": orders, "paging_id": page.NextCursor})
}

func (h *DeveloperDashboardHandler) query(c *gin.Context) (usecase.DeveloperQuery, bool) {
	user, authenticated := middleware.AuthenticatedUser(c.Request.Context())
	if !authenticated || user.User.ID == "" {
		writeAuthError(c, http.StatusUnauthorized)
		return usecase.DeveloperQuery{}, false
	}
	query := usecase.DeveloperQuery{
		OwnerUserID:   user.User.ID,
		ClientID:      c.Query("client_id"),
		Currency:      c.Query("currency"),
		Direction:     c.Query("direction"),
		Status:        c.Query("status"),
		PaymentMethod: c.Query("payment_method"),
		Cursor:        c.Query("paging_id"),
		Bucket:        c.Query("bucket"),
	}
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid developer query"})
			return usecase.DeveloperQuery{}, false
		}
		query.Limit = limit
	}
	for name, raw := range map[string]string{"from": c.Query("from"), "to": c.Query("to")} {
		if raw == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid developer query"})
			return usecase.DeveloperQuery{}, false
		}
		if name == "from" {
			query.From = parsed
		} else {
			query.To = parsed
		}
	}
	return query, true
}

func (h *DeveloperDashboardHandler) writeError(c *gin.Context, operation string, err error) {
	if errors.Is(err, usecase.ErrInvalidDeveloperQuery) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid developer query"})
		return
	}
	h.logger.ErrorContext(c.Request.Context(), operation+" failed", slog.Any("error", err))
	writeRequestError(c, http.StatusInternalServerError)
}

type developerOverviewResponse struct {
	Environment                string                   `json:"environment"`
	Network                    string                   `json:"network"`
	From                       time.Time                `json:"from"`
	To                         time.Time                `json:"to"`
	TotalOrders                int64                    `json:"total_orders"`
	ActiveOrders               int64                    `json:"active_orders"`
	CompletedOrders            int64                    `json:"completed_orders"`
	FailedOrders               int64                    `json:"failed_orders"`
	BuyOrders                  int64                    `json:"buy_orders"`
	SellOrders                 int64                    `json:"sell_orders"`
	GrossIDRMinor              string                   `json:"gross_idr_minor"`
	AssetVolume                string                   `json:"asset_volume"`
	FeeIDRMinor                string                   `json:"fee_idr_minor"`
	NetIDRMinor                string                   `json:"net_idr_minor"`
	SuccessRate                float64                  `json:"success_rate"`
	AverageCompletionSeconds   float64                  `json:"average_completion_seconds"`
	PendingWebhookDeliveries   int64                    `json:"pending_webhook_deliveries"`
	ExhaustedWebhookDeliveries int64                    `json:"exhausted_webhook_deliveries"`
	RecentOrders               []developerOrderResponse `json:"recent_orders"`
}

func developerOverviewResponseFrom(value usecase.DeveloperOverview) developerOverviewResponse {
	orders := make([]developerOrderResponse, 0, len(value.RecentOrders))
	for _, order := range value.RecentOrders {
		orders = append(orders, developerOrderResponseFrom(order))
	}
	return developerOverviewResponse{
		Environment:                value.Environment,
		Network:                    value.Network,
		From:                       value.From,
		To:                         value.To,
		TotalOrders:                value.TotalOrders,
		ActiveOrders:               value.ActiveOrders,
		CompletedOrders:            value.CompletedOrders,
		FailedOrders:               value.FailedOrders,
		BuyOrders:                  value.BuyOrders,
		SellOrders:                 value.SellOrders,
		GrossIDRMinor:              strconv.FormatInt(value.GrossIDRMinor, 10),
		AssetVolume:                entity.Stroops(value.AssetVolumeStroops).String(),
		FeeIDRMinor:                strconv.FormatInt(value.FeeIDRMinor, 10),
		NetIDRMinor:                strconv.FormatInt(value.NetIDRMinor, 10),
		SuccessRate:                value.SuccessRate,
		AverageCompletionSeconds:   value.AverageCompletionSeconds,
		PendingWebhookDeliveries:   value.PendingWebhookDeliveries,
		ExhaustedWebhookDeliveries: value.ExhaustedWebhookDeliveries,
		RecentOrders:               orders,
	}
}

type developerAnalyticsResponse struct {
	Environment string                             `json:"environment"`
	Network     string                             `json:"network"`
	From        time.Time                          `json:"from"`
	To          time.Time                          `json:"to"`
	Bucket      string                             `json:"bucket"`
	Buckets     []developerAnalyticsBucketResponse `json:"buckets"`
}

type developerAnalyticsBucketResponse struct {
	Start                    time.Time        `json:"start"`
	End                      time.Time        `json:"end"`
	Orders                   int64            `json:"orders"`
	BuyOrders                int64            `json:"buy_orders"`
	SellOrders               int64            `json:"sell_orders"`
	GrossIDRMinor            string           `json:"gross_idr_minor"`
	AssetVolume              string           `json:"asset_volume"`
	FeeIDRMinor              string           `json:"fee_idr_minor"`
	NetIDRMinor              string           `json:"net_idr_minor"`
	CompletedOrders          int64            `json:"completed_orders"`
	FailedOrders             int64            `json:"failed_orders"`
	AverageCompletionSeconds float64          `json:"average_completion_seconds"`
	PaymentMethods           map[string]int64 `json:"payment_methods"`
	WebhookDelivered         int64            `json:"webhook_delivered"`
	WebhookRetried           int64            `json:"webhook_retried"`
	WebhookExhausted         int64            `json:"webhook_exhausted"`
}

func developerAnalyticsResponseFrom(value usecase.DeveloperAnalytics) developerAnalyticsResponse {
	buckets := make([]developerAnalyticsBucketResponse, 0, len(value.Buckets))
	for _, bucket := range value.Buckets {
		paymentMethods := bucket.PaymentMethods
		if paymentMethods == nil {
			paymentMethods = map[string]int64{}
		}
		buckets = append(buckets, developerAnalyticsBucketResponse{
			Start:                    bucket.Start,
			End:                      bucket.End,
			Orders:                   bucket.Orders,
			BuyOrders:                bucket.BuyOrders,
			SellOrders:               bucket.SellOrders,
			GrossIDRMinor:            strconv.FormatInt(bucket.GrossIDRMinor, 10),
			AssetVolume:              entity.Stroops(bucket.AssetVolumeStroops).String(),
			FeeIDRMinor:              strconv.FormatInt(bucket.FeeIDRMinor, 10),
			NetIDRMinor:              strconv.FormatInt(bucket.NetIDRMinor, 10),
			CompletedOrders:          bucket.CompletedOrders,
			FailedOrders:             bucket.FailedOrders,
			AverageCompletionSeconds: bucket.AverageCompletionSeconds,
			PaymentMethods:           paymentMethods,
			WebhookDelivered:         bucket.WebhookDelivered,
			WebhookRetried:           bucket.WebhookRetried,
			WebhookExhausted:         bucket.WebhookExhausted,
		})
	}
	return developerAnalyticsResponse{Environment: value.Environment, Network: value.Network, From: value.From, To: value.To, Bucket: value.Bucket, Buckets: buckets}
}

type developerRevenueSummaryResponse struct {
	Environment           string    `json:"environment"`
	Network               string    `json:"network"`
	From                  time.Time `json:"from"`
	To                    time.Time `json:"to"`
	GrossIDRMinor         string    `json:"gross_idr_minor"`
	FeeIDRMinor           string    `json:"fee_idr_minor"`
	PlatformRevenueMinor  string    `json:"platform_revenue_minor"`
	DeveloperRevenueMinor string    `json:"developer_revenue_minor"`
	NetIDRMinor           string    `json:"net_idr_minor"`
	EntryCount            int64     `json:"entry_count"`
	Simulated             bool      `json:"simulated"`
	Disclosure            string    `json:"disclosure"`
}

func developerRevenueSummaryResponseFrom(value usecase.DeveloperRevenueSummary) developerRevenueSummaryResponse {
	return developerRevenueSummaryResponse{
		Environment:           value.Environment,
		Network:               value.Network,
		From:                  value.From,
		To:                    value.To,
		GrossIDRMinor:         strconv.FormatInt(value.GrossIDRMinor, 10),
		FeeIDRMinor:           strconv.FormatInt(value.FeeIDRMinor, 10),
		PlatformRevenueMinor:  strconv.FormatInt(value.PlatformRevenueMinor, 10),
		DeveloperRevenueMinor: strconv.FormatInt(value.DeveloperRevenueMinor, 10),
		NetIDRMinor:           strconv.FormatInt(value.NetIDRMinor, 10),
		EntryCount:            value.EntryCount,
		Simulated:             value.Simulated,
		Disclosure:            value.Disclosure,
	}
}

type developerRevenueEntryResponse struct {
	OrderID               string    `json:"order_id"`
	ClientID              string    `json:"client_id"`
	Direction             string    `json:"direction"`
	CreatedAt             time.Time `json:"created_at"`
	GrossIDRMinor         string    `json:"gross_idr_minor"`
	FeeIDRMinor           string    `json:"fee_idr_minor"`
	PlatformRevenueMinor  string    `json:"platform_revenue_minor"`
	DeveloperRevenueMinor string    `json:"developer_revenue_minor"`
	NetIDRMinor           string    `json:"net_idr_minor"`
	AssetAmount           string    `json:"asset_amount"`
	AssetAmountStroops    string    `json:"asset_amount_stroops"`
	FeeCurrency           string    `json:"fee_currency"`
	FeePolicyVersion      string    `json:"fee_policy_version"`
	Source                string    `json:"source"`
	Simulated             bool      `json:"simulated"`
}

func developerRevenueEntryResponseFrom(value usecase.DeveloperRevenueEntry) developerRevenueEntryResponse {
	return developerRevenueEntryResponse{
		OrderID:               value.OrderID,
		ClientID:              value.ClientID,
		Direction:             value.Direction,
		CreatedAt:             value.CreatedAt,
		GrossIDRMinor:         strconv.FormatInt(value.GrossIDRMinor, 10),
		FeeIDRMinor:           strconv.FormatInt(value.FeeIDRMinor, 10),
		PlatformRevenueMinor:  strconv.FormatInt(value.PlatformRevenueMinor, 10),
		DeveloperRevenueMinor: strconv.FormatInt(value.DeveloperRevenueMinor, 10),
		NetIDRMinor:           strconv.FormatInt(value.NetIDRMinor, 10),
		AssetAmount:           value.AssetAmount,
		AssetAmountStroops:    strconv.FormatInt(value.AssetAmountStroops, 10),
		FeeCurrency:           value.FeeCurrency,
		FeePolicyVersion:      value.FeePolicyVersion,
		Source:                value.Source,
		Simulated:             value.Simulated,
	}
}

type developerOrderResponse struct {
	ID                     string                         `json:"id"`
	ClientID               string                         `json:"client_id"`
	ClientName             string                         `json:"client_name"`
	Environment            string                         `json:"environment"`
	Direction              string                         `json:"direction"`
	Status                 string                         `json:"status"`
	Currency               string                         `json:"currency"`
	FiatAmountMinor        string                         `json:"fiat_amount_minor"`
	AssetAmount            string                         `json:"asset_amount"`
	AssetAmountStroops     string                         `json:"asset_amount_stroops"`
	PaymentMethod          string                         `json:"payment_method"`
	GatewayProvider        string                         `json:"gateway_provider"`
	GatewayReference       string                         `json:"gateway_reference"`
	StellarIntentID        string                         `json:"stellar_intent_id"`
	StellarTransactionHash string                         `json:"stellar_transaction_hash"`
	SEP24TransactionID     string                         `json:"sep24_transaction_id"`
	QuoteID                string                         `json:"quote_id"`
	WalletAccount          string                         `json:"wallet_account"`
	PayoutReference        string                         `json:"payout_reference"`
	PayoutSimulated        bool                           `json:"payout_simulated"`
	PayoutDisclosure       string                         `json:"payout_disclosure,omitempty"`
	LatestEventType        string                         `json:"latest_event_type"`
	LatestTransitionAt     time.Time                      `json:"latest_transition_at,omitempty"`
	CreatedAt              time.Time                      `json:"created_at"`
	UpdatedAt              time.Time                      `json:"updated_at"`
	CompletedAt            *time.Time                     `json:"completed_at,omitempty"`
	FailureCode            string                         `json:"failure_code,omitempty"`
	Financial              *developerRevenueEntryResponse `json:"financial,omitempty"`
}

func developerOrderResponseFrom(value usecase.DeveloperOrder) developerOrderResponse {
	var financial *developerRevenueEntryResponse
	if value.Financial != nil {
		converted := developerRevenueEntryResponseFrom(*value.Financial)
		financial = &converted
	}
	return developerOrderResponse{
		ID:                     value.ID,
		ClientID:               value.ClientID,
		ClientName:             value.ClientName,
		Environment:            value.Environment,
		Direction:              value.Direction,
		Status:                 value.Status,
		Currency:               value.Currency,
		FiatAmountMinor:        strconv.FormatInt(value.FiatAmountMinor, 10),
		AssetAmount:            value.AssetAmount,
		AssetAmountStroops:     strconv.FormatInt(value.AssetAmountStroops, 10),
		PaymentMethod:          value.PaymentMethod,
		GatewayProvider:        value.GatewayProvider,
		GatewayReference:       value.GatewayReference,
		StellarIntentID:        value.StellarIntentID,
		StellarTransactionHash: value.StellarTransactionHash,
		SEP24TransactionID:     value.SEP24TransactionID,
		QuoteID:                value.QuoteID,
		WalletAccount:          value.WalletAccount,
		PayoutReference:        value.PayoutReference,
		PayoutSimulated:        value.PayoutSimulated,
		PayoutDisclosure:       value.PayoutDisclosure,
		LatestEventType:        value.LatestEventType,
		LatestTransitionAt:     value.LatestTransitionAt,
		CreatedAt:              value.CreatedAt,
		UpdatedAt:              value.UpdatedAt,
		CompletedAt:            value.CompletedAt,
		FailureCode:            value.FailureCode,
		Financial:              financial,
	}
}
