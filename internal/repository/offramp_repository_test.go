package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
)

func TestValidateObservedDeposit(t *testing.T) {
	order := entity.OrderRecord{
		Direction:          "offramp",
		AssetAmountStroops: 40_000_000,
		StellarSource:      stringPtrForTest("GDEPOSIT"),
		StellarMemo:        stringPtrForTest("off-order-1"),
	}
	valid := usecase.ObservedPayment{TransactionHash: "hash-1", To: "GDEPOSIT", Amount: 40_000_000,
		Memo: "off-order-1", LedgerAt: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}

	tests := []struct {
		name    string
		payment usecase.ObservedPayment
		wantErr bool
	}{
		{name: "valid", payment: valid},
		{name: "empty hash", payment: func() usecase.ObservedPayment { p := valid; p.TransactionHash = ""; return p }(), wantErr: true},
		{name: "wrong destination", payment: func() usecase.ObservedPayment { p := valid; p.To = "GOTHER"; return p }(), wantErr: true},
		{name: "wrong amount", payment: func() usecase.ObservedPayment { p := valid; p.Amount++; return p }(), wantErr: true},
		{name: "wrong memo", payment: func() usecase.ObservedPayment { p := valid; p.Memo = "other"; return p }(), wantErr: true},
		{name: "missing ledger evidence", payment: func() usecase.ObservedPayment { p := valid; p.LedgerAt = time.Time{}; return p }(), wantErr: true},
		{name: "wrong order direction", payment: valid, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			testOrder := order
			if testCase.name == "wrong order direction" {
				testOrder.Direction = "onramp"
			}
			err := validateObservedDeposit(testOrder, testCase.payment)
			if testCase.wantErr {
				if !errors.Is(err, errInvalidDepositPayment) {
					t.Fatalf("validateObservedDeposit() error = %v, want %v", err, errInvalidDepositPayment)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateObservedDeposit() error = %v, want nil", err)
			}
		})
	}
}

func stringPtrForTest(value string) *string { return &value }
