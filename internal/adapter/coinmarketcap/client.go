package coinmarketcap

import (
	"bytes"
	"context"
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
		ErrorCode int `json:"error_code"`
	} `json:"status"`
	Data []latestQuoteAsset `json:"data"`
}

type latestQuoteAsset struct {
	ID    int                 `json:"id"`
	Quote latestCurrencyQuote `json:"quote"`
}

type latestCurrencyQuote struct {
	Symbol      string      `json:"symbol"`
	Price       json.Number `json:"price"`
	LastUpdated string      `json:"last_updated"`
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
	if err := decoder.Decode(&payload); err != nil || payload.Status.ErrorCode != 0 {
		return usecase.MarketPrice{}, fmt.Errorf("%w: invalid quote response", ErrUnavailable)
	}
	var xlm *latestQuoteAsset
	for index := range payload.Data {
		if payload.Data[index].ID == xlmCoinMarketCapID {
			xlm = &payload.Data[index]
			break
		}
	}
	if xlm == nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: XLM quote missing", ErrUnavailable)
	}
	if xlm.Quote.Symbol != quoteCurrencyIDR || xlm.Quote.Price.String() == "" {
		return usecase.MarketPrice{}, fmt.Errorf("%w: IDR quote missing", ErrUnavailable)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, xlm.Quote.LastUpdated)
	if err != nil {
		return usecase.MarketPrice{}, fmt.Errorf("%w: invalid quote timestamp", ErrUnavailable)
	}
	return usecase.MarketPrice{IDRPerXLM: xlm.Quote.Price.String(), ObservedAt: observedAt.UTC()}, nil
}
