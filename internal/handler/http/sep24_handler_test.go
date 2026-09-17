package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/handler/middleware"
	"github.com/febry3/kailopay-be/internal/usecase"
	"github.com/gin-gonic/gin"
)

type sep24HandlerServiceFake struct {
	depositCommand       usecase.Sep24DepositCommand
	withdrawCommand      usecase.Sep24WithdrawCommand
	transactionPrincipal usecase.OrderPrincipal
	transactionID        string
	depositView          usecase.Sep24TransactionView
	withdrawView         usecase.Sep24TransactionView
	transactionView      usecase.Sep24TransactionView
	transactionViews     []usecase.Sep24TransactionView
	depositErr           error
	withdrawErr          error
	transactionErr       error
}

func (f *sep24HandlerServiceFake) StartDeposit(_ context.Context, command usecase.Sep24DepositCommand) (usecase.Sep24TransactionView, error) {
	f.depositCommand = command
	return f.depositView, f.depositErr
}

func (f *sep24HandlerServiceFake) StartWithdraw(_ context.Context, command usecase.Sep24WithdrawCommand) (usecase.Sep24TransactionView, error) {
	f.withdrawCommand = command
	return f.withdrawView, f.withdrawErr
}

func (f *sep24HandlerServiceFake) GetTransaction(_ context.Context, principal usecase.OrderPrincipal, transactionID string) (usecase.Sep24TransactionView, error) {
	f.transactionPrincipal = principal
	f.transactionID = transactionID
	return f.transactionView, f.transactionErr
}

func (f *sep24HandlerServiceFake) ListTransactions(_ context.Context, _ usecase.OrderPrincipal, _ int) ([]usecase.Sep24TransactionView, error) {
	return f.transactionViews, nil
}

type sep24HandlerAPIAuthenticatorFake struct {
	principal usecase.Principal
}

func (f sep24HandlerAPIAuthenticatorFake) Authenticate(context.Context, string) (usecase.Principal, error) {
	return f.principal, nil
}

func TestSEP24DepositRequiresAuthenticatedOrderPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &sep24HandlerServiceFake{}
	handler := NewSep24Handler(service, Sep24Config{TransferServerURL: "https://anchor.example/sep24"}, nil)
	router := gin.New()
	router.POST("/transactions/deposit/interactive", handler.Deposit)

	request := httptest.NewRequest(http.MethodPost, "/transactions/deposit/interactive", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestSEP24InfoUsesExplicitAmountUnits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewSep24Handler(nil, Sep24Config{
		DepositMinAmountMinor: 10_000,
		DepositMaxAmountMinor: 10_000_000,
	}, nil)
	router := gin.New()
	router.GET("/sep24/info", handler.Info)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sep24/info", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	var payload struct {
		Deposit  map[string]map[string]any `json:"deposit"`
		Withdraw map[string]map[string]any `json:"withdraw"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if payload.Deposit["XLM"]["amount_unit"] != "idr_minor" || payload.Deposit["XLM"]["fiat_currency"] != "IDR" ||
		payload.Deposit["XLM"]["min_amount"] != float64(10_000) || payload.Deposit["XLM"]["max_amount"] != float64(10_000_000) {
		t.Fatalf("deposit info = %#v", payload.Deposit["XLM"])
	}
	if payload.Withdraw["XLM"]["amount_unit"] != "XLM" || payload.Withdraw["XLM"]["fiat_currency"] != "IDR" {
		t.Fatalf("withdraw info = %#v", payload.Withdraw["XLM"])
	}
}

func TestSEP24DepositCreatesMappedInteractiveResponseFromMultipartForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := usecase.Principal{ClientID: "client-1", OwnerUserID: "user-1"}
	service := &sep24HandlerServiceFake{depositView: usecase.Sep24TransactionView{
		ID: "deposit-order-1", Kind: usecase.Sep24KindDeposit, Status: "pending_user_transfer_start",
		Order: usecase.OrderView{Checkout: &usecase.Checkout{
			PaymentLinkURL: "https://checkout-staging.xendit.co/sessions/ps-1",
		}},
	}}
	handler := NewSep24Handler(service, Sep24Config{TransferServerURL: "https://anchor.example/sep24"}, nil)
	router := gin.New()
	router.Use(middleware.RequireOrderPrincipal(sep24HandlerAPIAuthenticatorFake{principal: principal}, nil, middleware.DefaultSessionCookieName, nil))
	router.POST("/transactions/deposit/interactive", handler.Deposit)

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"asset_code":   "XLM",
		"amount_minor": "100000",
		"account":      "G" + strings.Repeat("A", 55),
		"memo":         "wallet-memo",
	} {
		if err := form.WriteField(key, value); err != nil {
			t.Fatalf("writing form field %q: %v", key, err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatalf("closing form: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/transactions/deposit/interactive", &body)
	request.Header.Set("Authorization", "Bearer pk_test_secret")
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Idempotency-Key", "wallet-deposit-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if service.depositCommand.Principal.OwnerUserID != "user-1" || service.depositCommand.AmountMinor != entity.IDR(100_000) ||
		service.depositCommand.Destination != "G"+strings.Repeat("A", 55) || service.depositCommand.Memo != "wallet-memo" {
		t.Fatalf("deposit command = %+v", service.depositCommand)
	}
	var payload struct {
		Type           string `json:"type"`
		ID             string `json:"id"`
		URL            string `json:"url"`
		PaymentLinkURL string `json:"payment_link_url"`
		KYCRequired    bool   `json:"kyc_required"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if payload.Type != "interactive_customer_info_needed" || payload.ID != "deposit-order-1" ||
		payload.URL != "https://anchor.example/sep24/interactive/deposit-order-1" ||
		payload.PaymentLinkURL != "https://checkout-staging.xendit.co/sessions/ps-1" || payload.KYCRequired {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestSEP24TransactionUsesPersistedStatusInsteadOfClientSuppliedStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := usecase.Principal{ClientID: "client-1", OwnerUserID: "user-1"}
	service := &sep24HandlerServiceFake{transactionView: usecase.Sep24TransactionView{
		ID: "deposit-order-1", Kind: usecase.Sep24KindDeposit, Status: "pending_user_transfer_start",
		Order: usecase.OrderView{Checkout: &usecase.Checkout{
			PaymentLinkURL: "https://checkout-staging.xendit.co/sessions/ps-1",
		}},
	}}
	handler := NewSep24Handler(service, Sep24Config{TransferServerURL: "https://anchor.example/sep24"}, nil)
	router := gin.New()
	router.Use(middleware.RequireOrderPrincipal(sep24HandlerAPIAuthenticatorFake{principal: principal}, nil, middleware.DefaultSessionCookieName, nil))
	router.GET("/transaction", handler.Transaction)

	request := httptest.NewRequest(http.MethodGet, "/transaction?id=deposit-order-1&internal_status=completed", nil)
	request.Header.Set("Authorization", "Bearer pk_test_secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if service.transactionID != "deposit-order-1" || service.transactionPrincipal.OwnerUserID != "user-1" {
		t.Fatalf("lookup = %q/%+v", service.transactionID, service.transactionPrincipal)
	}
	var payload struct {
		Transaction struct {
			Status         string `json:"status"`
			PaymentLinkURL string `json:"payment_link_url"`
		} `json:"transaction"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if payload.Transaction.Status != "pending_user_transfer_start" ||
		payload.Transaction.PaymentLinkURL != "https://checkout-staging.xendit.co/sessions/ps-1" {
		t.Fatalf("transaction = %+v", payload.Transaction)
	}
}

func TestSEP24TransactionsReturnsOwnerHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := usecase.Principal{ClientID: "client-1", OwnerUserID: "user-1"}
	service := &sep24HandlerServiceFake{transactionViews: []usecase.Sep24TransactionView{
		{ID: "deposit-order-1", Kind: usecase.Sep24KindDeposit, Status: "completed"},
	}}
	handler := NewSep24Handler(service, Sep24Config{TransferServerURL: "https://anchor.example/sep24"}, nil)
	router := gin.New()
	router.Use(middleware.RequireOrderPrincipal(sep24HandlerAPIAuthenticatorFake{principal: principal}, nil, middleware.DefaultSessionCookieName, nil))
	router.GET("/transactions", handler.Transactions)

	request := httptest.NewRequest(http.MethodGet, "/transactions?limit=10", nil)
	request.Header.Set("Authorization", "Bearer pk_test_secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	var payload struct {
		Transactions []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(payload.Transactions) != 1 || payload.Transactions[0].ID != "deposit-order-1" || payload.Transactions[0].Status != "completed" {
		t.Fatalf("transactions = %+v", payload.Transactions)
	}
}
