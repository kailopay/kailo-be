package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type sep10HandlerServiceFake struct {
	challenge usecase.SEP10ChallengeView
	token     usecase.SEP10TokenView
	err       error
}

func (f sep10HandlerServiceFake) Challenge(context.Context, string) (usecase.SEP10ChallengeView, error) {
	return f.challenge, f.err
}

func (f sep10HandlerServiceFake) Exchange(context.Context, string) (usecase.SEP10TokenView, error) {
	return f.token, f.err
}

func (f sep10HandlerServiceFake) Authenticate(context.Context, string) (usecase.SEP10Principal, error) {
	return usecase.SEP10Principal{}, f.err
}

func TestSEP10HandlerReturnsProtocolChallengeAndTokenShapes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSEP10Handler(sep10HandlerServiceFake{
		challenge: usecase.SEP10ChallengeView{Transaction: "challenge-x"},
		token:     usecase.SEP10TokenView{Token: "token-x"},
	}, nil)
	router := gin.New()
	router.GET("/auth", handler.Challenge)
	router.POST("/auth", handler.Exchange)

	challengeResponse := httptest.NewRecorder()
	challengeRequest := httptest.NewRequest(http.MethodGet, "/auth?account=GACCOUNT", nil)
	router.ServeHTTP(challengeResponse, challengeRequest)
	if challengeResponse.Code != http.StatusOK || !strings.Contains(challengeResponse.Body.String(), `"transaction":"challenge-x"`) {
		t.Fatalf("challenge response = %d %q", challengeResponse.Code, challengeResponse.Body.String())
	}

	tokenResponse := httptest.NewRecorder()
	tokenRequest := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"transaction":"signed-x"}`))
	tokenRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(tokenResponse, tokenRequest)
	if tokenResponse.Code != http.StatusOK || !strings.Contains(tokenResponse.Body.String(), `"token":"token-x"`) {
		t.Fatalf("token response = %d %q", tokenResponse.Code, tokenResponse.Body.String())
	}
}

func TestSEP10HandlerRejectsInvalidInputWithoutProviderDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSEP10Handler(sep10HandlerServiceFake{err: usecase.ErrSEP10InvalidChallenge}, nil)
	router := gin.New()
	router.GET("/auth", handler.Challenge)
	router.POST("/auth", handler.Exchange)

	challengeResponse := httptest.NewRecorder()
	router.ServeHTTP(challengeResponse, httptest.NewRequest(http.MethodGet, "/auth", nil))
	if challengeResponse.Code != http.StatusBadRequest {
		t.Fatalf("missing account status = %d", challengeResponse.Code)
	}

	tokenResponse := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"transaction":"signed-x"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(tokenResponse, request)
	if tokenResponse.Code != http.StatusBadRequest || strings.Contains(tokenResponse.Body.String(), "provider") {
		t.Fatalf("invalid token response = %d %q", tokenResponse.Code, tokenResponse.Body.String())
	}
}
