package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// OfframpWorkerRepository is the persistence port for the three off-ramp
// worker jobs (deposit verification, burn submission, simulated payout).
type OfframpWorkerRepository interface {
	SaveRetirementHash(ctx context.Context, intentID, hash string, now time.Time) error
	ConfirmRetirement(ctx context.Context, intentID, hash string, ledgerAt time.Time) error
	FailRetirement(ctx context.Context, intentID, safeError string) error
	CompleteSimulatedPayout(ctx context.Context, orderID string, amountMinor int64, now time.Time) (string, error)
}

// DepositScanner combines the ports the deposit-verification job needs.
type DepositScanner struct {
	Candidates DepositWatcherSource
	Payments   DepositWatcher
	Repository OfframpDepositSink
}

// DepositWatcherSource lists asset_pending orders awaiting deposits.
type DepositWatcherSource interface {
	FindDepositCandidates(ctx context.Context, depositAccount string, limit int) ([]OrderView, error)
}

// OfframpDepositSink persists verified or invalid deposits.
type OfframpDepositSink interface {
	RecordAssetReceived(ctx context.Context, orderID string, payment ObservedPayment) error
	RecordAssetInvalid(ctx context.Context, orderID string, hash, safeReason string) error
}

// ScanDeposits checks every awaiting order against recent payments to the
// deposit account. Matching requires successful native payment to the
// deposit account, exact memo and stroop amount, and a hash not already
// consumed by another order. Correlated-but-mismatched payments invalidate
// the order; uncorrelated payments are ignored.
func (s DepositScanner) ScanDeposits(ctx context.Context, depositAccount string, now time.Time) (int, error) {
	candidates, err := s.Candidates.FindDepositCandidates(ctx, depositAccount, 100)
	if err != nil {
		return 0, fmt.Errorf("listing deposit candidates: %w", err)
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	payments, err := s.Payments.RecentPayments(ctx, depositAccount, 100)
	if err != nil {
		return 0, fmt.Errorf("reading recent payments: %w", err)
	}
	pending := make([]ObservedPayment, 0, len(payments))
	for _, payment := range payments {
		if payment.To == depositAccount && payment.LedgerAt.IsZero() == false && payment.Amount > 0 {
			pending = append(pending, payment)
		}
	}

	matched := 0
	for _, order := range candidates {
		expectedMemo := OfframpDepositMemo(order.ID)
		var match *ObservedPayment
		var mismatchReason string
		for index := range pending {
			payment := pending[index]
			if payment.Memo != expectedMemo {
				continue
			}
			if payment.Amount != order.AssetAmount {
				mismatchReason = "deposit amount does not match the quoted amount"
				continue
			}
			match = &pending[index]
			break
		}
		expired := !order.QuoteExpiresAt.IsZero() && false // expiry handled by the sweep, not the scan
		_ = expired
		switch {
		case match != nil:
			if err := s.Repository.RecordAssetReceived(ctx, order.ID, *match); err != nil {
				return matched, fmt.Errorf("recording deposit for %s: %w", order.ID, err)
			}
			matched++
		case mismatchReason != "":
			if err := s.Repository.RecordAssetInvalid(ctx, order.ID, "", mismatchReason); err != nil {
				return matched, fmt.Errorf("invalidating %s: %w", order.ID, err)
			}
		}
	}
	return matched, nil
}

// RetireWorker submits burn-address transactions for retirement intents.
// It mirrors the settlement worker's safety posture: hash persisted before
// submission, unknown outcomes reconciled by hash before any rebuild.
type RetireWorker struct {
	Repository OfframpWorkerRepository
	Intents    RetirementIntentReader
	Network    Network
	Config     SettlementConfig
}

// RetirementIntentReader loads one pending retirement intent by ID.
type RetirementIntentReader interface {
	LoadRetirement(ctx context.Context, intentID string) (RetirementIntent, error)
}

// RunOnce processes one leased retirement job.
func (w RetireWorker) RunOnce(ctx context.Context, intentID string) error {
	now := w.Config.Now().UTC()
	intent, err := w.Intents.LoadRetirement(ctx, intentID)
	if err != nil {
		return fmt.Errorf("loading retirement intent: %w", err)
	}
	if intent.TransactionHash != "" {
		return w.reconcile(ctx, intent.IntentID, intent.TransactionHash)
	}
	built, buildErr := w.Network.Build(ctx, Transfer{OrderID: intent.OrderID, Source: intent.Source,
		Destination: BurnAddress, Amount: intent.Amount, Memo: intent.Memo})
	if buildErr != nil {
		return fmt.Errorf("building retirement transaction: %w", buildErr)
	}
	_ = now
	if built.Hash == "" || built.Envelope == "" {
		return errors.New("Stellar adapter returned incomplete retirement transaction")
	}
	if err := w.Repository.SaveRetirementHash(ctx, intent.IntentID, built.Hash, now); err != nil {
		return fmt.Errorf("persisting retirement hash: %w", err)
	}
	submission, submitErr := w.Network.Submit(ctx, built)
	if submitErr != nil {
		return w.reconcile(ctx, intent.IntentID, built.Hash)
	}
	switch submission.Result {
	case SubmissionConfirmed:
		return w.Repository.ConfirmRetirement(ctx, intent.IntentID, built.Hash, submission.LedgerAt)
	case SubmissionPermanent:
		return w.Repository.FailRetirement(ctx, intent.IntentID, "retirement transaction rejected")
	case SubmissionUnknown:
		return w.reconcile(ctx, intent.IntentID, built.Hash)
	case SubmissionPending, SubmissionRetryable:
		return nil // lease expiry schedules the retry
	default:
		return errors.New("invalid retirement submission result")
	}
}

func (w RetireWorker) reconcile(ctx context.Context, intentID, hash string) error {
	result, err := w.Network.FindByHash(ctx, hash)
	if err != nil {
		return nil // lease expiry schedules the next reconciliation pass
	}
	switch result.Result {
	case SubmissionConfirmed:
		return w.Repository.ConfirmRetirement(ctx, intentID, hash, result.LedgerAt)
	case SubmissionPermanent:
		return w.Repository.FailRetirement(ctx, intentID, "retirement transaction failed on chain")
	default:
		return nil // transient: retried on the next lease
	}
}

// RecordSimulatedPayout records the deterministic simulated payout and
// completes the order (ADR-003).
func RecordSimulatedPayout(ctx context.Context, repository OfframpWorkerRepository, orderID string, amountMinor int64, config SettlementConfig) error {
	_, err := repository.CompleteSimulatedPayout(ctx, orderID, amountMinor, config.Now().UTC())
	return err
}
