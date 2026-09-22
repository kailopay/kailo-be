package usecase

import (
	"context"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

const (
	Sep38IDRAsset = "iso4217:IDR"
	Sep38XLMAsset = "stellar:native"
)

var ErrInvalidSEP38Request = errors.New("invalid sep-38 request")

type Sep38PriceRequest struct {
	SellAsset  string
	BuyAsset   string
	SellAmount string
	BuyAmount  string
}

type Sep38Price struct {
	TotalPrice string
	Price      string
	SellAsset  string
	SellAmount string
	BuyAsset   string
	BuyAmount  string
}

type Sep38IndicativePrice struct {
	Price        string
	SellAsset    string
	BuyAsset     string
	BuyDecimals  int
	SellDecimals int
}

type Sep38UsecaseConfig struct {
	QuotePolicy QuotePolicy
	MinIDR      entity.IDR
	MaxIDR      entity.IDR
	Now         func() time.Time
}

type Sep38Usecase struct {
	prices PriceReader
	config Sep38UsecaseConfig
}

func NewSep38Usecase(prices PriceReader, config Sep38UsecaseConfig) (*Sep38Usecase, error) {
	if prices == nil || config.Now == nil || config.QuotePolicy.TTL <= 0 || config.QuotePolicy.MaxAge <= 0 ||
		config.QuotePolicy.SpreadBPS < 0 || config.QuotePolicy.SpreadBPS > 10_000 ||
		config.MinIDR <= 0 || config.MaxIDR < config.MinIDR {
		return nil, errors.New("valid sep-38 dependencies and configuration are required")
	}
	return &Sep38Usecase{prices: prices, config: config}, nil
}

func (s *Sep38Usecase) IndicativePrice(ctx context.Context, sellAsset, buyAsset string) (Sep38IndicativePrice, error) {
	sellAsset = strings.TrimSpace(sellAsset)
	buyAsset = strings.TrimSpace(buyAsset)
	if !validSEP38Pair(sellAsset, buyAsset) {
		return Sep38IndicativePrice{}, ErrInvalidSEP38Request
	}
	market, err := s.prices.LatestXLMIDR(ctx)
	if err != nil {
		return Sep38IndicativePrice{}, err
	}
	now := s.config.Now().UTC()
	if _, err := s.config.QuotePolicy.Create(now, s.config.MinIDR, market); err != nil {
		return Sep38IndicativePrice{}, err
	}
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(market.IDRPerXLM))
	if !ok || rate.Sign() <= 0 {
		return Sep38IndicativePrice{}, ErrInvalidPrice
	}
	adjusted := new(big.Rat).Mul(rate, new(big.Rat).SetFrac64(int64(10_000+s.config.QuotePolicy.SpreadBPS), 10_000))
	if sellAsset == Sep38XLMAsset {
		adjusted.Inv(adjusted)
	}
	return Sep38IndicativePrice{
		Price:        decimal(adjusted),
		SellAsset:    sellAsset,
		BuyAsset:     buyAsset,
		BuyDecimals:  sep38Decimals(buyAsset),
		SellDecimals: sep38Decimals(sellAsset),
	}, nil
}

func (s *Sep38Usecase) Price(ctx context.Context, request Sep38PriceRequest) (Sep38Price, error) {
	request.SellAsset = strings.TrimSpace(request.SellAsset)
	request.BuyAsset = strings.TrimSpace(request.BuyAsset)
	request.SellAmount = strings.TrimSpace(request.SellAmount)
	request.BuyAmount = strings.TrimSpace(request.BuyAmount)
	if !validSEP38Pair(request.SellAsset, request.BuyAsset) ||
		(request.SellAmount == "" && request.BuyAmount == "") ||
		(request.SellAmount != "" && request.BuyAmount != "") {
		return Sep38Price{}, ErrInvalidSEP38Request
	}

	market, err := s.prices.LatestXLMIDR(ctx)
	if err != nil {
		return Sep38Price{}, err
	}
	if request.SellAsset == Sep38IDRAsset {
		return s.priceIDRToXLM(request, market)
	}
	return s.priceXLMToIDR(request, market)
}

func (s *Sep38Usecase) priceIDRToXLM(request Sep38PriceRequest, market MarketPrice) (Sep38Price, error) {
	var quote Quote
	var err error
	if request.SellAmount != "" {
		amount, parseErr := parseSEP38IDR(request.SellAmount)
		if parseErr != nil || amount < int64(s.config.MinIDR) || amount > int64(s.config.MaxIDR) {
			return Sep38Price{}, ErrInvalidSEP38Request
		}
		quote, err = s.config.QuotePolicy.Create(s.config.Now().UTC(), entity.IDR(amount), market)
	} else {
		amount, parseErr := parseDecimalStroops(request.BuyAmount)
		if parseErr != nil {
			return Sep38Price{}, ErrInvalidSEP38Request
		}
		quote, err = s.config.QuotePolicy.CreateReverse(s.config.Now().UTC(), amount, market)
		if err == nil && (quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR) {
			err = ErrAmountOutOfRange
		}
	}
	if err != nil {
		return Sep38Price{}, err
	}
	return Sep38Price{
		TotalPrice: quote.AdjustedRate,
		Price:      quote.Rate,
		SellAsset:  request.SellAsset,
		SellAmount: strconv.FormatInt(int64(quote.FiatAmount), 10),
		BuyAsset:   request.BuyAsset,
		BuyAmount:  quote.AssetAmount.String(),
	}, nil
}

func (s *Sep38Usecase) priceXLMToIDR(request Sep38PriceRequest, market MarketPrice) (Sep38Price, error) {
	var quote Quote
	var err error
	if request.SellAmount != "" {
		amount, parseErr := parseDecimalStroops(request.SellAmount)
		if parseErr != nil {
			return Sep38Price{}, ErrInvalidSEP38Request
		}
		quote, err = s.config.QuotePolicy.CreateReverse(s.config.Now().UTC(), amount, market)
	} else {
		amount, parseErr := parseSEP38IDR(request.BuyAmount)
		if parseErr != nil || entity.IDR(amount) < s.config.MinIDR || entity.IDR(amount) > s.config.MaxIDR {
			return Sep38Price{}, ErrInvalidSEP38Request
		}
		quote, err = s.config.QuotePolicy.Create(s.config.Now().UTC(), entity.IDR(amount), market)
	}
	if err != nil {
		return Sep38Price{}, err
	}
	if quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR {
		return Sep38Price{}, ErrAmountOutOfRange
	}
	rate, ok := new(big.Rat).SetString(quote.Rate)
	if !ok {
		return Sep38Price{}, ErrInvalidPrice
	}
	adjustedRate, ok := new(big.Rat).SetString(quote.AdjustedRate)
	if !ok {
		return Sep38Price{}, ErrInvalidPrice
	}
	return Sep38Price{
		TotalPrice: decimal(new(big.Rat).Inv(adjustedRate)),
		Price:      decimal(new(big.Rat).Inv(rate)),
		SellAsset:  request.SellAsset,
		SellAmount: quote.AssetAmount.String(),
		BuyAsset:   request.BuyAsset,
		BuyAmount:  strconv.FormatInt(int64(quote.FiatAmount), 10),
	}, nil
}

func validSEP38Pair(sellAsset, buyAsset string) bool {
	return (sellAsset == Sep38IDRAsset && buyAsset == Sep38XLMAsset) ||
		(sellAsset == Sep38XLMAsset && buyAsset == Sep38IDRAsset)
}

func sep38Decimals(asset string) int {
	if asset == Sep38XLMAsset {
		return 7
	}
	return 0
}

func parseSEP38IDR(value string) (int64, error) {
	amount, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || amount <= 0 {
		return 0, ErrInvalidSEP38Request
	}
	return amount, nil
}
