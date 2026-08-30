package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	stellaradapter "github.com/febry3/kailopay-be/internal/adapter/stellar"
	"github.com/febry3/kailopay-be/internal/platform"
	"github.com/febry3/kailopay-be/internal/repository"
	"github.com/febry3/kailopay-be/internal/usecase"
)

// outboxTopics are drained in order each tick; a topic with work keeps the
// loop running before other topics are polled.
var outboxTopics = []string{"stellar.settle_onramp", "stellar.retire_offramp", "stellar.pay_offramp"}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("worker stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := platform.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger, err := platform.NewLogger(cfg.Logging.Level, cfg.Logging.Format, os.Stdout)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	logger = platform.WithMetadata(logger, "kailopay", "worker", cfg.App.Environment, cfg.App.Version)
	slog.SetDefault(logger)
	db, closeDB, err := platform.Open(ctx, platform.DBConfig{DSN: cfg.Database.DSN, MaxOpenConns: cfg.Database.MaxOpenConns,
		MaxIdleConns: cfg.Database.MaxIdleConns, ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.Database.ConnMaxIdleTime, PingTimeout: cfg.Database.PingTimeout}, logger)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := closeDB(); err != nil {
			logger.Error("closing worker database failed", slog.Any("error", err))
		}
	}()
	treasurySecret, err := platform.LoadWorkerTreasurySecret()
	if err != nil {
		return fmt.Errorf("loading treasury secret: %w", err)
	}
	depositSecret, err := platform.LoadWorkerDepositSecret()
	if err != nil {
		return fmt.Errorf("loading deposit secret: %w", err)
	}
	httpClient := &http.Client{Timeout: cfg.Week1.Stellar.Timeout}
	network, err := stellaradapter.New(stellaradapter.Config{HorizonURL: cfg.Week1.Stellar.HorizonURL,
		NetworkPassphrase: cfg.Week1.Stellar.NetworkPassphrase, TreasurySecret: treasurySecret,
		HTTPClient: httpClient, TransactionTimeout: cfg.Week1.Worker.SubmissionTimeout})
	if err != nil {
		return fmt.Errorf("creating Stellar client: %w", err)
	}
	// The deposit signer submits burn-address retirements from the deposit
	// account; it is a separate client so the treasury key never signs them.
	depositSigner, err := stellaradapter.New(stellaradapter.Config{HorizonURL: cfg.Week1.Stellar.HorizonURL,
		NetworkPassphrase: cfg.Week1.Stellar.NetworkPassphrase, TreasurySecret: depositSecret,
		HTTPClient: httpClient, TransactionTimeout: cfg.Week1.Worker.SubmissionTimeout})
	if err != nil {
		return fmt.Errorf("creating deposit signer client: %w", err)
	}

	settlementStore := repository.NewSettlementRepository(db, cfg.Week1.Worker.MaxAttempts)
	settlementService, err := usecase.NewSettlementUsecase(settlementStore, network, usecase.SettlementConfig{
		LeaseDuration: cfg.Week1.Worker.LeaseDuration, RetryDelay: cfg.Week1.Worker.RetryDelay, Now: time.Now})
	if err != nil {
		return fmt.Errorf("creating settlement service: %w", err)
	}
	offrampRepo := repository.NewOfframpRepository(db, cfg.Week1.Offramp.DepositAccount, usecase.StellarTestnetNetwork)
	retireWorker := usecase.RetireWorker{Repository: offrampRepo, Intents: offrampRepo,
		Network: depositSigner, Config: usecase.SettlementConfig{
			LeaseDuration: cfg.Week1.Worker.LeaseDuration, RetryDelay: cfg.Week1.Worker.RetryDelay, Now: time.Now}}
	paymentWatcher := depositWatcherFunc(func(ctx context.Context, account string, limit int) ([]usecase.ObservedPayment, error) {
		observed, err := network.RecentPayments(ctx, account, limit)
		if err != nil {
			return nil, err
		}
		converted := make([]usecase.ObservedPayment, 0, len(observed))
		for _, payment := range observed {
			converted = append(converted, usecase.ObservedPayment{TransactionHash: payment.TransactionHash,
				From: payment.From, To: payment.To, Amount: payment.Amount, Memo: payment.Memo, LedgerAt: payment.LedgerAt})
		}
		return converted, nil
	})
	scanner := usecase.DepositScanner{Candidates: offrampRepo, Payments: paymentWatcher, Repository: offrampRepo}

	workerID, err := platform.NewID()
	if err != nil {
		return fmt.Errorf("generating worker id: %w", err)
	}
	runSettlement := func() (bool, error) { return settlementService.RunOnce(ctx, workerID) }
	runRetirement := func() (bool, error) {
		job, err := settlementStore.LeaseOutbox(ctx, "stellar.retire_offramp", workerID, time.Now().UTC(),
			cfg.Week1.Worker.LeaseDuration, intentPayloadDecoder)
		if errors.Is(err, usecase.ErrNoJob) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, retireWorker.RunOnce(ctx, job.IntentID)
	}
	runPayout := func() (bool, error) {
		job, err := settlementStore.LeaseOutbox(ctx, "stellar.pay_offramp", workerID, time.Now().UTC(),
			cfg.Week1.Worker.LeaseDuration, orderPayloadDecoder)
		if errors.Is(err, usecase.ErrNoJob) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		amount, loadErr := offrampRepo.LoadOfframpOrderAmount(ctx, job.IntentID)
		if loadErr != nil {
			return true, loadErr
		}
		if _, payErr := offrampRepo.CompleteSimulatedPayout(ctx, job.IntentID, amount, time.Now().UTC()); payErr != nil {
			return true, payErr
		}
		return true, settlementStore.FinishOutbox(ctx, "stellar.pay_offramp", job.IntentID, time.Now().UTC())
	}
	drain := map[string]func() (bool, error){
		"stellar.settle_onramp":  runSettlement,
		"stellar.retire_offramp": runRetirement,
		"stellar.pay_offramp":    runPayout,
	}

	depositPollInterval := cfg.Week1.Worker.PollInterval * 10
	ticker := time.NewTicker(cfg.Week1.Worker.PollInterval)
	defer ticker.Stop()
	var lastSweep time.Time
	var lastScan time.Time
	for {
		if time.Since(lastSweep) >= cfg.Week1.Worker.PollInterval {
			released, sweepErr := settlementService.ReleaseExpired(ctx)
			if sweepErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "releasing expired reservations failed", slog.Any("error", sweepErr))
			} else if released > 0 {
				logger.InfoContext(ctx, "released expired treasury reservations", slog.Int("released", released))
			}
			lastSweep = time.Now()
		}
		if time.Since(lastScan) >= depositPollInterval {
			matched, scanErr := scanner.ScanDeposits(ctx, cfg.Week1.Offramp.DepositAccount, time.Now())
			if scanErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "deposit scan failed", slog.Any("error", scanErr))
			} else if matched > 0 {
				logger.InfoContext(ctx, "verified off-ramp deposits", slog.Int("matched", matched))
			}
			lastScan = time.Now()
		}
		workDone := false
		for _, topic := range outboxTopics {
			processed, jobErr := drain[topic]()
			if jobErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, topic+" job failed", slog.Any("error", jobErr))
			}
			if processed {
				workDone = true
				break // keep draining this topic before polling the next
			}
		}
		if workDone {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func intentPayloadDecoder(payload []byte) (usecase.Job, error) {
	var decoded struct {
		IntentID string `json:"intent_id"`
	}
	if json.Unmarshal(payload, &decoded) != nil || decoded.IntentID == "" {
		return usecase.Job{}, errors.New("invalid intent payload")
	}
	return usecase.Job{IntentID: decoded.IntentID}, nil
}

func orderPayloadDecoder(payload []byte) (usecase.Job, error) {
	var decoded struct {
		OrderID string `json:"order_id"`
	}
	if json.Unmarshal(payload, &decoded) != nil || decoded.OrderID == "" {
		return usecase.Job{}, errors.New("invalid payout payload")
	}
	return usecase.Job{OutboxID: "pay-" + decoded.OrderID, IntentID: decoded.OrderID}, nil
}

// depositWatcherFunc adapts the stellar adapter's observation type to the
// usecase-owned DepositWatcher port.
type depositWatcherFunc func(ctx context.Context, account string, limit int) ([]usecase.ObservedPayment, error)

func (f depositWatcherFunc) RecentPayments(ctx context.Context, account string, limit int) ([]usecase.ObservedPayment, error) {
	return f(ctx, account, limit)
}
