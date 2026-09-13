package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const kycProviderPersona = "persona"

type KYCStatus string

const (
	KYCStatusNotStarted    KYCStatus = "not_started"
	KYCStatusCreating      KYCStatus = "creating"
	KYCStatusCreated       KYCStatus = "created"
	KYCStatusPending       KYCStatus = "pending"
	KYCStatusCompleted     KYCStatus = "completed"
	KYCStatusPendingReview KYCStatus = "pending_review"
	KYCStatusApproved      KYCStatus = "approved"
	KYCStatusDeclined      KYCStatus = "declined"
	KYCStatusFailed        KYCStatus = "failed"
	KYCStatusExpired       KYCStatus = "expired"
)

var (
	ErrKYCRequired              = errors.New("approved identity verification is required")
	ErrKYCProviderUnavailable   = errors.New("kyc provider is unavailable")
	ErrKYCInvalidWebhook        = errors.New("invalid kyc webhook")
	ErrKYCInvalidRequest        = errors.New("invalid kyc request")
	ErrKYCInquiryNotFound       = errors.New("kyc inquiry not found")
	ErrKYCProviderEventConflict = errors.New("kyc provider event conflicts with an existing event")
)

type KYCProviderUnavailableError struct {
	Err error
}

func (e *KYCProviderUnavailableError) Error() string {
	return ErrKYCProviderUnavailable.Error()
}

func (e *KYCProviderUnavailableError) Unwrap() error { return e.Err }

func (e *KYCProviderUnavailableError) Is(target error) bool {
	return target == ErrKYCProviderUnavailable
}

type KYCInquiryRecord struct {
	ID, UserID, Provider, ProviderRequestKey string
	ProviderInquiryID, ProviderStatus        string
	Status                                   KYCStatus
	CreatedAt, UpdatedAt                     time.Time
	ProviderEventAt                          *time.Time
	LastProviderEventID                      *string
	ApprovedAt                               *time.Time
	ExpiresAt                                *time.Time
}

type KYCProviderEventRecord struct {
	ID, Provider, ProviderEventID, InquiryID, EventType, PayloadHash string
	ProviderEventAt, ReceivedAt                                      time.Time
}

type PersonaInquiry struct {
	ID        string
	Status    string
	ExpiresAt *time.Time
}

type PersonaEvent struct {
	ID, Type, InquiryID, Status string
	CreatedAt                   time.Time
}

type KYCStatusView struct {
	Provider       string
	Status         KYCStatus
	ProviderStatus string
	InquiryID      string
	CreatedAt      *time.Time
	UpdatedAt      *time.Time
	ExpiresAt      *time.Time
	ApprovedAt     *time.Time
}

type KYCInquiryView struct {
	Status         KYCStatus
	ProviderStatus string
	InquiryID      string
	EnvironmentID  string
	SessionToken   string
	ExpiresAt      *time.Time
}

type KYCRepository interface {
	FindCurrent(ctx context.Context, userID string) (KYCInquiryRecord, bool, error)
	FindByProviderInquiryID(ctx context.Context, provider, providerInquiryID string) (KYCInquiryRecord, bool, error)
	Reserve(ctx context.Context, userID, internalID, requestKey string, now time.Time) (KYCInquiryRecord, bool, error)
	AttachProviderInquiry(ctx context.Context, internalID, providerID, providerStatus string, expiresAt *time.Time, now time.Time) error
	RecordProviderEvent(ctx context.Context, event KYCProviderEventRecord, mappedStatus KYCStatus, providerStatus string, now time.Time) error
}

type PersonaGateway interface {
	CreateInquiry(ctx context.Context, referenceID, idempotencyKey string) (PersonaInquiry, error)
	ResumeInquiry(ctx context.Context, providerInquiryID string) (string, error)
	VerifyWebhook(rawBody []byte, signature string, now time.Time) (PersonaEvent, error)
}

type KYCStatusReader interface {
	IsApproved(ctx context.Context, userID string) (bool, error)
}

type KYCService interface {
	Status(ctx context.Context, userID string) (KYCStatusView, error)
	StartInquiry(ctx context.Context, userID string) (KYCInquiryView, error)
	ProcessPersonaWebhook(ctx context.Context, rawBody []byte, signature string, receivedAt time.Time) error
}

type KYCDependencies struct {
	Repository KYCRepository
	Persona    PersonaGateway
	Clock      Clock
	NewID      func() (string, error)
}

type KYCServiceConfig struct {
	EnvironmentID string
}

type KYCUsecase struct {
	dependencies KYCDependencies
	config       KYCServiceConfig
}

func NewKYCUsecase(dependencies KYCDependencies, config KYCServiceConfig) (*KYCUsecase, error) {
	if dependencies.Repository == nil || dependencies.Persona == nil || dependencies.Clock == nil || dependencies.NewID == nil ||
		strings.TrimSpace(config.EnvironmentID) == "" {
		return nil, errors.New("valid kyc dependencies and configuration are required")
	}
	return &KYCUsecase{dependencies: dependencies, config: config}, nil
}

func (s *KYCUsecase) Status(ctx context.Context, userID string) (KYCStatusView, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return KYCStatusView{}, ErrKYCInvalidRequest
	}
	record, found, err := s.dependencies.Repository.FindCurrent(ctx, userID)
	if err != nil {
		return KYCStatusView{}, fmt.Errorf("reading kyc status: %w", err)
	}
	if !found {
		return KYCStatusView{Provider: kycProviderPersona, Status: KYCStatusNotStarted}, nil
	}
	return s.statusView(record, s.now()), nil
}

func (s *KYCUsecase) IsApproved(ctx context.Context, userID string) (bool, error) {
	status, err := s.Status(ctx, userID)
	if err != nil {
		return false, err
	}
	return status.Status == KYCStatusApproved, nil
}

func (s *KYCUsecase) StartInquiry(ctx context.Context, userID string) (KYCInquiryView, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return KYCInquiryView{}, ErrKYCInvalidRequest
	}
	now := s.now()
	internalID, err := s.dependencies.NewID()
	if err != nil {
		return KYCInquiryView{}, fmt.Errorf("generating kyc inquiry id: %w", err)
	}
	requestKey := "kyc_" + internalID
	record, _, err := s.dependencies.Repository.Reserve(ctx, userID, internalID, requestKey, now)
	if err != nil {
		return KYCInquiryView{}, fmt.Errorf("reserving kyc inquiry: %w", err)
	}

	status := s.statusView(record, now)
	if status.Status == KYCStatusApproved {
		return s.inquiryView(record, ""), nil
	}
	if strings.TrimSpace(record.ProviderInquiryID) == "" {
		inquiry, err := s.dependencies.Persona.CreateInquiry(ctx, userID, record.ProviderRequestKey)
		if err != nil {
			return KYCInquiryView{}, &KYCProviderUnavailableError{Err: fmt.Errorf("creating persona inquiry: %w", err)}
		}
		if strings.TrimSpace(inquiry.ID) == "" {
			return KYCInquiryView{}, &KYCProviderUnavailableError{Err: errors.New("persona returned an empty inquiry id")}
		}
		if err := s.dependencies.Repository.AttachProviderInquiry(ctx, record.ID, inquiry.ID, inquiry.Status, inquiry.ExpiresAt, now); err != nil {
			return KYCInquiryView{}, fmt.Errorf("attaching persona inquiry: %w", err)
		}
		record.ProviderInquiryID = inquiry.ID
		record.ProviderStatus = inquiry.Status
		record.ExpiresAt = inquiry.ExpiresAt
		record.Status = mapPersonaStatus(inquiry.Status)
		return s.inquiryView(record, ""), nil
	}

	sessionToken, err := s.dependencies.Persona.ResumeInquiry(ctx, record.ProviderInquiryID)
	if err != nil {
		return KYCInquiryView{}, &KYCProviderUnavailableError{Err: fmt.Errorf("resuming persona inquiry: %w", err)}
	}
	return s.inquiryView(record, sessionToken), nil
}

func (s *KYCUsecase) ProcessPersonaWebhook(ctx context.Context, rawBody []byte, signature string, receivedAt time.Time) error {
	if len(rawBody) == 0 || strings.TrimSpace(signature) == "" {
		return ErrKYCInvalidWebhook
	}
	if receivedAt.IsZero() {
		receivedAt = s.now()
	}
	event, err := s.dependencies.Persona.VerifyWebhook(rawBody, signature, receivedAt.UTC())
	if err != nil {
		return fmt.Errorf("%w: persona webhook verification failed", ErrKYCInvalidWebhook)
	}
	if strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.InquiryID) == "" || event.CreatedAt.IsZero() {
		return ErrKYCInvalidWebhook
	}
	mappedStatus, ok := mapPersonaStatusForEvent(event)
	if !ok {
		return nil
	}
	record, found, err := s.dependencies.Repository.FindByProviderInquiryID(ctx, kycProviderPersona, event.InquiryID)
	if err != nil {
		return fmt.Errorf("finding kyc inquiry for persona event: %w", err)
	}
	if !found {
		return ErrKYCInquiryNotFound
	}
	eventID, err := s.dependencies.NewID()
	if err != nil {
		return fmt.Errorf("generating kyc provider event id: %w", err)
	}
	hash := sha256.Sum256(rawBody)
	providerStatus := strings.TrimSpace(event.Status)
	if providerStatus == "" {
		providerStatus = strings.TrimPrefix(strings.ToLower(event.Type), "inquiry.")
	}
	providerEvent := KYCProviderEventRecord{
		ID: eventID, Provider: kycProviderPersona, ProviderEventID: event.ID, InquiryID: record.ID,
		EventType: event.Type, ProviderEventAt: event.CreatedAt.UTC(), PayloadHash: hex.EncodeToString(hash[:]), ReceivedAt: receivedAt.UTC(),
	}
	if err := s.dependencies.Repository.RecordProviderEvent(ctx, providerEvent, mappedStatus, providerStatus, receivedAt.UTC()); err != nil {
		return fmt.Errorf("recording persona webhook: %w", err)
	}
	return nil
}

func (s *KYCUsecase) now() time.Time {
	return s.dependencies.Clock.Now().UTC()
}

func (s *KYCUsecase) statusView(record KYCInquiryRecord, now time.Time) KYCStatusView {
	status := record.Status
	if isActiveKYCStatus(status) && record.ExpiresAt != nil && !record.ExpiresAt.After(now) {
		status = KYCStatusExpired
	}
	return KYCStatusView{
		Provider: kycProviderPersona, Status: status, ProviderStatus: record.ProviderStatus, InquiryID: record.ProviderInquiryID,
		CreatedAt: timePointer(record.CreatedAt), UpdatedAt: timePointer(record.UpdatedAt), ExpiresAt: record.ExpiresAt, ApprovedAt: record.ApprovedAt,
	}
}

func (s *KYCUsecase) inquiryView(record KYCInquiryRecord, sessionToken string) KYCInquiryView {
	return KYCInquiryView{
		Status: record.Status, ProviderStatus: record.ProviderStatus, InquiryID: record.ProviderInquiryID,
		EnvironmentID: s.config.EnvironmentID, SessionToken: sessionToken, ExpiresAt: record.ExpiresAt,
	}
}

func mapPersonaStatusForEvent(event PersonaEvent) (KYCStatus, bool) {
	eventType := normalizePersonaValue(event.Type)
	switch eventType {
	case "inquiry.created":
		return KYCStatusCreated, true
	case "inquiry.started":
		return KYCStatusPending, true
	case "inquiry.completed":
		return KYCStatusCompleted, true
	case "inquiry.marked_for_review":
		return KYCStatusPendingReview, true
	case "inquiry.approved":
		return KYCStatusApproved, true
	case "inquiry.declined":
		return KYCStatusDeclined, true
	case "inquiry.failed":
		return KYCStatusFailed, true
	case "inquiry.expired":
		return KYCStatusExpired, true
	case "inquiry.transitioned":
		return mapPersonaStatusForTransition(event.Status)
	default:
		return KYCStatusNotStarted, false
	}
}

func mapPersonaStatus(providerStatus string) KYCStatus {
	status, ok := mapPersonaStatusForTransition(providerStatus)
	if !ok {
		return KYCStatusCreated
	}
	return status
}

func mapPersonaStatusForTransition(providerStatus string) (KYCStatus, bool) {
	switch normalizePersonaValue(providerStatus) {
	case "created":
		return KYCStatusCreated, true
	case "pending", "started":
		return KYCStatusPending, true
	case "completed":
		return KYCStatusCompleted, true
	case "marked_for_review", "review":
		return KYCStatusPendingReview, true
	case "approved":
		return KYCStatusApproved, true
	case "declined":
		return KYCStatusDeclined, true
	case "failed":
		return KYCStatusFailed, true
	case "expired":
		return KYCStatusExpired, true
	default:
		return KYCStatusNotStarted, false
	}
}

func normalizePersonaValue(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
}

func isActiveKYCStatus(status KYCStatus) bool {
	switch status {
	case KYCStatusCreating, KYCStatusCreated, KYCStatusPending, KYCStatusCompleted, KYCStatusPendingReview:
		return true
	default:
		return false
	}
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}
