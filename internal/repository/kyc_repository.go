package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/febry3/kailopay-be/internal/usecase"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const kycProvider = "persona"

type KYCRepository struct {
	db *gorm.DB
	tx txManager
}

func NewKYCRepository(db *gorm.DB) *KYCRepository {
	return &KYCRepository{db: db, tx: newTxManager(db)}
}

func (r *KYCRepository) FindCurrent(ctx context.Context, userID string) (usecase.KYCInquiryRecord, bool, error) {
	var row entity.KYCInquiry
	err := r.db.WithContext(ctx).Where("user_id = ?", strings.TrimSpace(userID)).
		Order("created_at DESC, id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.KYCInquiryRecord{}, false, nil
	}
	if err != nil {
		return usecase.KYCInquiryRecord{}, false, fmt.Errorf("finding current kyc inquiry: %w", err)
	}
	return kycInquiryRecord(row), true, nil
}

func (r *KYCRepository) FindByProviderInquiryID(ctx context.Context, provider, providerInquiryID string) (usecase.KYCInquiryRecord, bool, error) {
	var row entity.KYCInquiry
	err := r.db.WithContext(ctx).Where("provider = ? AND provider_inquiry_id = ?", strings.TrimSpace(provider), strings.TrimSpace(providerInquiryID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return usecase.KYCInquiryRecord{}, false, nil
	}
	if err != nil {
		return usecase.KYCInquiryRecord{}, false, fmt.Errorf("finding kyc inquiry by provider id: %w", err)
	}
	return kycInquiryRecord(row), true, nil
}

func (r *KYCRepository) Reserve(ctx context.Context, userID, internalID, requestKey string, now time.Time) (usecase.KYCInquiryRecord, bool, error) {
	userID = strings.TrimSpace(userID)
	internalID = strings.TrimSpace(internalID)
	requestKey = strings.TrimSpace(requestKey)
	now = now.UTC()
	var result usecase.KYCInquiryRecord
	created := false
	err := r.tx.do(ctx, func(tx *gorm.DB) error {
		var user entity.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", userID, activeUserStatus).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return usecase.ErrKYCInquiryNotFound
		}
		if err != nil {
			return fmt.Errorf("locking kyc user: %w", err)
		}

		var existing entity.KYCInquiry
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).
			Order("created_at DESC, id DESC").First(&existing).Error
		if err == nil {
			if existing.Status == entity.KYCInquiryApproved || retainInquiry(existing, now) {
				result = kycInquiryRecord(existing)
				return nil
			}
			if isExpirableInquiry(existing.Status) && existing.ExpiresAt != nil && !existing.ExpiresAt.After(now) {
				if err := tx.Model(&entity.KYCInquiry{}).Where("id = ?", existing.ID).Updates(map[string]any{
					"status": string(entity.KYCInquiryExpired), "updated_at": now,
				}).Error; err != nil {
					return fmt.Errorf("expiring previous kyc inquiry: %w", err)
				}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("finding current kyc inquiry for reservation: %w", err)
		}

		row := entity.KYCInquiry{
			ID: internalID, UserID: userID, Provider: kycProvider, ProviderRequestKey: requestKey,
			Status: entity.KYCInquiryCreating, CreatedAt: now, UpdatedAt: now,
		}
		createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if createResult.Error != nil {
			return fmt.Errorf("creating kyc inquiry reservation: %w", createResult.Error)
		}
		if createResult.RowsAffected == 0 {
			var concurrent entity.KYCInquiry
			findErr := tx.Where("user_id = ? AND status IN ?", userID, activeKYCEntityStatuses()).
				Order("created_at DESC, id DESC").First(&concurrent).Error
			if findErr == nil {
				result = kycInquiryRecord(concurrent)
				return nil
			}
			if !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("finding concurrent kyc inquiry reservation: %w", findErr)
			}
			return errors.New("kyc inquiry reservation conflicts with an existing record")
		}
		result = kycInquiryRecord(row)
		created = true
		return nil
	})
	if err != nil {
		return usecase.KYCInquiryRecord{}, false, err
	}
	return result, created, nil
}

func (r *KYCRepository) AttachProviderInquiry(ctx context.Context, internalID, providerID, providerStatus string, expiresAt *time.Time, now time.Time) error {
	internalID = strings.TrimSpace(internalID)
	providerID = strings.TrimSpace(providerID)
	if internalID == "" || providerID == "" {
		return errors.New("kyc inquiry and provider inquiry ids are required")
	}
	now = now.UTC()
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var row entity.KYCInquiry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", internalID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrKYCInquiryNotFound
			}
			return fmt.Errorf("finding kyc inquiry reservation: %w", err)
		}
		if row.ProviderInquiryID != nil {
			if *row.ProviderInquiryID == providerID {
				return nil
			}
			return errors.New("kyc inquiry is already attached to another provider inquiry")
		}
		if row.Status != entity.KYCInquiryCreating {
			return errors.New("kyc inquiry is not awaiting provider attachment")
		}
		if err := tx.Model(&entity.KYCInquiry{}).Where("id = ? AND status = ? AND provider_inquiry_id IS NULL", internalID, entity.KYCInquiryCreating).
			Updates(map[string]any{
				"provider_inquiry_id": providerID, "provider_status": strings.TrimSpace(providerStatus),
				"status": string(entity.KYCInquiryCreated), "expires_at": expiresAt, "updated_at": now,
			}).Error; err != nil {
			return fmt.Errorf("attaching provider inquiry: %w", err)
		}
		return nil
	})
}

func (r *KYCRepository) RecordProviderEvent(ctx context.Context, event usecase.KYCProviderEventRecord, mappedStatus usecase.KYCStatus, providerStatus string, now time.Time) error {
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.Provider) == "" || strings.TrimSpace(event.ProviderEventID) == "" ||
		strings.TrimSpace(event.InquiryID) == "" || strings.TrimSpace(event.EventType) == "" || strings.TrimSpace(event.PayloadHash) == "" ||
		event.ProviderEventAt.IsZero() || event.ReceivedAt.IsZero() || !validKYCEventStatus(mappedStatus) {
		return errors.New("invalid kyc provider event")
	}
	event.Provider = strings.TrimSpace(event.Provider)
	event.ProviderEventID = strings.TrimSpace(event.ProviderEventID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.ProviderEventAt = event.ProviderEventAt.UTC()
	event.ReceivedAt = event.ReceivedAt.UTC()
	now = now.UTC()
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		var inquiry entity.KYCInquiry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND provider = ?", event.InquiryID, event.Provider).First(&inquiry).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return usecase.ErrKYCInquiryNotFound
			}
			return fmt.Errorf("finding kyc inquiry for provider event: %w", err)
		}

		var existing entity.KYCProviderEvent
		err := tx.Where("provider = ? AND provider_event_id = ?", event.Provider, event.ProviderEventID).First(&existing).Error
		if err == nil {
			if existing.PayloadHash != event.PayloadHash || existing.InquiryID != event.InquiryID {
				return usecase.ErrKYCProviderEventConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("finding existing kyc provider event: %w", err)
		}

		row := entity.KYCProviderEvent{
			ID: event.ID, Provider: event.Provider, ProviderEventID: event.ProviderEventID, InquiryID: event.InquiryID,
			EventType: event.EventType, ProviderEventAt: event.ProviderEventAt, PayloadHash: event.PayloadHash, ReceivedAt: event.ReceivedAt,
		}
		createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if createResult.Error != nil {
			return fmt.Errorf("creating kyc provider event: %w", createResult.Error)
		}
		if createResult.RowsAffected == 0 {
			var concurrent entity.KYCProviderEvent
			findErr := tx.Where("provider = ? AND provider_event_id = ?", event.Provider, event.ProviderEventID).First(&concurrent).Error
			if findErr != nil {
				if errors.Is(findErr, gorm.ErrRecordNotFound) {
					return errors.New("kyc provider event conflicts with an existing record")
				}
				return fmt.Errorf("finding concurrent kyc provider event: %w", findErr)
			}
			if concurrent.PayloadHash != event.PayloadHash || concurrent.InquiryID != event.InquiryID {
				return usecase.ErrKYCProviderEventConflict
			}
			return nil
		}

		if !isProviderEventNewer(inquiry.ProviderEventAt, inquiry.LastProviderEventID, event.ProviderEventAt, event.ProviderEventID) {
			return nil
		}
		updates := map[string]any{
			"provider_event_at":      event.ProviderEventAt,
			"last_provider_event_id": event.ProviderEventID,
			"provider_status":        strings.TrimSpace(providerStatus),
			"status":                 string(mappedStatus),
			"approved_at":            nil,
			"updated_at":             now,
		}
		switch mappedStatus {
		case usecase.KYCStatusPending:
			if inquiry.StartedAt == nil {
				updates["started_at"] = event.ProviderEventAt
			}
		case usecase.KYCStatusCompleted:
			if inquiry.CompletedAt == nil {
				updates["completed_at"] = event.ProviderEventAt
			}
		case usecase.KYCStatusApproved:
			updates["approved_at"] = event.ProviderEventAt
		}
		if err := tx.Model(&entity.KYCInquiry{}).Where("id = ?", inquiry.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("updating kyc inquiry from provider event: %w", err)
		}
		return nil
	})
}

func isProviderEventNewer(currentAt *time.Time, currentID *string, eventAt time.Time, eventID string) bool {
	if currentAt == nil {
		return true
	}
	if eventAt.After(*currentAt) {
		return true
	}
	if eventAt.Before(*currentAt) {
		return false
	}
	if currentID == nil || strings.TrimSpace(*currentID) == "" {
		return true
	}
	return eventID > *currentID
}

func kycInquiryRecord(row entity.KYCInquiry) usecase.KYCInquiryRecord {
	return usecase.KYCInquiryRecord{
		ID: row.ID, UserID: row.UserID, Provider: row.Provider, ProviderRequestKey: row.ProviderRequestKey,
		ProviderInquiryID: dereference(row.ProviderInquiryID), ProviderStatus: row.ProviderStatus, Status: usecase.KYCStatus(row.Status),
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(), ProviderEventAt: cloneTime(row.ProviderEventAt),
		LastProviderEventID: cloneString(row.LastProviderEventID), ApprovedAt: cloneTime(row.ApprovedAt), ExpiresAt: cloneTime(row.ExpiresAt),
	}
}

func retainInquiry(row entity.KYCInquiry, now time.Time) bool {
	if !isExpirableInquiry(row.Status) {
		return false
	}
	return row.ExpiresAt == nil || row.ExpiresAt.After(now)
}

func isExpirableInquiry(status entity.KYCInquiryStatus) bool {
	switch status {
	case entity.KYCInquiryCreating, entity.KYCInquiryCreated, entity.KYCInquiryPending, entity.KYCInquiryCompleted, entity.KYCInquiryPendingReview:
		return true
	default:
		return false
	}
}

func activeKYCEntityStatuses() []entity.KYCInquiryStatus {
	return []entity.KYCInquiryStatus{
		entity.KYCInquiryCreating, entity.KYCInquiryCreated, entity.KYCInquiryPending,
		entity.KYCInquiryCompleted, entity.KYCInquiryPendingReview,
	}
}

func validKYCEventStatus(status usecase.KYCStatus) bool {
	switch status {
	case usecase.KYCStatusCreated, usecase.KYCStatusPending, usecase.KYCStatusCompleted,
		usecase.KYCStatusPendingReview, usecase.KYCStatusApproved, usecase.KYCStatusDeclined,
		usecase.KYCStatusFailed, usecase.KYCStatusExpired:
		return true
	default:
		return false
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

var _ usecase.KYCRepository = (*KYCRepository)(nil)
