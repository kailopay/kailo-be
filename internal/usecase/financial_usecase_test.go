package usecase

import (
	"testing"
	"time"
)

func TestBuildSandboxFinancialSnapshotUsesExactZeroFeePolicy(t *testing.T) {
	createdAt := time.Date(2026, 9, 22, 10, 30, 0, 0, time.UTC)
	clientID := "client-1"
	snapshot, err := BuildSandboxFinancialSnapshot(FinancialSnapshotInput{
		SnapshotID:         "financial-1",
		OrderID:            "order-1",
		ClientID:           &clientID,
		Direction:          "onramp",
		Environment:        "test",
		Network:            StellarTestnetNetwork,
		Currency:           IDRCurrency,
		AssetCode:          NativeXLMAssetCode,
		AssetAmount:        "12.3456789",
		AssetAmountStroops: 123456789,
		GrossAmountMinor:   987654321,
		QuoteSpreadBPS:     125,
		CreatedAt:          createdAt,
	})
	if err != nil {
		t.Fatalf("BuildSandboxFinancialSnapshot() error = %v", err)
	}
	if snapshot.ID != "financial-1" || snapshot.OrderID != "order-1" || snapshot.ClientID == nil || *snapshot.ClientID != clientID {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if snapshot.GrossAmountMinor != 987654321 || snapshot.NetAmountMinor != 987654321 {
		t.Fatalf("gross/net = %d/%d, want 987654321/987654321", snapshot.GrossAmountMinor, snapshot.NetAmountMinor)
	}
	if snapshot.FeeAmountMinor != 0 || snapshot.PlatformRevenueMinor != 0 || snapshot.DeveloperRevenueMinor != 0 {
		t.Fatalf("sandbox fee/revenue = %d/%d/%d, want zero", snapshot.FeeAmountMinor, snapshot.PlatformRevenueMinor, snapshot.DeveloperRevenueMinor)
	}
	if snapshot.FeeCurrency != IDRCurrency || snapshot.FeePolicyVersion != SandboxFeePolicyVersion || snapshot.Source != SandboxFinancialSource || !snapshot.Simulated {
		t.Fatalf("sandbox policy fields = %#v", snapshot)
	}
	if snapshot.AssetAmount != "12.3456789" || snapshot.AssetAmountStroops != 123456789 || snapshot.CreatedAt != createdAt {
		t.Fatalf("exact asset/audit fields = %#v", snapshot)
	}
}

func TestBuildSandboxFinancialSnapshotRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input FinancialSnapshotInput
	}{
		{name: "missing order", input: FinancialSnapshotInput{SnapshotID: "financial-1", Direction: "onramp", Environment: "test", Network: StellarTestnetNetwork, Currency: IDRCurrency, AssetCode: NativeXLMAssetCode, AssetAmount: "1", AssetAmountStroops: 10, GrossAmountMinor: 1, CreatedAt: time.Now()}},
		{name: "unsupported direction", input: FinancialSnapshotInput{SnapshotID: "financial-1", OrderID: "order-1", Direction: "transfer", Environment: "test", Network: StellarTestnetNetwork, Currency: IDRCurrency, AssetCode: NativeXLMAssetCode, AssetAmount: "1", AssetAmountStroops: 10, GrossAmountMinor: 1, CreatedAt: time.Now()}},
		{name: "zero asset", input: FinancialSnapshotInput{SnapshotID: "financial-1", OrderID: "order-1", Direction: "onramp", Environment: "test", Network: StellarTestnetNetwork, Currency: IDRCurrency, AssetCode: NativeXLMAssetCode, AssetAmount: "0", AssetAmountStroops: 0, GrossAmountMinor: 1, CreatedAt: time.Now()}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := BuildSandboxFinancialSnapshot(testCase.input); err == nil {
				t.Fatal("BuildSandboxFinancialSnapshot() error = nil")
			}
		})
	}
}
