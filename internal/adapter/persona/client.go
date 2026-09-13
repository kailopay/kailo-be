package persona

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
)

const (
	personaAPIPath     = "/api/v1"
	personaAPIVersion  = "2025-10-27"
	maxAllowedResponse = 16 << 20
)

var errInvalidPersonaResponse = errors.New("invalid persona response")

type Config struct {
	BaseURL            string
	APIKey             string
	TemplateID         string
	WebhookSecret      string
	Timeout            time.Duration
	SignatureTolerance time.Duration
	MaxResponseBytes   int64
	HTTPClient         *http.Client
}

type Client struct {
	baseURL            *url.URL
	apiKey             string
	templateID         string
	webhookSecret      string
	timeout            time.Duration
	signatureTolerance time.Duration
	maxResponseBytes   int64
	httpClient         *http.Client
}

func New(config Config) (*Client, error) {
	baseURL, err := parseBaseURL(config.BaseURL)
	if err != nil || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.TemplateID) == "" ||
		len(strings.TrimSpace(config.WebhookSecret)) < 32 || config.Timeout <= 0 || config.SignatureTolerance <= 0 ||
		config.MaxResponseBytes <= 0 || config.MaxResponseBytes > maxAllowedResponse || config.HTTPClient == nil {
		return nil, errors.New("valid persona configuration is required")
	}
	return &Client{
		baseURL:            baseURL,
		apiKey:             config.APIKey,
		templateID:         config.TemplateID,
		webhookSecret:      config.WebhookSecret,
		timeout:            config.Timeout,
		signatureTolerance: config.SignatureTolerance,
		maxResponseBytes:   config.MaxResponseBytes,
		httpClient:         config.HTTPClient,
	}, nil
}

func (c *Client) CreateInquiry(ctx context.Context, referenceID, idempotencyKey string) (usecase.PersonaInquiry, error) {
	payload := struct {
		Data struct {
			Type       string `json:"type"`
			Attributes struct {
				InquiryTemplateID string `json:"inquiry-template-id"`
				ReferenceID       string `json:"reference-id"`
			} `json:"attributes"`
		} `json:"data"`
	}{}
	payload.Data.Type = "inquiry"
	payload.Data.Attributes.InquiryTemplateID = c.templateID
	payload.Data.Attributes.ReferenceID = strings.TrimSpace(referenceID)
	body, err := json.Marshal(payload)
	if err != nil {
		return usecase.PersonaInquiry{}, fmt.Errorf("encoding persona inquiry request: %w", err)
	}

	responseBody, err := c.doJSON(ctx, http.MethodPost, "/inquiries", idempotencyKey, body)
	if err != nil {
		return usecase.PersonaInquiry{}, err
	}
	var response struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Status    string `json:"status"`
				ExpiresAt string `json:"expires-at"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil || strings.TrimSpace(response.Data.ID) == "" {
		return usecase.PersonaInquiry{}, errInvalidPersonaResponse
	}
	expiresAt, err := parseOptionalTime(response.Data.Attributes.ExpiresAt)
	if err != nil {
		return usecase.PersonaInquiry{}, errInvalidPersonaResponse
	}
	return usecase.PersonaInquiry{ID: response.Data.ID, Status: response.Data.Attributes.Status, ExpiresAt: expiresAt}, nil
}

func (c *Client) ResumeInquiry(ctx context.Context, providerInquiryID string) (string, error) {
	body := []byte(`{"meta":{}}`)
	responseBody, err := c.doJSON(ctx, http.MethodPost, "/inquiries/"+url.PathEscape(strings.TrimSpace(providerInquiryID))+"/resume", "", body)
	if err != nil {
		return "", err
	}
	var response struct {
		Meta struct {
			SessionToken string `json:"session-token"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil || strings.TrimSpace(response.Meta.SessionToken) == "" {
		return "", errInvalidPersonaResponse
	}
	return response.Meta.SessionToken, nil
}

func (c *Client) VerifyWebhook(rawBody []byte, signature string, now time.Time) (usecase.PersonaEvent, error) {
	if len(rawBody) == 0 {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	timestamp, signatures, ok := parseSignature(signature)
	if !ok {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	parsedTimestamp, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	signatureTime := time.Unix(parsedTimestamp, 0)
	if signatureTime.Before(now.Add(-c.signatureTolerance)) || signatureTime.After(now.Add(c.signatureTolerance)) {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}

	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	_, _ = mac.Write([]byte(timestamp + "." + string(rawBody)))
	expected := mac.Sum(nil)
	valid := false
	for _, candidate := range signatures {
		decoded, decodeErr := hex.DecodeString(candidate)
		if decodeErr == nil && hmac.Equal(expected, decoded) {
			valid = true
			break
		}
	}
	if !valid {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}

	var payload struct {
		Data struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Name      string `json:"name"`
				CreatedAt string `json:"created-at"`
				Payload   struct {
					Data struct {
						ID         string `json:"id"`
						Type       string `json:"type"`
						Attributes struct {
							Status string `json:"status"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"payload"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	if payload.Data.Type != "event" || strings.TrimSpace(payload.Data.ID) == "" ||
		strings.TrimSpace(payload.Data.Attributes.Name) == "" || payload.Data.Attributes.Payload.Data.Type != "inquiry" ||
		strings.TrimSpace(payload.Data.Attributes.Payload.Data.ID) == "" {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	createdAt, err := time.Parse(time.RFC3339Nano, payload.Data.Attributes.CreatedAt)
	if err != nil {
		return usecase.PersonaEvent{}, errInvalidPersonaResponse
	}
	return usecase.PersonaEvent{
		ID: payload.Data.ID, Type: payload.Data.Attributes.Name, InquiryID: payload.Data.Attributes.Payload.Data.ID,
		Status: payload.Data.Attributes.Payload.Data.Attributes.Status, CreatedAt: createdAt.UTC(),
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path, idempotencyKey string, body []byte) ([]byte, error) {
	requestContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + personaAPIPath + path
	endpoint.RawPath = ""
	request, err := http.NewRequestWithContext(requestContext, method, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating persona request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Persona-Version", personaAPIVersion)
	if strings.TrimSpace(idempotencyKey) != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("sending persona request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("persona request returned status %d", response.StatusCode)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading persona response: %w", err)
	}
	if int64(len(responseBody)) > c.maxResponseBytes {
		return nil, errors.New("persona response exceeds the configured size limit")
	}
	return responseBody, nil
}

func parseBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("persona base URL is invalid")
	}
	if parsed.Scheme == "https" {
		return parsed, nil
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		return parsed, nil
	}
	return nil, errors.New("persona base URL must use HTTPS")
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func parseSignature(raw string) (string, []string, bool) {
	var timestamp string
	var signatures []string
	for _, group := range strings.Fields(strings.TrimSpace(raw)) {
		for _, part := range strings.Split(group, ",") {
			keyValue := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(keyValue) != 2 || strings.TrimSpace(keyValue[1]) == "" {
				return "", nil, false
			}
			switch strings.TrimSpace(keyValue[0]) {
			case "t":
				if timestamp != "" && timestamp != keyValue[1] {
					return "", nil, false
				}
				timestamp = keyValue[1]
			case "v1":
				signatures = append(signatures, keyValue[1])
			default:
				return "", nil, false
			}
		}
	}
	return timestamp, signatures, timestamp != "" && len(signatures) > 0
}

var _ usecase.PersonaGateway = (*Client)(nil)
