package coinmarketcap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
)

const (
	defaultMaxResponseBytes int64 = 1 << 20
	xlmCoinMarketCapID            = 512
	quoteCurrencyIDR              = "IDR"
)

var ErrUnavailable = errors.New("coinmarketcap unavailable")

type Config struct {
	BaseURL          string
	APIKey           string
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

type Client struct {
	baseURL          *url.URL
	apiKey           string
	httpClient       *http.Client
	maxResponseBytes int64
}

type latestQuoteResponse struct {
	Status struct {
		ErrorCode *latestErrorCode `json:"error_code"`
	} `json:"status"`
	Data []latestQuoteAsset `json:"data"`
}

type latestErrorCode int

func (code *latestErrorCode) UnmarshalJSON(data []byte) error {
	raw := bytes.TrimSpace(data)
	if len(raw) == 0 {
		return errors.New("invalid coinmarketcap error code")
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return errors.New("invalid coinmarketcap error code")
		}
		raw = []byte(value)
	}
	parsed, err := strconv.Atoi(string(raw))
	if err != nil {
		return errors.New("invalid coinmarketcap error code")
	}
	*code = latestErrorCode(parsed)
	return nil
}

type latestQuoteAsset struct {
	ID     int                   `json:"id"`
	Symbol string                `json:"symbol"`
	Quote  []latestCurrencyQuote `json:"quote"`
}

type latestCurrencyQuote struct {
	ID          int         `json:"id"`
	Symbol      string      `json:"symbol"`
	Price       json.Number `json:"price"`
	LastUpdated string      `json:"last_updated"`
}

func validPrice(price json.Number) bool {
	value := price.String()
	if value == "" {
		return false
	}
	parsed, ok := new(big.Rat).SetString(value)
	return ok && parsed.Sign() > 0
}

func New(config Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(config.BaseURL, "/"))
	if err != nil || !baseURL.IsAbs() || strings.TrimSpace(config.APIKey) == "" || config.HTTPClient == nil {
		return nil, errors.New("valid CoinMarketCap configuration is required")
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	if maxResponseBytes < 1 {
		return nil, errors.New("CoinMarketCap response limit must be positive")
	}
	return &Client{baseURL: baseURL, apiKey: config.APIKey, httpClient: config.HTTPClient, maxResponseBytes: maxResponseBytes}, nil
}

func (c *Client) LatestXLMIDR(ctx context.Context) (usecase.MarketPrice, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + "/v3/cryptocurrency/quotes/latest"
	query := requestURL.Query()
	query.Set("id", strconv.Itoa(xlmCoinMarketCapID))
	query.Set("convert", quoteCurrencyIDR)
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return usecase.MarketPrice{}, fmt.Errorf("creating CoinMarketCap request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-CMC_PRO_API_KEY", c.apiKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: requesting quote: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: reading quote response: %v", ErrUnavailable, err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return usecase.MarketPrice{}, fmt.Errorf("%w: quote response too large", ErrUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return usecase.MarketPrice{}, fmt.Errorf("%w: quote response status %d", ErrUnavailable, response.StatusCode)
	}
	var payload latestQuoteResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil ||
		payload.Status.ErrorCode == nil ||
		*payload.Status.ErrorCode != 0 {
		return usecase.MarketPrice{}, fmt.Errorf("%w: invalid quote response", ErrUnavailable)
	}
	if len(payload.Data) == 0 {
		return usecase.MarketPrice{}, fmt.Errorf("%w: quote data missing", ErrUnavailable)
	}
	var xlm *latestQuoteAsset
	for index := range payload.Data {
		if payload.Data[index].ID == xlmCoinMarketCapID {
			xlm = &payload.Data[index]
			break
		}
	}
	if xlm == nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: unexpected asset ID", ErrUnavailable)
	}
	var idrQuote *latestCurrencyQuote
	for index := range xlm.Quote {
		if xlm.Quote[index].Symbol == quoteCurrencyIDR {
			idrQuote = &xlm.Quote[index]
			break
		}
	}
	if idrQuote == nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: IDR quote missing", ErrUnavailable)
	}
	if !validPrice(idrQuote.Price) {
		return usecase.MarketPrice{}, fmt.Errorf("%w: invalid IDR price", ErrUnavailable)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, idrQuote.LastUpdated)
	if err != nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: invalid quote timestamp", ErrUnavailable)
	}
	return usecase.MarketPrice{IDRPerXLM: idrQuote.Price.String(), ObservedAt: observedAt.UTC()}, nil
}
