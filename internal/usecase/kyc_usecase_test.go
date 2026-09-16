package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

type fakeKYCRepository struct {
	current          KYCInquiryRecord
	currentFound     bool
	reserved         KYCInquiryRecord
	reserveCreated   bool
	reserveCalls     int
	failed           bool
	attached         bool
	providerRecord   KYCInquiryRecord
	providerFound    bool
	recordedEvent    KYCProviderEventRecord
	recordedStatus   KYCStatus
	recordedProvider string
}

type fakeKYCStatusReader struct {
	approved bool
	err      error
	calls    int
}

func (r *fakeKYCStatusReader) IsApproved(context.Context, string) (bool, error) {
	r.calls++
	return r.approved, r.err
}

func (r *fakeKYCRepository) FindCurrent(context.Context, string) (KYCInquiryRecord, bool, error) {
	return r.current, r.currentFound, nil
}

func (r *fakeKYCRepository) FindByProviderInquiryID(context.Context, string, string) (KYCInquiryRecord, bool, error) {
	return r.providerRecord, r.providerFound, nil
}

func (r *fakeKYCRepository) Reserve(_ context.Context, userID, internalID, requestKey string, now time.Time) (KYCInquiryRecord, bool, error) {
	r.reserveCalls++
	if r.reserved.ID != "" {
		return r.reserved, false, nil
	}
	r.reserved = KYCInquiryRecord{
		ID: internalID, UserID: userID, Provider: "persona", ProviderRequestKey: requestKey,
		Status: KYCStatusCreating, CreatedAt: now, UpdatedAt: now,
	}
	r.current = r.reserved
	r.currentFound = true
	r.reserveCreated = true
	return r.reserved, true, nil
}

func (r *fakeKYCRepository) AttachProviderInquiry(_ context.Context, internalID, providerID, providerStatus string, expiresAt *time.Time, now time.Time) error {
	r.attached = true
	r.reserved.ID = internalID
	r.reserved.ProviderInquiryID = providerID
	r.reserved.ProviderStatus = providerStatus
	r.reserved.Status = KYCStatusCreated
	r.reserved.ExpiresAt = expiresAt
	r.reserved.UpdatedAt = now
	r.current = r.reserved
	return nil
}

func (r *fakeKYCRepository) MarkFailed(_ context.Context, internalID string, now time.Time) error {
	r.failed = true
	r.reserved.ID = internalID
	r.reserved.Status = KYCStatusFailed
	r.reserved.UpdatedAt = now
	r.current = r.reserved
	return nil
}

func (r *fakeKYCRepository) RecordProviderEvent(_ context.Context, event KYCProviderEventRecord, mappedStatus KYCStatus, providerStatus string, now time.Time) error {
	r.recordedEvent = event
	r.recordedStatus = mappedStatus
	r.recordedProvider = providerStatus
	r.providerRecord.Status = mappedStatus
	r.providerRecord.ProviderStatus = providerStatus
	r.providerRecord.ProviderEventAt = &event.ProviderEventAt
	r.providerRecord.UpdatedAt = now
	return nil
}

type fakePersonaGateway struct {
	createdReference string
	createdKey       string
	createdCalls     int
	createResult     PersonaInquiry
	createErr        error
	resumeID         string
	resumeToken      string
	resumeCalls      int
	resumeErr        error
	webhookEvent     PersonaEvent
	webhookErr       error
}

type permanentKYCProviderError struct {
	err error
}

func (e permanentKYCProviderError) Error() string { return e.err.Error() }

func (e permanentKYCProviderError) Unwrap() error { return e.err }

func (e permanentKYCProviderError) Retryable() bool { return false }

func (p *fakePersonaGateway) CreateInquiry(_ context.Context, referenceID, idempotencyKey string) (PersonaInquiry, error) {
	p.createdCalls++
	p.createdReference = referenceID
	p.createdKey = idempotencyKey
	return p.createResult, p.createErr
}

func (p *fakePersonaGateway) ResumeInquiry(_ context.Context, inquiryID string) (string, error) {
	p.resumeCalls++
	p.resumeID = inquiryID
	return p.resumeToken, p.resumeErr
}

func (p *fakePersonaGateway) VerifyWebhook([]byte, string, time.Time) (PersonaEvent, error) {
	return p.webhookEvent, p.webhookErr
}

func newTestKYCUsecase(t *testing.T, repository KYCRepository, persona PersonaGateway, now time.Time) *KYCUsecase {
	t.Helper()
	service, err := NewKYCUsecase(KYCDependencies{
		Repository: repository,
		Persona:    persona,
		Clock:      fixedClock{now: now},
		NewID: func() (string, error) {
			return "kyc-internal-1", nil
		},
	}, KYCServiceConfig{EnvironmentID: "env_test", HostedFlowURL: "https://inquiry.withpersona.com/verify"})
	if err != nil {
		t.Fatalf("NewKYCUsecase() error = %v", err)
	}
	return service
}

func TestKYCStatusReaderApprovesOnlyApprovedInquiry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	statuses := []KYCStatus{
		KYCStatusCreating, KYCStatusCreated, KYCStatusPending, KYCStatusCompleted,
		KYCStatusPendingReview, KYCStatusDeclined, KYCStatusFailed, KYCStatusExpired,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			repository := &fakeKYCRepository{
				current:      KYCInquiryRecord{ID: "internal-1", UserID: "user-1", Status: status},
				currentFound: true,
			}
			service := newTestKYCUsecase(t, repository, &fakePersonaGateway{}, now)
			approved, err := service.IsApproved(context.Background(), "user-1")
			if err != nil {
				t.Fatalf("IsApproved() error = %v", err)
			}
			if approved {
				t.Fatal("IsApproved() = true for non-approved status")
			}
		})
	}

	repository := &fakeKYCRepository{}
	service := newTestKYCUsecase(t, repository, &fakePersonaGateway{}, now)
	approved, err := service.IsApproved(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("IsApproved() with no record error = %v", err)
	}
	if approved {
		t.Fatal("IsApproved() = true without an inquiry")
	}

	repository.current = KYCInquiryRecord{ID: "internal-approved", UserID: "user-1", Status: KYCStatusApproved}
	repository.currentFound = true
	approved, err = service.IsApproved(context.Background(), "user-1")
	if err != nil || !approved {
		t.Fatalf("IsApproved() for approved status = %v, %v", approved, err)
	}
}

func TestKYCStatusMapsAnExpiredActiveInquiryToExpired(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	repository := &fakeKYCRepository{
		current: KYCInquiryRecord{
			ID: "internal-1", UserID: "user-1", Provider: "persona", Status: KYCStatusPending,
			ProviderInquiryID: "inq-1", ProviderStatus: "pending", ExpiresAt: &expiredAt,
		},
		currentFound: true,
	}
	service := newTestKYCUsecase(t, repository, &fakePersonaGateway{}, now)
	view, err := service.Status(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if view.Status != KYCStatusExpired {
		t.Fatalf("Status() = %q, want %q", view.Status, KYCStatusExpired)
	}
}

func TestKYCStartCreatesInquiryWithoutResumingNewInquiry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	persona := &fakePersonaGateway{createResult: PersonaInquiry{ID: "inq-1", Status: "created", ExpiresAt: &expiresAt}}
	repository := &fakeKYCRepository{}
	service := newTestKYCUsecase(t, repository, persona, now)

	view, err := service.StartInquiry(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("StartInquiry() error = %v", err)
	}
	if persona.createdCalls != 1 || persona.createdReference != "user-1" {
		t.Fatalf("CreateInquiry() calls/reference = %d/%q", persona.createdCalls, persona.createdReference)
	}
	if persona.createdKey == "" || view.InquiryID != "inq-1" || view.URL != "https://inquiry.withpersona.com/verify?inquiry-id=inq-1" || view.SessionToken != "" {
		t.Fatalf("StartInquiry() view = %+v, idempotency key = %q", view, persona.createdKey)
	}
	if !repository.reserveCreated || !repository.attached || view.Status != KYCStatusCreated {
		t.Fatalf("reservation/attachment/status = %v/%v/%q", repository.reserveCreated, repository.attached, view.Status)
	}
}

func TestKYCStartReusesPendingInquiryAndResumesIt(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	repository := &fakeKYCRepository{
		reserved: KYCInquiryRecord{
			ID: "internal-1", UserID: "user-1", Provider: "persona",
			ProviderInquiryID: "inq-pending", ProviderStatus: "pending", Status: KYCStatusPending,
			CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute),
		},
	}
	persona := &fakePersonaGateway{resumeToken: "session-token"}
	service := newTestKYCUsecase(t, repository, persona, now)

	view, err := service.StartInquiry(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("StartInquiry() error = %v", err)
	}
	if repository.reserveCalls != 1 || persona.createdCalls != 0 || persona.resumeCalls != 1 || persona.resumeID != "inq-pending" {
		t.Fatalf("reserve/create/resume = %d/%d/%d/%q", repository.reserveCalls, persona.createdCalls, persona.resumeCalls, persona.resumeID)
	}
	if view.SessionToken != "session-token" || view.Status != KYCStatusPending || view.URL != "https://inquiry.withpersona.com/verify?inquiry-id=inq-pending" {
		t.Fatalf("StartInquiry() view = %+v", view)
	}
}

func TestKYCStartDoesNotCallPersonaForApprovedInquiry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	repository := &fakeKYCRepository{
		reserved: KYCInquiryRecord{ID: "internal-1", UserID: "user-1", Status: KYCStatusApproved, ProviderInquiryID: "inq-approved"},
	}
	persona := &fakePersonaGateway{}
	service := newTestKYCUsecase(t, repository, persona, now)

	view, err := service.StartInquiry(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("StartInquiry() error = %v", err)
	}
	if persona.createdCalls != 0 || persona.resumeCalls != 0 || view.Status != KYCStatusApproved {
		t.Fatalf("provider calls/status = %d/%d/%q", persona.createdCalls, persona.resumeCalls, view.Status)
	}
}

func TestKYCStartClassifiesPersonaFailureAndKeepsInquiryNonEligible(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	repository := &fakeKYCRepository{}
	persona := &fakePersonaGateway{createErr: errors.New("persona timeout")}
	service := newTestKYCUsecase(t, repository, persona, now)

	_, err := service.StartInquiry(context.Background(), "user-1")
	if !errors.Is(err, ErrKYCProviderUnavailable) {
		t.Fatalf("StartInquiry() error = %v, want %v", err, ErrKYCProviderUnavailable)
	}
	if repository.reserved.Status == KYCStatusApproved || repository.attached {
		t.Fatalf("failed inquiry became eligible or attached: %+v/%v", repository.reserved, repository.attached)
	}
}

func TestKYCStartMarksPermanentProviderFailureForFreshRetry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	repository := &fakeKYCRepository{}
	persona := &fakePersonaGateway{createErr: permanentKYCProviderError{err: errors.New("persona request returned status 400: Idempotency error")}}
	service := newTestKYCUsecase(t, repository, persona, now)

	_, err := service.StartInquiry(context.Background(), "user-1")
	if !errors.Is(err, ErrKYCProviderUnavailable) {
		t.Fatalf("StartInquiry() error = %v, want %v", err, ErrKYCProviderUnavailable)
	}
	if !repository.failed || repository.reserved.Status != KYCStatusFailed {
		t.Fatalf("permanent provider failure was not marked failed: failed=%v record=%+v", repository.failed, repository.reserved)
	}
}

func TestKYCWebhookMapsApprovedEventAndRecordsItsIdentity(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	eventAt := now.Add(-time.Second)
	repository := &fakeKYCRepository{
		providerRecord: KYCInquiryRecord{
			ID: "internal-1", UserID: "user-1", Provider: "persona", ProviderInquiryID: "inq-1", Status: KYCStatusPending,
		},
		providerFound: true,
	}
	persona := &fakePersonaGateway{webhookEvent: PersonaEvent{
		ID: "evt-1", Type: "inquiry.approved", InquiryID: "inq-1", Status: "approved", CreatedAt: eventAt,
	}}
	service := newTestKYCUsecase(t, repository, persona, now)

	if err := service.ProcessPersonaWebhook(context.Background(), []byte(`{"data":{}}`), "valid", now); err != nil {
		t.Fatalf("ProcessPersonaWebhook() error = %v", err)
	}
	if repository.recordedEvent.ProviderEventID != "evt-1" || repository.recordedEvent.InquiryID != "internal-1" {
		t.Fatalf("recorded event = %+v", repository.recordedEvent)
	}
	if repository.recordedStatus != KYCStatusApproved || repository.recordedProvider != "approved" {
		t.Fatalf("recorded status/provider = %q/%q", repository.recordedStatus, repository.recordedProvider)
	}
}

func TestKYCWebhookKeepsCompletedInquiryNonEligible(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	eventAt := now.Add(-time.Second)
	repository := &fakeKYCRepository{
		providerRecord: KYCInquiryRecord{
			ID: "internal-1", UserID: "user-1", Provider: "persona", ProviderInquiryID: "inq-1", Status: KYCStatusPending,
		},
		providerFound: true,
	}
	persona := &fakePersonaGateway{webhookEvent: PersonaEvent{
		ID: "evt-completed", Type: "inquiry.completed", InquiryID: "inq-1", Status: "completed", CreatedAt: eventAt,
	}}
	service := newTestKYCUsecase(t, repository, persona, now)

	if err := service.ProcessPersonaWebhook(context.Background(), []byte(`{"data":{}}`), "valid", now); err != nil {
		t.Fatalf("ProcessPersonaWebhook() error = %v", err)
	}
	if repository.recordedStatus != KYCStatusCompleted {
		t.Fatalf("recorded status = %q, want %q", repository.recordedStatus, KYCStatusCompleted)
	}
	approved, err := service.IsApproved(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("IsApproved() error = %v", err)
	}
	if approved {
		t.Fatal("completed inquiry must not be approved")
	}
}

func TestKYCWebhookReturnsUnknownInquiryForProviderRetry(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	persona := &fakePersonaGateway{webhookEvent: PersonaEvent{
		ID: "evt-unknown", Type: "inquiry.approved", InquiryID: "inq-not-attached", Status: "approved", CreatedAt: now,
	}}
	service := newTestKYCUsecase(t, &fakeKYCRepository{providerFound: false}, persona, now)

	err := service.ProcessPersonaWebhook(context.Background(), []byte(`{"data":{}}`), "valid", now)
	if !errors.Is(err, ErrKYCInquiryNotFound) {
		t.Fatalf("ProcessPersonaWebhook() error = %v, want %v", err, ErrKYCInquiryNotFound)
	}
}

func TestKYCWebhookPreservesSafeValidationReason(t *testing.T) {
	now := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	persona := &fakePersonaGateway{webhookErr: errors.New("persona webhook signature does not match configured secret")}
	service := newTestKYCUsecase(t, &fakeKYCRepository{}, persona, now)

	err := service.ProcessPersonaWebhook(context.Background(), []byte(`{"data":{}}`), "invalid", now)
	if !errors.Is(err, ErrKYCInvalidWebhook) {
		t.Fatalf("ProcessPersonaWebhook() error = %v, want %v", err, ErrKYCInvalidWebhook)
	}
	if !strings.Contains(err.Error(), "signature does not match configured secret") {
		t.Fatalf("ProcessPersonaWebhook() error = %v, want the safe validation reason", err)
	}
}

func TestAPIKeyCreationRequiresApprovedKYC(t *testing.T) {
	store := &fakeStore{developerEnabled: true}
	service := newTestServiceWithKYC(t, store, &fakeKYCStatusReader{})

	_, err := service.Create(context.Background(), "user-1", "Default")
	if !errors.Is(err, ErrKYCRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrKYCRequired)
	}
}

func TestOnrampCreationRequiresApprovedKYC(t *testing.T) {
	now := time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC)
	store := &createStoreFake{}
	service, err := NewOnrampUsecase(OnrampDependencies{
		Repository: store, Prices: priceFake{now: now}, Treasury: treasuryFake{}, Gateway: &gatewayFake{}, Destinations: destinationFake{},
		KYC: &fakeKYCStatusReader{},
	}, ServiceConfig{
		QuotePolicy: QuotePolicy{TTL: 5 * time.Minute, MaxAge: 2 * time.Minute}, MinIDR: 10_000, MaxIDR: 10_000_000,
		TreasuryAccount: "G" + string(make([]byte, 55)), NewID: func() (string, error) { return "order-kyc", nil }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewOnrampUsecase() error = %v", err)
	}
	_, _, err = service.Create(context.Background(), Command{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-kyc", Amount: 100_000,
		Destination: "G" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	})
	if !errors.Is(err, ErrKYCRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrKYCRequired)
	}
}

func TestOfframpCreationRequiresApprovedKYC(t *testing.T) {
	repository := &fakeOfframpRepo{}
	service := testOfframpServiceWithKYC(t, repository, &fakeKYCStatusReader{})
	_, _, err := service.Create(context.Background(), OfframpCommand{
		Principal: apiOrderPrincipal("client-1", "owner-1"), IdempotencyKey: "idem-kyc", AssetNetwork: StellarTestnetNetwork,
		AssetCode: NativeXLMAssetCode, FiatCurrency: IDRCurrency, AssetAmount: "40",
		WithdrawalMethod: entity.WithdrawalMethodSandboxTransfer, DestinationToken: "demo-token",
	})
	if !errors.Is(err, ErrKYCRequired) {
		t.Fatalf("Create() error = %v, want %v", err, ErrKYCRequired)
	}
}
