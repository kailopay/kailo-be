package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
)

var ErrNoJob = errors.New("no settlement job available")

type SubmissionResult string

const (
	SubmissionUnknown   SubmissionResult = "unknown"
	SubmissionConfirmed SubmissionResult = "confirmed"
	SubmissionPending   SubmissionResult = "pending"
	SubmissionPermanent SubmissionResult = "permanent_failure"
	SubmissionRetryable SubmissionResult = "retryable"
)

type Job struct {
	OutboxID string
	IntentID string
}

type Transfer struct {
	OrderID     string
	Source      string
	Destination string
	Amount      entity.Stroops
	Memo        string
}

type Intent struct {
	IntentID        string
	TransactionHash string
	Transfer        Transfer
}

type BuiltTransaction struct {
	Hash     string
	Envelope string
}

type Submission struct {
	Result    SubmissionResult
	LedgerAt  time.Time
	SafeError string
}

type SettlementRepository interface {
	Lease(ctx context.Context, workerID string, now time.Time, duration time.Duration) (Job, error)
	LoadIntent(ctx context.Context, intentID string) (Intent, error)
	MarkSubmitted(ctx context.Context, intentID, hash string, now time.Time) error
	Confirm(ctx context.Context, intentID, hash string, ledgerAt time.Time) error
	MarkUnknown(ctx context.Context, intentID, safeError string) error
	FailPermanent(ctx context.Context, intentID, safeError string) error
	RetryLater(ctx context.Context, intentID string, availableAt time.Time, safeError string) error
	ResetSubmitted(ctx context.Context, intentID, safeError string) error
	ReleaseExpiredReservations(ctx context.Context, now time.Time, limit int) (int, error)
}

type Network interface {
	Build(ctx context.Context, transfer Transfer) (BuiltTransaction, error)
	Submit(ctx context.Context, transaction BuiltTransaction) (Submission, error)
	FindByHash(ctx context.Context, hash string) (Submission, error)
	SpendableBalance(ctx context.Context, account string) (entity.Stroops, error)
}

type SettlementConfig struct {
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	Now           func() time.Time
}

type SettlementUsecase struct {
	repository SettlementRepository
	network    Network
	config     SettlementConfig
}

func NewSettlementUsecase(repository SettlementRepository, network Network, config SettlementConfig) (*SettlementUsecase, error) {
	if repository == nil || network == nil || config.LeaseDuration <= 0 || config.RetryDelay <= 0 || config.Now == nil {
		return nil, errors.New("valid settlement dependencies and configuration are required")
	}
	return &SettlementUsecase{repository: repository, network: network, config: config}, nil
}

const expiredReservationBatchLimit = 100

// ReleaseExpired releases treasury inventory held by reservations whose expiry
// has passed and returns the number of released reservations.
func (s *SettlementUsecase) ReleaseExpired(ctx context.Context) (int, error) {
	return s.repository.ReleaseExpiredReservations(ctx, s.config.Now().UTC(), expiredReservationBatchLimit)
}

func (s *SettlementUsecase) RunOnce(ctx context.Context, workerID string) (bool, error) {
	now := s.config.Now().UTC()
	job, err := s.repository.Lease(ctx, workerID, now, s.config.LeaseDuration)
	if errors.Is(err, ErrNoJob) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("leasing settlement: %w", err)
	}
	intent, err := s.repository.LoadIntent(ctx, job.IntentID)
	if err != nil {
		return true, fmt.Errorf("loading settlement intent: %w", err)
	}
	if intent.TransactionHash != "" {
		return true, s.reconcile(ctx, intent.IntentID, intent.TransactionHash)
	}
	built, err := s.network.Build(ctx, intent.Transfer)
	if err != nil {
		_ = s.repository.RetryLater(ctx, intent.IntentID, now.Add(s.config.RetryDelay), "building transaction failed")
		return true, fmt.Errorf("building Stellar transaction: %w", err)
	}
	if built.Hash == "" || built.Envelope == "" {
		return true, errors.New("Stellar adapter returned incomplete transaction")
	}
	if err := s.repository.MarkSubmitted(ctx, intent.IntentID, built.Hash, now); err != nil {
		return true, fmt.Errorf("persisting Stellar transaction hash: %w", err)
	}
	submission, err := s.network.Submit(ctx, built)
	if err != nil {
		_ = s.repository.MarkUnknown(ctx, intent.IntentID, "submission outcome unknown")
		return true, s.reconcile(ctx, intent.IntentID, built.Hash)
	}
	if submission.Result == SubmissionRetryable {
		// Horizon rejected the transaction before it could apply, so the
		// persisted hash can never confirm; clear it and rebuild later.
		if err := s.repository.ResetSubmitted(ctx, intent.IntentID, submission.SafeError); err != nil {
			return true, fmt.Errorf("resetting rejected submission: %w", err)
		}
		return true, s.repository.RetryLater(ctx, intent.IntentID, now.Add(s.config.RetryDelay), submission.SafeError)
	}
	return true, s.handleSubmission(ctx, intent.IntentID, built.Hash, submission)
}

func (s *SettlementUsecase) reconcile(ctx context.Context, intentID, hash string) error {
	result, err := s.network.FindByHash(ctx, hash)
	if err != nil {
		return s.repository.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), "transaction reconciliation unavailable")
	}
	if result.Result == SubmissionUnknown || result.Result == SubmissionPending || result.Result == SubmissionRetryable {
		return s.repository.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), result.SafeError)
	}
	return s.handleSubmission(ctx, intentID, hash, result)
}

func (s *SettlementUsecase) handleSubmission(ctx context.Context, intentID, hash string, submission Submission) error {
	switch submission.Result {
	case SubmissionConfirmed:
		return s.repository.Confirm(ctx, intentID, hash, submission.LedgerAt.UTC())
	case SubmissionPermanent:
		return s.repository.FailPermanent(ctx, intentID, submission.SafeError)
	case SubmissionUnknown:
		if err := s.repository.MarkUnknown(ctx, intentID, submission.SafeError); err != nil {
			return err
		}
		return s.reconcile(ctx, intentID, hash)
	case SubmissionPending, SubmissionRetryable:
		return s.repository.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), submission.SafeError)
	default:
		return errors.New("invalid Stellar submission result")
	}
}
