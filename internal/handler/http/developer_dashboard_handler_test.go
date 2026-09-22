package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type developerDashboardServiceSpy struct {
	overviewQuery usecase.DeveloperQuery
	overview      usecase.DeveloperOverview
	err           error
}

func (s *developerDashboardServiceSpy) Overview(_ context.Context, query usecase.DeveloperQuery) (usecase.DeveloperOverview, error) {
	s.overviewQuery = query
	if s.err != nil {
		return usecase.DeveloperOverview{}, s.err
	}
	return s.overview, nil
}

func (s *developerDashboardServiceSpy) Analytics(context.Context, usecase.DeveloperQuery) (usecase.DeveloperAnalytics, error) {
	return usecase.DeveloperAnalytics{Buckets: []usecase.DeveloperAnalyticsBucket{}}, nil
}

func (s *developerDashboardServiceSpy) RevenueSummary(context.Context, usecase.DeveloperQuery) (usecase.DeveloperRevenueSummary, error) {
	return usecase.DeveloperRevenueSummary{}, nil
}

func (s *developerDashboardServiceSpy) RevenueEntries(context.Context, usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperRevenueEntry], error) {
	return usecase.DeveloperPage[usecase.DeveloperRevenueEntry]{Items: []usecase.DeveloperRevenueEntry{}}, nil
}

func (s *developerDashboardServiceSpy) Orders(context.Context, usecase.DeveloperQuery) (usecase.DeveloperPage[usecase.DeveloperOrder], error) {
	return usecase.DeveloperPage[usecase.DeveloperOrder]{Items: []usecase.DeveloperOrder{}}, nil
}

func TestDeveloperDashboardHandlerOverviewUsesSessionOwnerAndExactAmountStrings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &developerDashboardServiceSpy{overview: usecase.DeveloperOverview{
		Environment:        "test",
		Network:            usecase.StellarTestnetNetwork,
		From:               time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		To:                 time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
		TotalOrders:        1,
		GrossIDRMinor:      100_000,
		AssetVolumeStroops: 12_345_678,
		RecentOrders:       []usecase.DeveloperOrder{},
	}}
	handler := NewDeveloperDashboardHandler(service, nil)
	router := gin.New()
	router.GET("/v1/developer/overview", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Overview)

	request := httptest.NewRequest(http.MethodGet, "/v1/developer/overview?client_id=client-1&from=2026-09-01T00:00:00Z&to=2026-09-22T00:00:00Z", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.overviewQuery.OwnerUserID != "user-1" || service.overviewQuery.ClientID != "client-1" {
		t.Fatalf("status = %d, query = %#v, body = %q", response.Code, service.overviewQuery, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"gross_idr_minor":"100000"`) || !strings.Contains(body, `"asset_volume":"1.2345678"`) || !strings.Contains(body, `"recent_orders":[]`) {
		t.Fatalf("response does not preserve exact dashboard values: %q", body)
	}
}

func TestDeveloperDashboardHandlerRejectsInvalidDateQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewDeveloperDashboardHandler(&developerDashboardServiceSpy{}, nil)
	router := gin.New()
	router.GET("/v1/developer/overview", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Overview)

	request := httptest.NewRequest(http.MethodGet, "/v1/developer/overview?from=not-a-time", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid developer query") {
		t.Fatalf("status/body = %d/%q", response.Code, response.Body.String())
	}
}
