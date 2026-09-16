package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeEmailOutboxRepository struct {
	job         EmailOutboxJob
	leaseErr    error
	completeID  string
	completeAt  time.Time
	retryID     string
	retryAt     time.Time
	retryError  string
	failID      string
	failAt      time.Time
	failError   string
	completeErr error
	retryErr    error
	failErr     error
}

func (r *fakeEmailOutboxRepository) LeaseEmail(context.Context, string, time.Time, time.Duration) (EmailOutboxJob, error) {
	if r.leaseErr != nil {
		return EmailOutboxJob{}, r.leaseErr
	}
	if r.job.ID == "" {
		return EmailOutboxJob{}, ErrNoJob
	}
	return r.job, nil
}

func (r *fakeEmailOutboxRepository) CompleteEmail(_ context.Context, id string, now time.Time) error {
	r.completeID, r.completeAt = id, now
	return r.completeErr
}

func (r *fakeEmailOutboxRepository) RetryEmail(_ context.Context, id string, availableAt time.Time, safeError string) error {
	r.retryID, r.retryAt, r.retryError = id, availableAt, safeError
	return r.retryErr
}

func (r *fakeEmailOutboxRepository) FailEmail(_ context.Context, id string, now time.Time, safeError string) error {
	r.failID, r.failAt, r.failError = id, now, safeError
	return r.failErr
}

type fakeEmailSender struct {
	verificationEmail string
	verificationLink  string
	resetEmail        string
	resetLink         string
	err               error
}

func (s *fakeEmailSender) SendEmailVerification(_ context.Context, email, link string) error {
	s.verificationEmail, s.verificationLink = email, link
	return s.err
}

func (s *fakeEmailSender) SendPasswordReset(_ context.Context, email, link string) error {
	s.resetEmail, s.resetLink = email, link
	return s.err
}

func newEmailDeliveryTestService(t *testing.T, repository *fakeEmailOutboxRepository, sender *fakeEmailSender, attempts int) (*EmailDeliveryUsecase, time.Time) {
	t.Helper()
	now := time.Date(2026, time.September, 16, 14, 30, 0, 0, time.UTC)
	service, err := NewEmailDeliveryUsecase(repository, sender, EmailDeliveryConfig{
		EncryptionKey: []byte("01234567890123456789012345678901"),
		LeaseDuration: time.Minute,
		RetryDelay:    10 * time.Second,
		MaxAttempts:   attempts,
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewEmailDeliveryUsecase() error = %v", err)
	}
	return service, now
}

func TestEmailDeliveryUsecaseSendsVerificationAndCompletes(t *testing.T) {
	repository := &fakeEmailOutboxRepository{job: EmailOutboxJob{ID: "job-id", Topic: EmailVerificationTopic, Attempts: 1}}
	sender := &fakeEmailSender{}
	service, now := newEmailDeliveryTestService(t, repository, sender, 3)
	repository.job.Payload, _ = encryptEmailDeliveryPayload(service.config.EncryptionKey, emailDeliveryPayload{
		Recipient: "user@example.com",
		Link:      "https://app.example.com/auth/verify-email?token=secret",
	})

	processed, err := service.RunOnce(context.Background(), "worker-id")
	if err != nil || !processed {
		t.Fatalf("RunOnce() = %v, %v; want processed", processed, err)
	}
	if sender.verificationEmail != "user@example.com" || sender.verificationLink == "" {
		t.Fatalf("verification delivery = %q, %q", sender.verificationEmail, sender.verificationLink)
	}
	if repository.completeID != "job-id" || !repository.completeAt.Equal(now) {
		t.Fatalf("completion = %q at %v", repository.completeID, repository.completeAt)
	}
}

func TestEmailDeliveryUsecaseRetriesFailedDelivery(t *testing.T) {
	repository := &fakeEmailOutboxRepository{job: EmailOutboxJob{ID: "job-id", Topic: PasswordResetTopic, Attempts: 1}}
	sender := &fakeEmailSender{err: errors.New("smtp unavailable")}
	service, now := newEmailDeliveryTestService(t, repository, sender, 3)
	repository.job.Payload, _ = encryptEmailDeliveryPayload(service.config.EncryptionKey, emailDeliveryPayload{
		Recipient: "user@example.com",
		Link:      "https://app.example.com/auth/reset-password?token=secret",
	})

	processed, err := service.RunOnce(context.Background(), "worker-id")
	if !processed || !errors.Is(err, sender.err) {
		t.Fatalf("RunOnce() = %v, %v; want sender error", processed, err)
	}
	if repository.retryID != "job-id" || !repository.retryAt.Equal(now.Add(10*time.Second)) || repository.retryError == "" {
		t.Fatalf("retry = %q at %v error %q", repository.retryID, repository.retryAt, repository.retryError)
	}
	if repository.completeID != "" || repository.failID != "" {
		t.Fatalf("failed delivery was incorrectly completed/failed: %+v", repository)
	}
}

func TestEmailDeliveryUsecaseFailsAfterMaximumAttempts(t *testing.T) {
	repository := &fakeEmailOutboxRepository{job: EmailOutboxJob{ID: "job-id", Topic: EmailVerificationTopic, Attempts: 3}}
	sender := &fakeEmailSender{err: errors.New("smtp unavailable")}
	service, now := newEmailDeliveryTestService(t, repository, sender, 3)
	repository.job.Payload, _ = encryptEmailDeliveryPayload(service.config.EncryptionKey, emailDeliveryPayload{
		Recipient: "user@example.com",
		Link:      "https://app.example.com/auth/verify-email?token=secret",
	})

	processed, err := service.RunOnce(context.Background(), "worker-id")
	if !processed || !errors.Is(err, sender.err) {
		t.Fatalf("RunOnce() = %v, %v; want sender error", processed, err)
	}
	if repository.failID != "job-id" || !repository.failAt.Equal(now) || repository.failError == "" {
		t.Fatalf("failure = %q at %v error %q", repository.failID, repository.failAt, repository.failError)
	}
	if repository.retryID != "" {
		t.Fatalf("maximum-attempt job was retried: %q", repository.retryID)
	}
}

func TestEmailDeliveryUsecaseRejectsInvalidPayload(t *testing.T) {
	repository := &fakeEmailOutboxRepository{job: EmailOutboxJob{ID: "job-id", Topic: EmailVerificationTopic, Payload: []byte(`{"ciphertext":"invalid"}`), Attempts: 1}}
	sender := &fakeEmailSender{}
	service, now := newEmailDeliveryTestService(t, repository, sender, 3)

	processed, err := service.RunOnce(context.Background(), "worker-id")
	if !processed || err == nil {
		t.Fatalf("RunOnce() = %v, %v; want invalid-payload error", processed, err)
	}
	if repository.failID != "job-id" || !repository.failAt.Equal(now) || sender.verificationEmail != "" {
		t.Fatalf("invalid payload handling = %+v sender=%+v", repository, sender)
	}
}
