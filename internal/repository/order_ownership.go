package repository

import (
	"context"
	"errors"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
)

type orderOwnership struct {
	ClientID        *string
	CreatedByUserID *string
	RetailSessionID *string
	RetailUserID    *string
}

func orderOwnershipFor(principal usecase.OrderPrincipal) (orderOwnership, error) {
	if err := principal.Validate(); err != nil {
		return orderOwnership{}, err
	}
	switch principal.Kind {
	case usecase.OrderPrincipalAPIClient:
		clientID := principal.ClientID
		return orderOwnership{ClientID: &clientID}, nil
	case usecase.OrderPrincipalRetailSession:
		userID := principal.OwnerUserID
		sessionID := principal.SessionID
		return orderOwnership{CreatedByUserID: &userID, RetailSessionID: &sessionID, RetailUserID: &userID}, nil
	default:
		return orderOwnership{}, usecase.ErrInvalidOrderPrincipal
	}
}

func validateRetailSessionOwnership(ctx context.Context, tx *gorm.DB, principal usecase.OrderPrincipal) error {
	if !principal.IsRetailSession() {
		return nil
	}
	var session entity.RetailSession
	if err := tx.WithContext(ctx).Select("user_id").Where("id = ?", principal.SessionID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.ErrInvalidOrderPrincipal
		}
		return err
	}
	if session.UserID != principal.OwnerUserID {
		return usecase.ErrInvalidOrderPrincipal
	}
	return nil
}

func applyOrderOwnership(query *gorm.DB, principal usecase.OrderPrincipal) (*gorm.DB, error) {
	if err := principal.Validate(); err != nil {
		return nil, err
	}
	if principal.IsAPIClient() {
		return query.Where("client_id = ? AND retail_session_id IS NULL", principal.ClientID), nil
	}
	return query.Where("client_id IS NULL AND retail_session_id IS NOT NULL AND created_by_user_id = ?", principal.OwnerUserID), nil
}

func applyIdempotencyOwnership(query *gorm.DB, principal usecase.OrderPrincipal) (*gorm.DB, error) {
	if err := principal.Validate(); err != nil {
		return nil, err
	}
	if principal.IsAPIClient() {
		return query.Where("client_id = ? AND retail_user_id IS NULL", principal.ClientID), nil
	}
	return query.Where("client_id IS NULL AND retail_user_id = ?", principal.OwnerUserID), nil
}
