package xendit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

const maxResponseBytes int64 = 1 << 20

type Config struct {
	BaseURL       string
	SecretKey     string
	CallbackToken string
	APIVersion    string
	QRISChannel   string
	VAChannel     string
	HTTPClient    *http.Client
}

type Client struct {
	baseURL       *url.URL
	secretKey     string
	callbackToken string
	apiVersion    string
	qrisChannel   string
	vaChannel     string
	httpClient    *http.Client
}

type paymentRequest struct {
	PaymentRequestID  string      `json:"payment_request_id"`
	ReferenceID       string      `json:"reference_id"`
	Currency          string      `json:"currency"`
	RequestAmount     json.Number `json:"request_amount"`
	ChannelCode       string      `json:"channel_code"`
	Status            string      `json:"status"`
	ChannelProperties struct {
		ExpiresAt string `json:"expires_at"`
	} `json:"channel_properties"`
	Actions []struct {
		Type       string `json:"type"`
		Descriptor string `json:"descriptor"`
		Value      string `json:"value"`
	} `json:"actions"`
}

type paymentSession struct {
	PaymentSessionID string      `json:"payment_session_id"`
	ReferenceID      string      `json:"reference_id"`
	Currency         string      `json:"currency"`
	Amount           json.Number `json:"amount"`
	Status           string      `json:"status"`
	ExpiresAt        string      `json:"expires_at"`
	PaymentLinkURL   string      `json:"payment_link_url"`
	PaymentRequestID string      `json:"payment_request_id"`
}

func New(config Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || !baseURL.IsAbs() || strings.TrimSpace(config.SecretKey) == "" || len(config.CallbackToken) < 32 || config.APIVersion == "" ||
		config.QRISChannel == "" || config.VAChannel == "" || config.HTTPClient == nil {
		return nil, errors.New("valid Xendit configuration is required")
	}
	return &Client{baseURL: baseURL, secretKey: config.SecretKey, callbackToken: config.CallbackToken, apiVersion: config.APIVersion,
		qrisChannel: config.QRISChannel, vaChannel: config.VAChannel, httpClient: config.HTTPClient}, nil
}

func (c *Client) VerifyCallback(raw []byte, token string) (usecase.Callback, error) {
	providedTokenHash := sha256.Sum256([]byte(token))
	expectedTokenHash := sha256.Sum256([]byte(c.callbackToken))
	if subtle.ConstantTimeCompare(providedTokenHash[:], expectedTokenHash[:]) != 1 {
		return usecase.Callback{}, usecase.ErrInvalidCallback
	}
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			PaymentID        string `json:"payment_id"`
			ID               string `json:"id"`
			PaymentSessionID string `json:"payment_session_id"`
			PaymentRequestID string `json:"payment_request_id"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil || payload.Event == "" {
		return usecase.Callback{}, usecase.ErrInvalidCallback
	}
	switch payload.Event {
	case "payment_session.completed", "payment_session.expired":
		sessionID := payload.Data.PaymentSessionID
		if sessionID == "" {
			sessionID = payload.Data.ID
		}
		if sessionID == "" {
			return usecase.Callback{}, usecase.ErrInvalidCallback
		}
		return usecase.Callback{EventID: payload.Event + ":" + sessionID, EventType: payload.Event,
			CheckoutID: sessionID}, nil
	case "payment.capture":
		if payload.Data.PaymentID == "" || (payload.Data.PaymentSessionID == "" && payload.Data.PaymentRequestID == "") {
			return usecase.Callback{}, usecase.ErrInvalidCallback
		}
		checkoutID := payload.Data.PaymentRequestID
		if payload.Data.PaymentSessionID != "" {
			checkoutID = payload.Data.PaymentSessionID
		}
		return usecase.Callback{EventID: payload.Data.PaymentID, EventType: payload.Event, CheckoutID: checkoutID}, nil
	default:
		return usecase.Callback{}, usecase.ErrInvalidCallback
	}
}

func (c *Client) GetPaymentState(ctx context.Context, checkoutID string) (usecase.PaymentState, error) {
	if strings.HasPrefix(checkoutID, "ps-") {
		return c.getPaymentSession(ctx, checkoutID)
	}
	return c.getPaymentRequest(ctx, checkoutID)
}

func (c *Client) getPaymentRequest(ctx context.Context, providerID string) (usecase.PaymentState, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v3/payment_requests/" + url.PathEscape(providerID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return usecase.PaymentState{}, fmt.Errorf("creating Xendit reconciliation request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("api-version", c.apiVersion)
	request.SetBasicAuth(c.secretKey, "")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return usecase.PaymentState{}, fmt.Errorf("getting Xendit payment request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return usecase.PaymentState{}, fmt.Errorf("Xendit reconciliation status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil || int64(len(body)) > maxResponseBytes {
		return usecase.PaymentState{}, errors.New("reading Xendit reconciliation response")
	}
	var provider paymentRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&provider); err != nil {
		return usecase.PaymentState{}, errors.New("decoding Xendit reconciliation response")
	}
	amount, err := providerAmount(provider.RequestAmount)
	if err != nil {
		return usecase.PaymentState{}, err
	}
	return usecase.PaymentState{ProviderID: provider.PaymentRequestID, ReferenceID: provider.ReferenceID, Status: provider.Status,
		Currency: provider.Currency, Amount: entity.IDR(amount), Channel: provider.ChannelCode}, nil
}

func (c *Client) getPaymentSession(ctx context.Context, providerID string) (usecase.PaymentState, error) {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/sessions/" + url.PathEscape(providerID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return usecase.PaymentState{}, fmt.Errorf("creating Xendit session reconciliation request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("api-version", c.apiVersion)
	request.SetBasicAuth(c.secretKey, "")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return usecase.PaymentState{}, fmt.Errorf("getting Xendit payment session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return usecase.PaymentState{}, fmt.Errorf("Xendit session reconciliation status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(body)) > maxResponseBytes {
		return usecase.PaymentState{}, errors.New("reading Xendit session reconciliation response")
	}
	var provider paymentSession
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&provider); err != nil {
		return usecase.PaymentState{}, errors.New("decoding Xendit session reconciliation response")
	}
	amount, err := providerAmount(provider.Amount)
	if err != nil {
		return usecase.PaymentState{}, err
	}
	if provider.PaymentSessionID != providerID || provider.ReferenceID == "" || provider.Currency != "IDR" {
		return usecase.PaymentState{}, errors.New("Xendit payment session response mismatch")
	}
	channel := ""
	if provider.PaymentRequestID != "" {
		payment, paymentErr := c.getPaymentRequest(ctx, provider.PaymentRequestID)
		if paymentErr != nil {
			return usecase.PaymentState{}, paymentErr
		}
		if payment.Status != "SUCCEEDED" || payment.ReferenceID != provider.ReferenceID || payment.Currency != provider.Currency || payment.Amount != entity.IDR(amount) {
			return usecase.PaymentState{}, errors.New("Xendit payment request does not match session")
		}
		channel = payment.Channel
	}
	return usecase.PaymentState{ProviderID: provider.PaymentSessionID, ReferenceID: provider.ReferenceID, Status: provider.Status,
		Currency: provider.Currency, Amount: entity.IDR(amount), Channel: channel}, nil
}

func providerAmount(number json.Number) (int64, error) {
	value := number.String()
	if integer, err := number.Int64(); err == nil {
		return integer, nil
	}
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 || strings.Trim(parts[1], "0") != "" {
		return 0, errors.New("Xendit amount is not an integer IDR value")
	}
	integer, err := json.Number(parts[0]).Int64()
	if err != nil {
		return 0, errors.New("Xendit amount is invalid")
	}
	return integer, nil
}

func (c *Client) CreateCheckout(ctx context.Context, input usecase.CheckoutInput) (usecase.Checkout, error) {
	allowedChannels, err := c.allowedPaymentChannels(input.Method)
	if err != nil {
		return usecase.Checkout{}, err
	}
	channelProperties := map[string]any{"expires_at": input.ExpiresAt.UTC().Format(time.RFC3339)}
	if len(allowedChannels) > 0 {
		channelProperties["allowed_payment_channels"] = allowedChannels
	}
	payload := map[string]any{
		"reference_id": input.OrderID, "session_type": "PAY", "mode": "PAYMENT_LINK", "country": "ID", "currency": "IDR",
		"amount": int64(input.Amount), "capture_method": "AUTOMATIC", "channel_properties": channelProperties,
		"description": "KailoPay sandbox XLM on-ramp",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return usecase.Checkout{}, fmt.Errorf("encoding Xendit payment request: %w", err)
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/sessions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return usecase.Checkout{}, fmt.Errorf("creating Xendit request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("api-version", c.apiVersion)
	request.SetBasicAuth(c.secretKey, "")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: fmt.Errorf("sending Xendit payment request: %w", err)}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > maxResponseBytes {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("reading Xendit payment response")}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: response.StatusCode >= 500, Err: fmt.Errorf("Xendit response status %d", response.StatusCode)}
	}
	var provider paymentSession
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	if err := decoder.Decode(&provider); err != nil {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("decoding Xendit payment session response")}
	}
	providerAmount, amountErr := providerAmount(provider.Amount)
	if amountErr != nil || provider.PaymentSessionID == "" || provider.ReferenceID != input.OrderID || provider.Currency != "IDR" ||
		providerAmount != int64(input.Amount) || !validPaymentLinkURL(provider.PaymentLinkURL) {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("Xendit payment session response mismatch")}
	}
	var expiresAt *time.Time
	if parsed, err := time.Parse(time.RFC3339Nano, provider.ExpiresAt); err == nil {
		parsed = parsed.UTC()
		expiresAt = &parsed
	}
	return usecase.Checkout{ProviderID: provider.PaymentSessionID, PaymentRequestID: provider.PaymentRequestID, Method: input.Method, Status: provider.Status,
		PresentationType: "PAYMENT_LINK", PresentationValue: provider.PaymentLinkURL, PaymentLinkURL: provider.PaymentLinkURL,
		ExpiresAt: expiresAt}, nil
}

func validPaymentLinkURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func (c *Client) allowedPaymentChannels(method entity.PaymentMethod) ([]string, error) {
	switch method {
	case usecase.PaymentMethodQRIS:
		return []string{c.qrisChannel}, nil
	case usecase.PaymentMethodBRIVA:
		return []string{c.vaChannel}, nil
	case usecase.PaymentMethodXendit:
		return nil, nil
	default:
		return nil, usecase.ErrInvalidCommand
	}
}
