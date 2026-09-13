package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type fakeKYCService struct {
	status        usecase.KYCStatusView
	statusErr     error
	inquiry       usecase.KYCInquiryView
	inquiryErr    error
	webhookErr    error
	statusUserID  string
	inquiryUserID string
	webhookBody   []byte
	webhookHeader string
}

func (s *fakeKYCService) Status(_ context.Context, userID string) (usecase.KYCStatusView, error) {
	s.statusUserID = userID
	return s.status, s.statusErr
}

func (s *fakeKYCService) StartInquiry(_ context.Context, userID string) (usecase.KYCInquiryView, error) {
	s.inquiryUserID = userID
	return s.inquiry, s.inquiryErr
}

func (s *fakeKYCService) ProcessPersonaWebhook(_ context.Context, body []byte, signature string, _ time.Time) error {
	s.webhookBody = append([]byte(nil), body...)
	s.webhookHeader = signature
	return s.webhookErr
}

func TestKYCHandlerRequiresSessionForStatusAndInquiry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewKYCHandler(&fakeKYCService{}, nil)
	router := gin.New()
	session := middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}})
	router.GET("/v1/kyc", session, handler.Status)
	router.POST("/v1/kyc/inquiry", session, handler.Inquiry)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/v1/kyc"
		if method == http.MethodPost {
			path += "/inquiry"
		}
		request := httptest.NewRequest(method, path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", method, path, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestKYCHandlerReturnsStatusForAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeKYCService{status: usecase.KYCStatusView{
		Provider: "persona", Status: usecase.KYCStatusApproved, ProviderStatus: "approved", InquiryID: "inq_123",
	}}
	handler := NewKYCHandler(service, nil)
	router := gin.New()
	router.GET("/v1/kyc", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Status)

	request := httptest.NewRequest(http.MethodGet, "/v1/kyc", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || service.statusUserID != "user-1" {
		t.Fatalf("status = %d, user = %q, body = %q", response.Code, service.statusUserID, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, `"provider":"persona"`) ||
		!strings.Contains(body, `"status":"approved"`) || strings.Contains(body, "session_token") {
		t.Fatalf("unexpected KYC status response: %q", body)
	}
}

func TestKYCHandlerReturnsInquiryWithOptionalSessionToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name      string
		token     string
		wantToken bool
	}{
		{name: "new inquiry", token: "", wantToken: false},
		{name: "resumed inquiry", token: "persona-session-token", wantToken: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeKYCService{inquiry: usecase.KYCInquiryView{
				Status: usecase.KYCStatusPending, ProviderStatus: "pending", InquiryID: "inq_123",
				EnvironmentID: "env_123", SessionToken: test.token,
			}}
			handler := NewKYCHandler(service, nil)
			router := gin.New()
			router.POST("/v1/kyc/inquiry", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
				User: usecase.UserProfile{ID: "user-1"},
			}}), handler.Inquiry)

			request := httptest.NewRequest(http.MethodPost, "/v1/kyc/inquiry", nil)
			request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusCreated || service.inquiryUserID != "user-1" {
				t.Fatalf("status = %d, user = %q, body = %q", response.Code, service.inquiryUserID, response.Body.String())
			}
			body := response.Body.String()
			if !strings.Contains(body, `"environment_id":"env_123"`) || !strings.Contains(body, `"inquiry_id":"inq_123"`) {
				t.Fatalf("missing inquiry fields: %q", body)
			}
			if strings.Contains(body, "persona-api-key") || strings.Contains(body, "webhook-secret") {
				t.Fatalf("response leaked provider credentials: %q", body)
			}
			if strings.Contains(body, "session_token") != test.wantToken {
				t.Fatalf("session token presence = %t, want %t: %q", strings.Contains(body, "session_token"), test.wantToken, body)
			}
		})
	}
}

func TestKYCWebhookHandlerUsesExactBodyAndDoesNotEchoIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeKYCService{}
	handler := NewKYCHandler(service, nil)
	router := gin.New()
	router.POST("/callbacks/kyc/persona", gin.WrapH(handler))
	rawBody := `{"data":{"id":"evt_1","private":"do-not-echo"}}`
	request := httptest.NewRequest(http.MethodPost, "/callbacks/kyc/persona", strings.NewReader(rawBody))
	request.Header.Set("Persona-Signature", "t=123,v1=signature")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK || string(service.webhookBody) != rawBody || service.webhookHeader != "t=123,v1=signature" {
		t.Fatalf("status = %d, body = %q, signature = %q, response = %q", response.Code, service.webhookBody, service.webhookHeader, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "do-not-echo") {
		t.Fatalf("callback response echoed raw body: %q", response.Body.String())
	}
}

func TestKYCWebhookHandlerRejectsInvalidSignatureAndMalformedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "invalid signature", err: usecase.ErrKYCInvalidWebhook},
		{name: "malformed payload", err: usecase.ErrKYCInvalidWebhook},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeKYCService{webhookErr: test.err}
			handler := NewKYCHandler(service, nil)
			request := httptest.NewRequest(http.MethodPost, "/callbacks/kyc/persona", strings.NewReader(`{"not":"persona"}`))
			request.Header.Set("Persona-Signature", "invalid")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d, body = %q", response.Code, http.StatusUnauthorized, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "persona") {
				t.Fatalf("response leaked provider details: %q", response.Body.String())
			}
		})
	}
}

func TestKYCWebhookHandlerRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewKYCHandler(&fakeKYCService{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/callbacks/kyc/persona", strings.NewReader(strings.Repeat("x", int(maxKYCWebhookBytes)+1)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestKYCHandlerMapsProviderUnavailableWithoutDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeKYCService{inquiryErr: &usecase.KYCProviderUnavailableError{Err: errors.New("persona bearer secret")}}
	handler := NewKYCHandler(service, nil)
	router := gin.New()
	router.POST("/v1/kyc/inquiry", middleware.RequireSession(fakeSessionAuthenticator{user: usecase.AuthenticatedUser{
		User: usecase.UserProfile{ID: "user-1"},
	}}), handler.Inquiry)
	request := httptest.NewRequest(http.MethodPost, "/v1/kyc/inquiry", nil)
	request.AddCookie(&http.Cookie{Name: middleware.DefaultSessionCookieName, Value: "session"})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "bearer") {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

var _ usecase.KYCService = (*fakeKYCService)(nil)
