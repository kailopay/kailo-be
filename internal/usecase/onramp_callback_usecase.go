package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/febry3/kailopay-be/internal/entity"
)

var (
	ErrInvalidCallback = errors.New("invalid payment callback")
	ErrPaymentMismatch = errors.New("payment evidence does not match checkout")
)

type Callback struct {
	EventID          string
	EventType        string
	PaymentRequestID string
}

type PaymentState struct {
	ProviderID  string
	ReferenceID string
	Status      string
	Currency    string
	Amount      entity.IDR
	Channel     string
}

type CallbackReceipt struct {
	EventID          string
	EventType        string
	PaymentRequestID string
	PayloadHash      string
}

type ExpectedPayment struct {
	OrderID     string
	ProviderID  string
	Amount      entity.IDR
	Currency    string
	Channel     string
	AssetAmount entity.Stroops
}

type PaymentConfirmation struct {
	EventID     string
	EventType   string
	PayloadHash string
	Expected    ExpectedPayment
	Actual      PaymentState
}

type CallbackGateway interface {
	VerifyCallback(raw []byte, token string) (Callback, error)
	GetPaymentRequest(ctx context.Context, providerID string) (PaymentState, error)
}

type CallbackStore interface {
	RecordCallbackReceipt(ctx context.Context, receipt CallbackReceipt) (processed bool, err error)
	ExpectedPayment(ctx context.Context, providerID string) (ExpectedPayment, error)
	ConfirmPaymentAndEnqueue(ctx context.Context, confirmation PaymentConfirmation) error
	CompleteCallback(ctx context.Context, eventID, result string) error
}

type OnrampCallbackUsecase struct {
	gateway CallbackGateway
	store   CallbackStore
}

func NewOnrampCallbackUsecase(gateway CallbackGateway, store CallbackStore) (*OnrampCallbackUsecase, error) {
	if gateway == nil || store == nil {
		return nil, errors.New("callback gateway and store are required")
	}
	return &OnrampCallbackUsecase{gateway: gateway, store: store}, nil
}

func (s *OnrampCallbackUsecase) Process(ctx context.Context, raw []byte, token string) error {
	callback, err := s.gateway.VerifyCallback(raw, token)
	if err != nil {
		return err
	}
	payloadDigest := sha256.Sum256(raw)
	payloadHash := hex.EncodeToString(payloadDigest[:])
	processed, err := s.store.RecordCallbackReceipt(ctx, CallbackReceipt{EventID: callback.EventID, EventType: callback.EventType,
		PaymentRequestID: callback.PaymentRequestID, PayloadHash: payloadHash})
	if err != nil {
		return fmt.Errorf("recording callback receipt: %w", err)
	}
	if processed {
		return nil
	}
	expected, err := s.store.ExpectedPayment(ctx, callback.PaymentRequestID)
	if err != nil {
		_ = s.store.CompleteCallback(ctx, callback.EventID, "unmatched")
		return fmt.Errorf("finding expected payment: %w", err)
	}
	actual, err := s.gateway.GetPaymentRequest(ctx, callback.PaymentRequestID)
	if err != nil {
		return fmt.Errorf("reconciling payment request: %w", err)
	}
	if callback.EventType != "payment.capture" || actual.Status != "SUCCEEDED" || actual.ProviderID != expected.ProviderID ||
		actual.ReferenceID != expected.OrderID || actual.Currency != expected.Currency || actual.Amount != expected.Amount || actual.Channel != expected.Channel {
		_ = s.store.CompleteCallback(ctx, callback.EventID, "mismatched")
		return ErrPaymentMismatch
	}
	if err := s.store.ConfirmPaymentAndEnqueue(ctx, PaymentConfirmation{EventID: callback.EventID, EventType: callback.EventType,
		PayloadHash: payloadHash, Expected: expected, Actual: actual}); err != nil {
		return fmt.Errorf("confirming payment: %w", err)
	}
	return s.store.CompleteCallback(ctx, callback.EventID, "processed")
}
