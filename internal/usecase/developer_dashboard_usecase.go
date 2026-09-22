package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var ErrInvalidDeveloperQuery = errors.New("invalid developer query")

const (
	defaultDeveloperWindow = 30 * 24 * time.Hour
	maxDeveloperWindow     = 366 * 24 * time.Hour
	maxDeveloperPageSize   = 100
)

type DeveloperQuery struct {
	OwnerUserID   string
	ClientID      string
	From          time.Time
	To            time.Time
	Currency      string
	Direction     string
	Status        string
	PaymentMethod string
	Limit         int
	Cursor        string
	Bucket        string
}

type DeveloperDashboardReader interface {
	GetOverview(ctx context.Context, query DeveloperQuery) (DeveloperOverview, error)
	GetAnalytics(ctx context.Context, query DeveloperQuery) (DeveloperAnalytics, error)
	GetRevenueSummary(ctx context.Context, query DeveloperQuery) (DeveloperRevenueSummary, error)
	ListRevenueEntries(ctx context.Context, query DeveloperQuery) (DeveloperPage[DeveloperRevenueEntry], error)
	ListOrders(ctx context.Context, query DeveloperQuery) (DeveloperPage[DeveloperOrder], error)
}

type DeveloperOverview struct {
	Environment                string
	Network                    string
	From                       time.Time
	To                         time.Time
	TotalOrders                int64
	ActiveOrders               int64
	CompletedOrders            int64
	FailedOrders               int64
	BuyOrders                  int64
	SellOrders                 int64
	GrossIDRMinor              int64
	AssetVolumeStroops         int64
	FeeIDRMinor                int64
	NetIDRMinor                int64
	SuccessRate                float64
	AverageCompletionSeconds   float64
	PendingWebhookDeliveries   int64
	ExhaustedWebhookDeliveries int64
	RecentOrders               []DeveloperOrder
}

type DeveloperAnalytics struct {
	Environment string
	Network     string
	From        time.Time
	To          time.Time
	Bucket      string
	Buckets     []DeveloperAnalyticsBucket
}

type DeveloperAnalyticsBucket struct {
	Start                    time.Time
	End                      time.Time
	Orders                   int64
	BuyOrders                int64
	SellOrders               int64
	GrossIDRMinor            int64
	AssetVolumeStroops       int64
	FeeIDRMinor              int64
	NetIDRMinor              int64
	CompletedOrders          int64
	FailedOrders             int64
	AverageCompletionSeconds float64
	PaymentMethods           map[string]int64
	WebhookDelivered         int64
	WebhookRetried           int64
	WebhookExhausted         int64
}

type DeveloperRevenueSummary struct {
	Environment           string
	Network               string
	From                  time.Time
	To                    time.Time
	GrossIDRMinor         int64
	FeeIDRMinor           int64
	PlatformRevenueMinor  int64
	DeveloperRevenueMinor int64
	NetIDRMinor           int64
	EntryCount            int64
	Simulated             bool
	Disclosure            string
}

type DeveloperRevenueEntry struct {
	OrderID               string
	ClientID              string
	Direction             string
	CreatedAt             time.Time
	GrossIDRMinor         int64
	FeeIDRMinor           int64
	PlatformRevenueMinor  int64
	DeveloperRevenueMinor int64
	NetIDRMinor           int64
	AssetAmount           string
	AssetAmountStroops    int64
	FeeCurrency           string
	FeePolicyVersion      string
	Source                string
	Simulated             bool
}

type DeveloperOrder struct {
	ID                     string
	ClientID               string
	ClientName             string
	Environment            string
	Direction              string
	Status                 string
	Currency               string
	FiatAmountMinor        int64
	AssetAmount            string
	AssetAmountStroops     int64
	PaymentMethod          string
	GatewayProvider        string
	GatewayReference       string
	StellarIntentID        string
	StellarTransactionHash string
	SEP24TransactionID     string
	QuoteID                string
	WalletAccount          string
	PayoutReference        string
	PayoutSimulated        bool
	PayoutDisclosure       string
	LatestEventType        string
	LatestTransitionAt     time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	CompletedAt            *time.Time
	FailureCode            string
	Financial              *DeveloperRevenueEntry
}

type DeveloperPage[T any] struct {
	Items      []T
	NextCursor string
}

type DeveloperDashboardUsecase struct {
	reader DeveloperDashboardReader
	now    func() time.Time
}

func NewDeveloperDashboardUsecase(reader DeveloperDashboardReader, now func() time.Time) (*DeveloperDashboardUsecase, error) {
	if reader == nil || now == nil {
		return nil, errors.New("developer dashboard dependencies are required")
	}
	return &DeveloperDashboardUsecase{reader: reader, now: now}, nil
}

func (s *DeveloperDashboardUsecase) Overview(ctx context.Context, query DeveloperQuery) (DeveloperOverview, error) {
	query, err := s.normalize(query, false)
	if err != nil {
		return DeveloperOverview{}, err
	}
	if query.Limit == 0 {
		query.Limit = 10
	}
	result, err := s.reader.GetOverview(ctx, query)
	if err != nil {
		return DeveloperOverview{}, fmt.Errorf("reading developer overview: %w", err)
	}
	if result.RecentOrders == nil {
		result.RecentOrders = []DeveloperOrder{}
	}
	return result, nil
}

func (s *DeveloperDashboardUsecase) Analytics(ctx context.Context, query DeveloperQuery) (DeveloperAnalytics, error) {
	query, err := s.normalize(query, false)
	if err != nil {
		return DeveloperAnalytics{}, err
	}
	if query.Bucket == "" {
		query.Bucket = "day"
	}
	if query.Bucket != "hour" && query.Bucket != "day" && query.Bucket != "week" {
		return DeveloperAnalytics{}, ErrInvalidDeveloperQuery
	}
	result, err := s.reader.GetAnalytics(ctx, query)
	if err != nil {
		return DeveloperAnalytics{}, fmt.Errorf("reading developer analytics: %w", err)
	}
	if result.Buckets == nil {
		result.Buckets = []DeveloperAnalyticsBucket{}
	}
	return result, nil
}

func (s *DeveloperDashboardUsecase) RevenueSummary(ctx context.Context, query DeveloperQuery) (DeveloperRevenueSummary, error) {
	query, err := s.normalize(query, false)
	if err != nil {
		return DeveloperRevenueSummary{}, err
	}
	result, err := s.reader.GetRevenueSummary(ctx, query)
	if err != nil {
		return DeveloperRevenueSummary{}, fmt.Errorf("reading developer revenue summary: %w", err)
	}
	return result, nil
}

func (s *DeveloperDashboardUsecase) RevenueEntries(ctx context.Context, query DeveloperQuery) (DeveloperPage[DeveloperRevenueEntry], error) {
	query, err := s.normalize(query, true)
	if err != nil {
		return DeveloperPage[DeveloperRevenueEntry]{}, err
	}
	result, err := s.reader.ListRevenueEntries(ctx, query)
	if err != nil {
		return DeveloperPage[DeveloperRevenueEntry]{}, fmt.Errorf("listing developer revenue entries: %w", err)
	}
	if result.Items == nil {
		result.Items = []DeveloperRevenueEntry{}
	}
	return result, nil
}

func (s *DeveloperDashboardUsecase) Orders(ctx context.Context, query DeveloperQuery) (DeveloperPage[DeveloperOrder], error) {
	query, err := s.normalize(query, true)
	if err != nil {
		return DeveloperPage[DeveloperOrder]{}, err
	}
	result, err := s.reader.ListOrders(ctx, query)
	if err != nil {
		return DeveloperPage[DeveloperOrder]{}, fmt.Errorf("listing developer orders: %w", err)
	}
	if result.Items == nil {
		result.Items = []DeveloperOrder{}
	}
	return result, nil
}

func (s *DeveloperDashboardUsecase) normalize(query DeveloperQuery, paginated bool) (DeveloperQuery, error) {
	query.OwnerUserID = strings.TrimSpace(query.OwnerUserID)
	query.ClientID = strings.TrimSpace(query.ClientID)
	query.Currency = strings.ToUpper(strings.TrimSpace(query.Currency))
	query.Direction = strings.ToLower(strings.TrimSpace(query.Direction))
	query.Status = strings.ToLower(strings.TrimSpace(query.Status))
	query.PaymentMethod = strings.ToLower(strings.TrimSpace(query.PaymentMethod))
	query.Bucket = strings.ToLower(strings.TrimSpace(query.Bucket))
	query.Cursor = strings.TrimSpace(query.Cursor)
	if query.OwnerUserID == "" || query.Cursor != "" && len(query.Cursor) > 512 {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	if query.Currency != "" && query.Currency != IDRCurrency && query.Currency != NativeXLMAssetCode {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	if query.Direction != "" && query.Direction != "onramp" && query.Direction != "offramp" {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	if query.Status != "" && !validDeveloperOrderStatus(query.Status) {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	if query.PaymentMethod != "" && query.PaymentMethod != string(PaymentMethodQRIS) && query.PaymentMethod != string(PaymentMethodBRIVA) && query.PaymentMethod != string(PaymentMethodXendit) {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	now := s.now().UTC()
	if query.To.IsZero() {
		query.To = now
	} else {
		query.To = query.To.UTC()
	}
	if query.From.IsZero() {
		query.From = query.To.Add(-defaultDeveloperWindow)
	} else {
		query.From = query.From.UTC()
	}
	if query.From.After(query.To) || query.To.Sub(query.From) > maxDeveloperWindow {
		return DeveloperQuery{}, ErrInvalidDeveloperQuery
	}
	if paginated {
		if query.Limit <= 0 {
			query.Limit = 20
		}
		if query.Limit > maxDeveloperPageSize {
			query.Limit = maxDeveloperPageSize
		}
	}
	return query, nil
}

func validDeveloperOrderStatus(status string) bool {
	switch status {
	case string(entity.OrderStatusCreated), string(entity.OrderStatusPaymentPending), string(entity.OrderStatusPaymentConfirmed),
		string(entity.OrderStatusAssetReceived), string(entity.OrderStatusStellarProcessing), string(entity.OrderStatusCompleted),
		string(entity.OrderStatusAssetPending), string(entity.OrderStatusExpired), string(entity.OrderStatusPaymentFailed),
		string(entity.OrderStatusStellarFailed), string(entity.OrderStatusCancelled):
		return true
	default:
		return false
	}
}
