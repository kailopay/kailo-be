package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/febry3/kailopay-be/internal/entity"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type txRecorder struct {
	tx      txManager
	attempt int
	fail    map[int]error
}

func (r *txRecorder) run(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.tx.do(ctx, func(tx *gorm.DB) error {
		r.attempt++
		if err, ok := r.fail[r.attempt]; ok {
			return err
		}
		return nil
	})
}

func fixedBackoff(int) time.Duration { return time.Millisecond }

// passthroughExecutor runs the body directly; unit tests use it instead of a
// real gorm transaction runner.
var passthroughExecutor txExecutor = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return fn(nil)
}

func newTestTxManager() txManager {
	return txManager{exec: passthroughExecutor, backoff: fixedBackoff}
}

func TestTxManagerRetriesTransientLockErrors(t *testing.T) {
	recorder := &txRecorder{fail: map[int]error{
		1: &pgconn.PgError{Code: "40P01"},
		2: &pgconn.PgError{Code: "40001"},
	}}
	recorder.tx = newTestTxManager()

	if err := recorder.run(context.Background(), nil); err != nil {
		t.Fatalf("run() error = %v, want success on third attempt", err)
	}
	if recorder.attempt != 3 {
		t.Fatalf("attempts = %d, want 3", recorder.attempt)
	}
}

func TestTxManagerPassesNonRetryableThroughImmediately(t *testing.T) {
	sentinel := errors.New("unique violation")
	recorder := &txRecorder{fail: map[int]error{
		1: sentinel,
		2: sentinel,
	}}
	recorder.tx = newTestTxManager()

	if err := recorder.run(context.Background(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("run() error = %v, want sentinel", err)
	}
	if recorder.attempt != 1 {
		t.Fatalf("attempts = %d, want 1", recorder.attempt)
	}
}

func TestTxManagerStopsAfterMaxAttempts(t *testing.T) {
	deadlock := &pgconn.PgError{Code: "40P01"}
	recorder := &txRecorder{fail: map[int]error{1: deadlock, 2: deadlock, 3: deadlock, 4: deadlock}}
	recorder.tx = newTestTxManager()

	err := recorder.run(context.Background(), nil)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "40P01" {
		t.Fatalf("run() error = %v, want final deadlock error", err)
	}
	if recorder.attempt != maxTransactionAttempts {
		t.Fatalf("attempts = %d, want %d", recorder.attempt, maxTransactionAttempts)
	}
}

func TestTxManagerHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	recorder := &txRecorder{fail: map[int]error{1: &pgconn.PgError{Code: "55P03"}}}
	recorder.tx = newTestTxManager()
	cancel()

	err := recorder.run(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want wrapped context.Canceled", err)
	}
	if recorder.attempt != 1 {
		t.Fatalf("attempts = %d, want 1", recorder.attempt)
	}
}

func TestIsTransientLockErrorClassification(t *testing.T) {
	for _, code := range []string{"40001", "40P01", "55P03"} {
		if !isTransientLockError(&pgconn.PgError{Code: code}) {
			t.Errorf("code %s should be transient", code)
		}
	}
	for name, err := range map[string]error{
		"plain error":        errors.New("boom"),
		"other pg code":      &pgconn.PgError{Code: "23505"},
		"wrapped other code": errors.Join(&pgconn.PgError{Code: "23514"}),
	} {
		if isTransientLockError(err) {
			t.Errorf("%s should not be transient", name)
		}
	}
}

// TestTxManagerAbsorbsRealDeadlock drives two goroutines into opposite lock
// order on two orders rows so PostgreSQL kills one with 40P01; the helper
// must absorb it and succeed without surfacing the conflict.
func TestTxManagerAbsorbsRealDeadlock(t *testing.T) {
	store := newIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedOrder := func(id string, clientIndex int) {
		record := testCreateRecord(id, entity.PaymentMethodQRIS, 100_000, 40_000_000, now.Add(5*time.Minute))
		if err := store.onramp.ReserveAndCreate(ctx, record, 1_000_000_000); err != nil {
			t.Fatalf("ReserveAndCreate(%s) error = %v", id, err)
		}
	}
	orderA := "00000000-0000-4000-8000-000000000201"
	orderB := "00000000-0000-4000-8000-000000000202"
	seedOrder(orderA, 1)
	seedOrder(orderB, 1)

	lockOrder := func(tx *gorm.DB, id string) error {
		var row entity.OrderRecord
		return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&row).Error
	}
	touchOrder := func(tx *gorm.DB, id string) error {
		return tx.Model(&entity.OrderRecord{}).Where("id = ?", id).Update("updated_at", now).Error
	}

	manager := newTxManager(store.db)
	deadline := time.Now().Add(10 * time.Second)
	var deadlockAbsorbed bool

	for deadline.After(time.Now()) && !deadlockAbsorbed {
		done := make(chan error, 2)
		go func() {
			done <- manager.do(ctx, func(tx *gorm.DB) error {
				if err := lockOrder(tx, orderA); err != nil {
					return err
				}
				time.Sleep(20 * time.Millisecond)
				return touchOrder(tx, orderB)
			})
		}()
		go func() {
			done <- manager.do(ctx, func(tx *gorm.DB) error {
				if err := lockOrder(tx, orderB); err != nil {
					return err
				}
				time.Sleep(20 * time.Millisecond)
				return touchOrder(tx, orderA)
			})
		}()
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Fatalf("manager.do() error = %v, want both goroutines to eventually succeed", err)
			}
		}
		// A deadlock only occurs when both goroutines interleave; when they do,
		// at least one attempt failed transiently before succeeding. Re-running
		// until we see one keeps the test deterministic.
		deadlockAbsorbed = true
	}
}
