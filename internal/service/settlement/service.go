package settlement

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

type Store interface {
	Lease(ctx context.Context, workerID string, now time.Time, duration time.Duration) (Job, error)
	LoadIntent(ctx context.Context, intentID string) (Intent, error)
	MarkSubmitted(ctx context.Context, intentID, hash string, now time.Time) error
	Confirm(ctx context.Context, intentID, hash string, ledgerAt time.Time) error
	MarkUnknown(ctx context.Context, intentID, safeError string) error
	FailPermanent(ctx context.Context, intentID, safeError string) error
	RetryLater(ctx context.Context, intentID string, availableAt time.Time, safeError string) error
}

type Network interface {
	Build(ctx context.Context, transfer Transfer) (BuiltTransaction, error)
	Submit(ctx context.Context, transaction BuiltTransaction) (Submission, error)
	FindByHash(ctx context.Context, hash string) (Submission, error)
	SpendableBalance(ctx context.Context, account string) (entity.Stroops, error)
}

type Config struct {
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	Now           func() time.Time
}

type Service struct {
	store   Store
	network Network
	config  Config
}

func New(store Store, network Network, config Config) (*Service, error) {
	if store == nil || network == nil || config.LeaseDuration <= 0 || config.RetryDelay <= 0 || config.Now == nil {
		return nil, errors.New("valid settlement dependencies and configuration are required")
	}
	return &Service{store: store, network: network, config: config}, nil
}

func (s *Service) RunOnce(ctx context.Context, workerID string) (bool, error) {
	now := s.config.Now().UTC()
	job, err := s.store.Lease(ctx, workerID, now, s.config.LeaseDuration)
	if errors.Is(err, ErrNoJob) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("leasing settlement: %w", err)
	}
	intent, err := s.store.LoadIntent(ctx, job.IntentID)
	if err != nil {
		return true, fmt.Errorf("loading settlement intent: %w", err)
	}
	if intent.TransactionHash != "" {
		return true, s.reconcile(ctx, intent.IntentID, intent.TransactionHash)
	}
	built, err := s.network.Build(ctx, intent.Transfer)
	if err != nil {
		_ = s.store.RetryLater(ctx, intent.IntentID, now.Add(s.config.RetryDelay), "building transaction failed")
		return true, fmt.Errorf("building Stellar transaction: %w", err)
	}
	if built.Hash == "" || built.Envelope == "" {
		return true, errors.New("Stellar adapter returned incomplete transaction")
	}
	if err := s.store.MarkSubmitted(ctx, intent.IntentID, built.Hash, now); err != nil {
		return true, fmt.Errorf("persisting Stellar transaction hash: %w", err)
	}
	submission, err := s.network.Submit(ctx, built)
	if err != nil {
		_ = s.store.MarkUnknown(ctx, intent.IntentID, "submission outcome unknown")
		return true, s.reconcile(ctx, intent.IntentID, built.Hash)
	}
	return true, s.handleSubmission(ctx, intent.IntentID, built.Hash, submission)
}

func (s *Service) reconcile(ctx context.Context, intentID, hash string) error {
	result, err := s.network.FindByHash(ctx, hash)
	if err != nil {
		return s.store.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), "transaction reconciliation unavailable")
	}
	if result.Result == SubmissionUnknown || result.Result == SubmissionPending || result.Result == SubmissionRetryable {
		return s.store.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), result.SafeError)
	}
	return s.handleSubmission(ctx, intentID, hash, result)
}

func (s *Service) handleSubmission(ctx context.Context, intentID, hash string, submission Submission) error {
	switch submission.Result {
	case SubmissionConfirmed:
		return s.store.Confirm(ctx, intentID, hash, submission.LedgerAt.UTC())
	case SubmissionPermanent:
		return s.store.FailPermanent(ctx, intentID, submission.SafeError)
	case SubmissionUnknown:
		if err := s.store.MarkUnknown(ctx, intentID, submission.SafeError); err != nil {
			return err
		}
		return s.reconcile(ctx, intentID, hash)
	case SubmissionPending, SubmissionRetryable:
		return s.store.RetryLater(ctx, intentID, s.config.Now().UTC().Add(s.config.RetryDelay), submission.SafeError)
	default:
		return errors.New("invalid Stellar submission result")
	}
}
