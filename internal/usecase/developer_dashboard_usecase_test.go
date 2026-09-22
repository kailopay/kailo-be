package usecase

import (
	"context"
	"testing"
	"time"
)

type dashboardReaderSpy struct {
	overviewQuery  DeveloperQuery
	analyticsQuery DeveloperQuery
	revenueQuery   DeveloperQuery
	ordersQuery    DeveloperQuery
}

func (s *dashboardReaderSpy) GetOverview(_ context.Context, query DeveloperQuery) (DeveloperOverview, error) {
	s.overviewQuery = query
	return DeveloperOverview{RecentOrders: []DeveloperOrder{}}, nil
}

func (s *dashboardReaderSpy) GetAnalytics(_ context.Context, query DeveloperQuery) (DeveloperAnalytics, error) {
	s.analyticsQuery = query
	return DeveloperAnalytics{Buckets: []DeveloperAnalyticsBucket{}}, nil
}

func (s *dashboardReaderSpy) GetRevenueSummary(_ context.Context, query DeveloperQuery) (DeveloperRevenueSummary, error) {
	s.revenueQuery = query
	return DeveloperRevenueSummary{}, nil
}

func (s *dashboardReaderSpy) ListRevenueEntries(_ context.Context, query DeveloperQuery) (DeveloperPage[DeveloperRevenueEntry], error) {
	s.revenueQuery = query
	return DeveloperPage[DeveloperRevenueEntry]{Items: []DeveloperRevenueEntry{}}, nil
}

func (s *dashboardReaderSpy) ListOrders(_ context.Context, query DeveloperQuery) (DeveloperPage[DeveloperOrder], error) {
	s.ordersQuery = query
	return DeveloperPage[DeveloperOrder]{Items: []DeveloperOrder{}}, nil
}

func TestDeveloperDashboardUsecaseAppliesAccountScopedOverviewDefaults(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	reader := &dashboardReaderSpy{}
	service, err := NewDeveloperDashboardUsecase(reader, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDeveloperDashboardUsecase() error = %v", err)
	}

	result, err := service.Overview(context.Background(), DeveloperQuery{OwnerUserID: "user-1"})
	if err != nil {
		t.Fatalf("Overview() error = %v", err)
	}
	if result.RecentOrders == nil {
		t.Fatal("Overview().RecentOrders is nil, want initialized empty slice")
	}
	if reader.overviewQuery.OwnerUserID != "user-1" || reader.overviewQuery.To != now || !reader.overviewQuery.From.Equal(now.Add(-30*24*time.Hour)) {
		t.Fatalf("overview query = %#v", reader.overviewQuery)
	}
	if reader.overviewQuery.Limit != 10 {
		t.Fatalf("overview limit = %d, want 10", reader.overviewQuery.Limit)
	}
}

func TestDeveloperDashboardUsecaseRejectsInvalidAnalyticsQuery(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	reader := &dashboardReaderSpy{}
	service, err := NewDeveloperDashboardUsecase(reader, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewDeveloperDashboardUsecase() error = %v", err)
	}

	for name, query := range map[string]DeveloperQuery{
		"missing owner":      {Bucket: "day"},
		"unsupported bucket": {OwnerUserID: "user-1", Bucket: "minute"},
		"range too wide":     {OwnerUserID: "user-1", Bucket: "day", From: now.Add(-400 * 24 * time.Hour), To: now},
		"reversed range":     {OwnerUserID: "user-1", Bucket: "day", From: now, To: now.Add(-time.Hour)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Analytics(context.Background(), query); err != ErrInvalidDeveloperQuery {
				t.Fatalf("Analytics() error = %v, want ErrInvalidDeveloperQuery", err)
			}
		})
	}
}

func TestDeveloperDashboardUsecaseClampsListLimitAndInitializesPages(t *testing.T) {
	reader := &dashboardReaderSpy{}
	service, err := NewDeveloperDashboardUsecase(reader, func() time.Time { return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("NewDeveloperDashboardUsecase() error = %v", err)
	}

	page, err := service.Orders(context.Background(), DeveloperQuery{OwnerUserID: "user-1", Limit: 1000})
	if err != nil {
		t.Fatalf("Orders() error = %v", err)
	}
	if page.Items == nil {
		t.Fatal("Orders().Items is nil, want initialized empty slice")
	}
	if reader.ordersQuery.Limit != 100 {
		t.Fatalf("orders limit = %d, want 100", reader.ordersQuery.Limit)
	}
}
