package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type callbackServiceFake struct {
	token string
	raw   []byte
	err   error
}

func (s *callbackServiceFake) Process(_ context.Context, raw []byte, token string) error {
	s.raw, s.token = raw, token
	return s.err
}

func TestXenditCallbackHandlerPassesExactBodyAndToken(t *testing.T) {
	service := &callbackServiceFake{}
	handler := NewXenditCallbackHandler(service, nil)
	router := http.NewServeMux()
	router.HandleFunc("POST /callbacks/payments/xendit", func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	})
	request := httptest.NewRequest(http.MethodPost, "/callbacks/payments/xendit", strings.NewReader(`{"event":"payment.capture"}`))
	request.Header.Set("x-callback-token", "secret-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.token != "secret-token" || string(service.raw) != `{"event":"payment.capture"}` {
		t.Fatalf("status = %d, token/raw = %q/%q", response.Code, service.token, service.raw)
	}
}
