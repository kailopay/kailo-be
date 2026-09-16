package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	QuoteDirectionBuy  QuoteDirection = "buy"
	QuoteDirectionSell QuoteDirection = "sell"
)

type QuoteDirection string

var ErrInvalidQuoteRequest = errors.New("invalid quote request")

// QuotePreviewCommand contains the amount on the side the customer wants to
// exchange. The other amount is calculated by the quote policy.
type QuotePreviewCommand struct {
	Principal       OrderPrincipal
	Direction       QuoteDirection
	FiatCurrency    string
	FiatAmountMinor entity.IDR
	AssetNetwork    string
	AssetCode       string
	AssetAmount     string
}

type QuotePreview struct {
	Direction QuoteDirection
	Quote     Quote
}

type QuotePreviewServiceConfig struct {
	QuotePolicy QuotePolicy
	MinIDR      entity.IDR
	MaxIDR      entity.IDR
	Now         func() time.Time
}

type QuotePreviewUsecase struct {
	prices PriceReader
	config QuotePreviewServiceConfig
}

func NewQuotePreviewUsecase(prices PriceReader, config QuotePreviewServiceConfig) (*QuotePreviewUsecase, error) {
	if prices == nil || config.Now == nil || config.QuotePolicy.TTL <= 0 || config.QuotePolicy.MaxAge <= 0 ||
		config.QuotePolicy.SpreadBPS < 0 || config.QuotePolicy.SpreadBPS > 10_000 || config.MinIDR <= 0 || config.MaxIDR < config.MinIDR {
		return nil, errors.New("valid quote preview dependencies and configuration are required")
	}
	return &QuotePreviewUsecase{prices: prices, config: config}, nil
}

func (s *QuotePreviewUsecase) Preview(ctx context.Context, command QuotePreviewCommand) (QuotePreview, error) {
	if err := command.Principal.Validate(); err != nil {
		return QuotePreview{}, ErrInvalidQuoteRequest
	}
	command.FiatCurrency = strings.TrimSpace(command.FiatCurrency)
	command.AssetNetwork = strings.TrimSpace(command.AssetNetwork)
	command.AssetCode = strings.TrimSpace(command.AssetCode)
	command.AssetAmount = strings.TrimSpace(command.AssetAmount)
	command.Direction = QuoteDirection(strings.TrimSpace(string(command.Direction)))
	if command.FiatCurrency != IDRCurrency || command.AssetNetwork != StellarTestnetNetwork || command.AssetCode != NativeXLMAssetCode {
		return QuotePreview{}, ErrInvalidQuoteRequest
	}

	var assetAmount entity.Stroops
	switch command.Direction {
	case QuoteDirectionBuy:
		if command.FiatAmountMinor.Validate() != nil || command.AssetAmount != "" {
			return QuotePreview{}, ErrInvalidQuoteRequest
		}
		if command.FiatAmountMinor < s.config.MinIDR || command.FiatAmountMinor > s.config.MaxIDR {
			return QuotePreview{}, ErrAmountOutOfRange
		}
	case QuoteDirectionSell:
		if command.FiatAmountMinor != 0 {
			return QuotePreview{}, ErrInvalidQuoteRequest
		}
		var err error
		assetAmount, err = parseDecimalStroops(command.AssetAmount)
		if err != nil {
			return QuotePreview{}, ErrInvalidQuoteRequest
		}
	default:
		return QuotePreview{}, ErrInvalidQuoteRequest
	}

	now := s.config.Now().UTC()
	market, err := s.prices.LatestXLMIDR(ctx)
	if err != nil {
		return QuotePreview{}, fmt.Errorf("reading XLM IDR price: %w", err)
	}

	if command.Direction == QuoteDirectionBuy {
		quote, err := s.config.QuotePolicy.Create(now, command.FiatAmountMinor, market)
		if err != nil {
			return QuotePreview{}, fmt.Errorf("creating quote: %w", err)
		}
		return QuotePreview{Direction: command.Direction, Quote: quote}, nil
	}
	quote, err := s.config.QuotePolicy.CreateReverse(now, assetAmount, market)
	if err != nil {
		return QuotePreview{}, fmt.Errorf("creating quote: %w", err)
	}
	if quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR {
		return QuotePreview{}, ErrAmountOutOfRange
	}
	return QuotePreview{Direction: command.Direction, Quote: quote}, nil
}
