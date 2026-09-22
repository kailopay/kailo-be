package repository

import (
	"fmt"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

func createSandboxOrderFinancial(tx *gorm.DB, order entity.OrderRecord) error {
	snapshotID, err := platform.NewID()
	if err != nil {
		return fmt.Errorf("generating order financial id: %w", err)
	}
	snapshot, err := usecase.BuildSandboxFinancialSnapshot(usecase.FinancialSnapshotInput{
		SnapshotID:         snapshotID,
		OrderID:            order.ID,
		ClientID:           order.ClientID,
		Direction:          order.Direction,
		Environment:        "test",
		Network:            order.Network,
		Currency:           order.Currency,
		AssetCode:          order.AssetCode,
		AssetIssuer:        order.AssetIssuer,
		AssetAmount:        order.AssetAmount,
		AssetAmountStroops: int64(order.AssetAmountStroops),
		GrossAmountMinor:   order.FiatAmountMinor,
		QuoteSpreadBPS:     order.QuoteSpreadBPS,
		CreatedAt:          order.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("building order financial snapshot: %w", err)
	}
	if err := tx.Create(&snapshot).Error; err != nil {
		return fmt.Errorf("creating order financial snapshot: %w", err)
	}
	return nil
}
