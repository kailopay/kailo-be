package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeveloperWalletRepository struct {
	db *gorm.DB
}

func NewDeveloperWalletRepository(db *gorm.DB) *DeveloperWalletRepository {
	return &DeveloperWalletRepository{db: db}
}

func (r *DeveloperWalletRepository) Upsert(ctx context.Context, wallet entity.DeveloperWallet) (usecase.DeveloperWalletView, error) {
	var result entity.DeveloperWallet
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if wallet.ClientID != nil {
			var client entity.APIClient
			if err := tx.Where("id = ? AND owner_user_id = ?", *wallet.ClientID, wallet.UserID).First(&client).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return usecase.ErrDeveloperWalletNotFound
				}
				return fmt.Errorf("checking developer wallet client: %w", err)
			}
			if client.Status != "active" {
				return usecase.ErrDeveloperWalletNotFound
			}
		}

		var existing entity.DeveloperWallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND network = ? AND wallet_account = ?", wallet.UserID, wallet.Network, wallet.WalletAccount).
			First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if wallet.IsPrimary {
				if err := demotePrimaryDeveloperWallet(tx, wallet.UserID, wallet.Network, ""); err != nil {
					return err
				}
			}
			if err := tx.Create(&wallet).Error; err != nil {
				return fmt.Errorf("creating developer wallet: %w", err)
			}
			result = wallet
			return nil
		}
		if err != nil {
			return fmt.Errorf("finding developer wallet: %w", err)
		}
		if wallet.IsPrimary {
			if err := demotePrimaryDeveloperWallet(tx, wallet.UserID, wallet.Network, existing.ID); err != nil {
				return err
			}
		}
		updates := map[string]any{
			"client_id":           wallet.ClientID,
			"label":               wallet.Label,
			"is_primary":          wallet.IsPrimary,
			"verification_method": wallet.VerificationMethod,
			"verified_at":         wallet.VerifiedAt,
			"status":              "active",
			"revoked_at":          nil,
			"updated_at":          wallet.UpdatedAt,
		}
		if err := tx.Model(&entity.DeveloperWallet{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("updating developer wallet: %w", err)
		}
		existing.ClientID = wallet.ClientID
		existing.Label = wallet.Label
		existing.IsPrimary = wallet.IsPrimary
		existing.VerificationMethod = wallet.VerificationMethod
		existing.VerifiedAt = wallet.VerifiedAt
		existing.Status = "active"
		existing.RevokedAt = nil
		existing.UpdatedAt = wallet.UpdatedAt
		result = existing
		return nil
	})
	if err != nil {
		return usecase.DeveloperWalletView{}, err
	}
	return developerWalletView(result), nil
}

func demotePrimaryDeveloperWallet(tx *gorm.DB, userID, network, exceptID string) error {
	query := tx.Model(&entity.DeveloperWallet{}).
		Where("user_id = ? AND network = ? AND status = ? AND is_primary = ?", userID, network, "active", true)
	if exceptID != "" {
		query = query.Where("id <> ?", exceptID)
	}
	if err := query.Update("is_primary", false).Error; err != nil {
		return fmt.Errorf("demoting developer wallet primary: %w", err)
	}
	return nil
}

func (r *DeveloperWalletRepository) List(ctx context.Context, userID string) ([]usecase.DeveloperWalletView, error) {
	rows := make([]entity.DeveloperWallet, 0)
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing developer wallets: %w", err)
	}
	views := make([]usecase.DeveloperWalletView, 0, len(rows))
	for _, row := range rows {
		views = append(views, developerWalletView(row))
	}
	return views, nil
}

func (r *DeveloperWalletRepository) Update(ctx context.Context, userID, walletID string, update usecase.DeveloperWalletUpdate, now time.Time) (usecase.DeveloperWalletView, error) {
	var result entity.DeveloperWallet
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var wallet entity.DeveloperWallet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", walletID, userID).First(&wallet).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrDeveloperWalletNotFound
			}
			return fmt.Errorf("finding developer wallet: %w", err)
		}
		if wallet.Status != "active" {
			return usecase.ErrDeveloperWalletNotFound
		}
		if update.IsPrimary != nil && *update.IsPrimary {
			if err := demotePrimaryDeveloperWallet(tx, userID, wallet.Network, wallet.ID); err != nil {
				return err
			}
		}
		updates := map[string]any{"updated_at": now.UTC()}
		if update.Label != nil {
			updates["label"] = *update.Label
			wallet.Label = *update.Label
		}
		if update.IsPrimary != nil {
			updates["is_primary"] = *update.IsPrimary
			wallet.IsPrimary = *update.IsPrimary
		}
		if err := tx.Model(&entity.DeveloperWallet{}).Where("id = ?", wallet.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("updating developer wallet settings: %w", err)
		}
		wallet.UpdatedAt = now.UTC()
		result = wallet
		return nil
	})
	if err != nil {
		return usecase.DeveloperWalletView{}, err
	}
	return developerWalletView(result), nil
}

func (r *DeveloperWalletRepository) Revoke(ctx context.Context, userID, walletID string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&entity.DeveloperWallet{}).
		Where("id = ? AND user_id = ? AND status = ?", walletID, userID, "active").
		Updates(map[string]any{"status": "revoked", "is_primary": false, "revoked_at": now.UTC(), "updated_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("revoking developer wallet: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var wallet entity.DeveloperWallet
	if err := r.db.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", walletID, userID).First(&wallet).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.ErrDeveloperWalletNotFound
	}
	return nil
}

func developerWalletView(wallet entity.DeveloperWallet) usecase.DeveloperWalletView {
	view := usecase.DeveloperWalletView{
		ID: wallet.ID, UserID: wallet.UserID, Network: wallet.Network, WalletAccount: wallet.WalletAccount,
		Label: wallet.Label, IsPrimary: wallet.IsPrimary, VerificationMethod: wallet.VerificationMethod,
		VerifiedAt: wallet.VerifiedAt.UTC(), Status: wallet.Status, RevokedAt: wallet.RevokedAt,
		CreatedAt: wallet.CreatedAt.UTC(), UpdatedAt: wallet.UpdatedAt.UTC(),
	}
	if wallet.ClientID != nil {
		view.ClientID = *wallet.ClientID
	}
	return view
}

var _ usecase.DeveloperWalletRepository = (*DeveloperWalletRepository)(nil)
