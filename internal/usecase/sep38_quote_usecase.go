package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrSEP38QuoteNotFound = errors.New("sep-38 quote not found")
	ErrSEP38QuoteExpired  = errors.New("sep-38 quote expired")
	ErrSEP38QuoteConsumed = errors.New("sep-38 quote already consumed")
	ErrSEP38QuoteConflict = errors.New("sep-38 quote conflicts with the authenticated wallet")
)

type Sep38QuoteRequest struct {
	SellAsset          string
	BuyAsset           string
	SellAmount         string
	BuyAmount          string
	SellDeliveryMethod string
	BuyDeliveryMethod  string
}

type Sep38QuoteView struct {
	QuoteID            string
	SellAsset          string
	BuyAsset           string
	SellAmount         string
	BuyAmount          string
	Price              string
	SpreadBPS          int
	DeliveryMethod     string
	SellDeliveryMethod string
	BuyDeliveryMethod  string
	ExpiresAt          time.Time
}

type SEP38QuoteRepository interface {
	Create(ctx context.Context, quote entity.SEP38Quote) error
	Find(ctx context.Context, walletAccount, quoteID string) (entity.SEP38Quote, error)
	Consume(ctx context.Context, walletAccount, quoteID string, now time.Time) error
}

type Sep38QuoteDependencies struct {
	Prices PriceReader
	Quotes SEP38QuoteRepository
}

type Sep38QuoteConfig struct {
	QuotePolicy QuotePolicy
	MinIDR      entity.IDR
	MaxIDR      entity.IDR
	Now         func() time.Time
	NewID       func() (string, error)
}

type Sep38QuoteService interface {
	Create(ctx context.Context, principal SEP10Principal, request Sep38QuoteRequest) (Sep38QuoteView, error)
	Get(ctx context.Context, principal SEP10Principal, quoteID string) (Sep38QuoteView, error)
	Consume(ctx context.Context, principal SEP10Principal, quoteID string) error
}

type Sep38QuoteUsecase struct {
	dependencies Sep38QuoteDependencies
	config       Sep38QuoteConfig
}

func NewSep38QuoteUsecase(dependencies Sep38QuoteDependencies, config Sep38QuoteConfig) (*Sep38QuoteUsecase, error) {
	if dependencies.Prices == nil || dependencies.Quotes == nil || config.Now == nil || config.NewID == nil ||
		config.QuotePolicy.TTL <= 0 || config.QuotePolicy.MaxAge <= 0 || config.MinIDR <= 0 || config.MaxIDR < config.MinIDR {
		return nil, errors.New("valid SEP-38 quote dependencies and configuration are required")
	}
	return &Sep38QuoteUsecase{dependencies: dependencies, config: config}, nil
}

func (s *Sep38QuoteUsecase) Create(ctx context.Context, principal SEP10Principal, request Sep38QuoteRequest) (Sep38QuoteView, error) {
	if !validClassicAccount(strings.TrimSpace(principal.Account)) {
		return Sep38QuoteView{}, ErrSEP10InvalidAccount
	}
	request.SellAsset = strings.TrimSpace(request.SellAsset)
	request.BuyAsset = strings.TrimSpace(request.BuyAsset)
	request.SellAmount = strings.TrimSpace(request.SellAmount)
	request.BuyAmount = strings.TrimSpace(request.BuyAmount)
	request.SellDeliveryMethod = strings.TrimSpace(request.SellDeliveryMethod)
	request.BuyDeliveryMethod = strings.TrimSpace(request.BuyDeliveryMethod)
	if !validSEP38Pair(request.SellAsset, request.BuyAsset) ||
		(request.SellAmount == "") == (request.BuyAmount == "") {
		return Sep38QuoteView{}, ErrInvalidSEP38Request
	}
	market, err := s.dependencies.Prices.LatestXLMIDR(ctx)
	if err != nil {
		return Sep38QuoteView{}, err
	}
	now := s.config.Now().UTC()
	var quote Quote
	if request.SellAsset == Sep38IDRAsset {
		if request.SellDeliveryMethod == "" {
			request.SellDeliveryMethod = "xendit"
		}
		if request.SellAmount != "" {
			amount, parseErr := parseSEP38IDR(request.SellAmount)
			if parseErr != nil || entity.IDR(amount) < s.config.MinIDR || entity.IDR(amount) > s.config.MaxIDR {
				return Sep38QuoteView{}, ErrInvalidSEP38Request
			}
			quote, err = s.config.QuotePolicy.Create(now, entity.IDR(amount), market)
		} else {
			amount, parseErr := parseDecimalStroops(request.BuyAmount)
			if parseErr != nil {
				return Sep38QuoteView{}, ErrInvalidSEP38Request
			}
			quote, err = s.config.QuotePolicy.CreateReverse(now, amount, market)
			if err == nil && (quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR) {
				err = ErrAmountOutOfRange
			}
		}
	} else {
		if request.BuyDeliveryMethod == "" {
			request.BuyDeliveryMethod = "sandbox_bank_transfer"
		}
		if request.SellAmount != "" {
			amount, parseErr := parseDecimalStroops(request.SellAmount)
			if parseErr != nil {
				return Sep38QuoteView{}, ErrInvalidSEP38Request
			}
			quote, err = s.config.QuotePolicy.CreateReverse(now, amount, market)
			if err == nil && (quote.FiatAmount < s.config.MinIDR || quote.FiatAmount > s.config.MaxIDR) {
				err = ErrAmountOutOfRange
			}
		} else {
			amount, parseErr := parseSEP38IDR(request.BuyAmount)
			if parseErr != nil || entity.IDR(amount) < s.config.MinIDR || entity.IDR(amount) > s.config.MaxIDR {
				return Sep38QuoteView{}, ErrInvalidSEP38Request
			}
			quote, err = s.config.QuotePolicy.Create(now, entity.IDR(amount), market)
		}
	}
	if err != nil {
		return Sep38QuoteView{}, err
	}
	id, err := s.config.NewID()
	if err != nil {
		return Sep38QuoteView{}, fmt.Errorf("generating SEP-38 quote id: %w", err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Sep38QuoteView{}, ErrInvalidSEP38Request
	}
	quoteID := "quote-" + id
	row := entity.SEP38Quote{
		ID: id, QuoteID: quoteID, WalletAccount: strings.TrimSpace(principal.Account),
		SellAsset: request.SellAsset, BuyAsset: request.BuyAsset,
		SellAmount: quoteSellAmount(request, quote), BuyAmount: quoteBuyAmount(request, quote),
		Price: quote.Rate, SpreadBPS: quote.SpreadBPS,
		DeliveryMethod: quoteDeliveryMethod(request), ExpiresAt: quote.ExpiresAt,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.dependencies.Quotes.Create(ctx, row); err != nil {
		return Sep38QuoteView{}, err
	}
	return sep38QuoteView(row), nil
}

func (s *Sep38QuoteUsecase) Get(ctx context.Context, principal SEP10Principal, quoteID string) (Sep38QuoteView, error) {
	if !validClassicAccount(strings.TrimSpace(principal.Account)) {
		return Sep38QuoteView{}, ErrSEP10InvalidAccount
	}
	row, err := s.dependencies.Quotes.Find(ctx, strings.TrimSpace(principal.Account), strings.TrimSpace(quoteID))
	if err != nil {
		return Sep38QuoteView{}, err
	}
	if !s.config.Now().UTC().Before(row.ExpiresAt.UTC()) {
		return Sep38QuoteView{}, ErrSEP38QuoteExpired
	}
	if row.ConsumedAt != nil {
		return Sep38QuoteView{}, ErrSEP38QuoteConsumed
	}
	return sep38QuoteView(row), nil
}

func (s *Sep38QuoteUsecase) Consume(ctx context.Context, principal SEP10Principal, quoteID string) error {
	if !validClassicAccount(strings.TrimSpace(principal.Account)) {
		return ErrSEP10InvalidAccount
	}
	return s.dependencies.Quotes.Consume(ctx, strings.TrimSpace(principal.Account), strings.TrimSpace(quoteID), s.config.Now().UTC())
}

func quoteSellAmount(request Sep38QuoteRequest, quote Quote) string {
	if request.SellAmount != "" {
		if request.SellAsset == Sep38XLMAsset {
			amount, err := parseDecimalStroops(request.SellAmount)
			if err == nil {
				return amount.String()
			}
		}
		return request.SellAmount
	}
	if request.SellAsset == Sep38XLMAsset {
		return quote.AssetAmount.String()
	}
	return strconv.FormatInt(int64(quote.FiatAmount), 10)
}

func quoteBuyAmount(request Sep38QuoteRequest, quote Quote) string {
	if request.BuyAmount != "" {
		if request.BuyAsset == Sep38XLMAsset {
			amount, err := parseDecimalStroops(request.BuyAmount)
			if err == nil {
				return amount.String()
			}
		}
		return request.BuyAmount
	}
	if request.BuyAsset == Sep38IDRAsset {
		return strconv.FormatInt(int64(quote.FiatAmount), 10)
	}
	return quote.AssetAmount.String()
}

func quoteDeliveryMethod(request Sep38QuoteRequest) string {
	if request.SellAsset == Sep38IDRAsset {
		return request.SellDeliveryMethod
	}
	return request.BuyDeliveryMethod
}

func sep38QuoteView(row entity.SEP38Quote) Sep38QuoteView {
	return Sep38QuoteView{QuoteID: row.QuoteID, SellAsset: row.SellAsset, BuyAsset: row.BuyAsset,
		SellAmount: row.SellAmount, BuyAmount: row.BuyAmount, Price: row.Price,
		SpreadBPS: row.SpreadBPS, DeliveryMethod: row.DeliveryMethod,
		ExpiresAt: row.ExpiresAt,
		SellDeliveryMethod: func() string {
			if row.SellAsset == Sep38IDRAsset {
				return row.DeliveryMethod
			}
			return ""
		}(),
		BuyDeliveryMethod: func() string {
			if row.BuyAsset == Sep38IDRAsset {
				return row.DeliveryMethod
			}
			return ""
		}(),
	}
}
