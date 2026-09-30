package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrRetirementHashAlreadySaved indicates another worker persisted a
// retirement transaction first. The caller must reconcile that hash instead
// of submitting its newly built transaction.
var ErrRetirementHashAlreadySaved = errors.New("retirement transaction hash already saved")

// OfframpWorkerRepository is the persistence port for off-ramp retirement
// worker jobs.
type OfframpWorkerRepository interface {
	SaveRetirementHash(ctx context.Context, intentID, hash string, now time.Time) error
	ResetRetirementHash(ctx context.Context, intentID, hash, safeError string, now time.Time) error
	SimulateRetirement(ctx context.Context, intentID string, now time.Time) error
	ConfirmRetirement(ctx context.Context, intentID, hash string, ledgerAt time.Time) error
	FailRetirement(ctx context.Context, intentID, safeError string) error
}

// DepositScanner combines the ports the deposit-verification job needs.
type DepositScanner struct {
	Candidates DepositWatcherSource
	Expiry     DepositExpirySource
	Payments   DepositWatcher
	Repository OfframpDepositSink
}

// DepositWatcherSource lists asset_pending orders awaiting deposits.
type DepositWatcherSource interface {
	FindDepositCandidates(ctx context.Context, depositAccount string, limit int) ([]OrderView, error)
}

// DepositExpirySource expires asset_pending orders whose deposit window has
// elapsed. It is separate from DepositWatcherSource so the worker can run the
// local state transition even when no external payment history is available.
type DepositExpirySource interface {
	ExpirePendingDeposits(ctx context.Context, now time.Time, limit int) (int, error)
}

// OfframpDepositSink persists verified or invalid deposits.
type OfframpDepositSink interface {
	RecordAssetReceived(ctx context.Context, orderID string, payment ObservedPayment) error
	RecordAssetInvalid(ctx context.Context, orderID string, hash, safeReason string) error
}

// ExpireDeposits advances expired off-ramp orders before a payment scan. The
// repository operation is idempotent, and a scanner without an expiry source
// remains usable by lightweight callers and tests.
func (s DepositScanner) ExpireDeposits(ctx context.Context, now time.Time) (int, error) {
	if s.Expiry == nil {
		return 0, nil
	}
	expired, err := s.Expiry.ExpirePendingDeposits(ctx, now.UTC(), 100)
	if err != nil {
		return 0, fmt.Errorf("expiring pending deposits: %w", err)
	}
	return expired, nil
}

// ScanDeposits checks every awaiting order against recent payments to the
// deposit account. Matching requires a successful payment with the exact
// configured asset code and issuer, memo, stroop amount, and an unused hash.
// Correlated-but-mismatched payments invalidate the order; uncorrelated
// payments are ignored.
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
		if payment.To == depositAccount && payment.TransactionHash != "" && payment.LedgerAt.IsZero() == false && payment.Amount > 0 {
			pending = append(pending, payment)
		}
	}

	matched := 0
	for _, order := range candidates {
		expectedMemo := order.StellarMemo
		if expectedMemo == "" {
			expectedMemo = OfframpDepositMemo(order.ID)
		}
		var match *ObservedPayment
		var mismatchReason string
		var mismatchHash string
		for index := range pending {
			payment := pending[index]
			if payment.Memo != expectedMemo {
				continue
			}
			if payment.Amount != order.AssetAmount {
				if mismatchReason == "" {
					mismatchReason = "deposit amount does not match the quoted amount"
					mismatchHash = payment.TransactionHash
				}
				continue
			}
			if payment.AssetCode != order.AssetCode || payment.AssetIssuer != order.AssetIssuer {
				if mismatchReason == "" {
					mismatchReason = "deposit asset does not match the configured KXLM issuer"
					mismatchHash = payment.TransactionHash
				}
				continue
			}
			match = &pending[index]
			break
		}
		switch {
		case match != nil:
			if err := s.Repository.RecordAssetReceived(ctx, order.ID, *match); err != nil {
				return matched, fmt.Errorf("recording deposit for %s: %w", order.ID, err)
			}
			matched++
		case mismatchReason != "":
			if err := s.Repository.RecordAssetInvalid(ctx, order.ID, mismatchHash, mismatchReason); err != nil {
				return matched, fmt.Errorf("invalidating %s: %w", order.ID, err)
			}
		}
	}
	return matched, nil
}

// RetireWorker processes retirement intents. Persisted hashes are always
// reconciled before any other action. Simulation remains available for
// explicit test configurations; the worker process submits testnet transfers.
type RetireWorker struct {
	Repository    OfframpWorkerRepository
	Intents       RetirementIntentReader
	Network       Network
	LegacyNetwork Network
	Config        SettlementConfig
	Simulate      bool
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
		if w.networkFor(intent) == nil {
			return errors.New("stellar network is required to reconcile a retirement hash")
		}
		return w.reconcile(ctx, intent.IntentID, intent.TransactionHash)
	}
	if w.Simulate {
		return w.Repository.SimulateRetirement(ctx, intent.IntentID, now)
	}
	network := w.networkFor(intent)
	if network == nil {
		return errors.New("stellar network is required to submit a retirement")
	}
	destination := intent.Destination
	if destination == "" && intent.ClawbackFrom == "" {
		destination = RetirementSinkAddress
	}
	transfer := Transfer{OrderID: intent.OrderID, Source: intent.Source, Destination: destination,
		AssetCode: intent.AssetCode, AssetIssuer: intent.AssetIssuer, ClawbackFrom: intent.ClawbackFrom,
		Amount: intent.Amount, Memo: intent.Memo}
	built, buildErr := network.Build(ctx, transfer)
	if buildErr != nil {
		return fmt.Errorf("building retirement transaction: %w", buildErr)
	}
	if built.Hash == "" || built.Envelope == "" {
		return errors.New("Stellar adapter returned incomplete retirement transaction")
	}
	if err := w.Repository.SaveRetirementHash(ctx, intent.IntentID, built.Hash, now); err != nil {
		if errors.Is(err, ErrRetirementHashAlreadySaved) {
			current, loadErr := w.Intents.LoadRetirement(ctx, intent.IntentID)
			if loadErr != nil {
				return fmt.Errorf("loading saved retirement transaction: %w", loadErr)
			}
			if current.TransactionHash == "" {
				return errors.New("retirement transaction hash changed before reconciliation")
			}
			return w.reconcile(ctx, intent.IntentID, current.TransactionHash)
		}
		return fmt.Errorf("persisting retirement hash: %w", err)
	}
	submission, submitErr := network.Submit(ctx, built)
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
	case SubmissionRetryable:
		if err := w.Repository.ResetRetirementHash(ctx, intent.IntentID, built.Hash, submission.SafeError, now); err != nil {
			return fmt.Errorf("resetting rejected retirement submission: %w", err)
		}
		return errors.New("stellar retirement transaction was rejected before application and will be retried")
	case SubmissionPending:
		return nil // lease expiry schedules the retry
	default:
		return errors.New("invalid retirement submission result")
	}
}

func (w RetireWorker) reconcile(ctx context.Context, intentID, hash string) error {
	intent, err := w.Intents.LoadRetirement(ctx, intentID)
	if err != nil {
		return fmt.Errorf("loading retirement intent for reconciliation: %w", err)
	}
	network := w.networkFor(intent)
	if network == nil {
		return errors.New("stellar network is required to reconcile a retirement hash")
	}
	result, err := network.FindByHash(ctx, hash)
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

func (w RetireWorker) networkFor(intent RetirementIntent) Network {
	if intent.ClawbackFrom != "" || w.LegacyNetwork == nil {
		return w.Network
	}
	return w.LegacyNetwork
}
