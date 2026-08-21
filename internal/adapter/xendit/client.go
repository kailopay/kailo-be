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
			PaymentRequestID string `json:"payment_request_id"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil || payload.Event == "" || payload.Data.PaymentID == "" || payload.Data.PaymentRequestID == "" {
		return usecase.Callback{}, usecase.ErrInvalidCallback
	}
	return usecase.Callback{EventID: payload.Data.PaymentID, EventType: payload.Event, PaymentRequestID: payload.Data.PaymentRequestID}, nil
}

func (c *Client) GetPaymentRequest(ctx context.Context, providerID string) (usecase.PaymentState, error) {
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
	channel, descriptor, err := c.channel(input.Method)
	if err != nil {
		return usecase.Checkout{}, err
	}
	channelProperties := map[string]any{"expires_at": input.ExpiresAt.UTC().Format(time.RFC3339)}
	if input.Method == usecase.PaymentMethodBRIVA {
		channelProperties["display_name"] = "KailoPay Sandbox"
	}
	payload := map[string]any{
		"reference_id": input.OrderID, "type": "PAY", "country": "ID", "currency": "IDR",
		"request_amount": int64(input.Amount), "capture_method": "AUTOMATIC", "channel_code": channel,
		"channel_properties": channelProperties, "description": "KailoPay sandbox XLM on-ramp",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return usecase.Checkout{}, fmt.Errorf("encoding Xendit payment request: %w", err)
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v3/payment_requests"
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
	var provider paymentRequest
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	if err := decoder.Decode(&provider); err != nil {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("decoding Xendit payment response")}
	}
	providerAmount, amountErr := provider.RequestAmount.Int64()
	if amountErr != nil || provider.PaymentRequestID == "" || provider.ReferenceID != input.OrderID || provider.Currency != "IDR" ||
		providerAmount != int64(input.Amount) || provider.ChannelCode != channel {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("Xendit payment response mismatch")}
	}
	presentation := ""
	for _, action := range provider.Actions {
		if action.Type == "PRESENT_TO_CUSTOMER" && action.Descriptor == descriptor {
			presentation = action.Value
			break
		}
	}
	if presentation == "" {
		return usecase.Checkout{}, &usecase.GatewayError{Unknown: true, Err: errors.New("Xendit presentation action missing")}
	}
	var expiresAt *time.Time
	if parsed, err := time.Parse(time.RFC3339Nano, provider.ChannelProperties.ExpiresAt); err == nil {
		parsed = parsed.UTC()
		expiresAt = &parsed
	}
	return usecase.Checkout{ProviderID: provider.PaymentRequestID, Method: input.Method, Status: provider.Status,
		PresentationType: descriptor, PresentationValue: presentation, ExpiresAt: expiresAt}, nil
}

func (c *Client) channel(method entity.PaymentMethod) (string, string, error) {
	switch method {
	case usecase.PaymentMethodQRIS:
		return c.qrisChannel, "QR_STRING", nil
	case usecase.PaymentMethodBRIVA:
		return c.vaChannel, "VIRTUAL_ACCOUNT_NUMBER", nil
	default:
		return "", "", usecase.ErrInvalidCommand
	}
}
