package repository

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// maxTransactionAttempts bounds the total runs of one transaction body:
// the first attempt plus two retries of transient lock conflicts.
const maxTransactionAttempts = 3

// txExecutor abstracts gorm's transaction runner so the retry loop can be
// unit-tested without a database. Production managers wrap a *gorm.DB.
type txExecutor func(ctx context.Context, fn func(tx *gorm.DB) error) error

func gormExecutor(db *gorm.DB) txExecutor {
	return func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn)
	}
}

// txManager executes transaction bodies with uniform context handling and
// bounded retry of transient PostgreSQL lock conflicts. Every body passed to
// it must be idempotent: a retried run starts from a fully rolled-back state.
type txManager struct {
	exec    txExecutor
	backoff func(attempt int) time.Duration
}

func newTxManager(db *gorm.DB) txManager {
	return txManager{exec: gormExecutor(db), backoff: defaultBackoff}
}

// do runs fn inside a database transaction. Deadlock, serialization, and
// lock-timeout failures roll the transaction back completely and are retried
// up to twice with jittered backoff; every other error aborts immediately.
// A context cancelled after a transient failure returns that failure joined
// with the cancellation cause.
func (m txManager) do(ctx context.Context, fn func(tx *gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < maxTransactionAttempts; attempt++ {
		if err = m.exec(ctx, fn); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		if !isTransientLockError(err) {
			return err
		}
		delay := m.backoff(attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
	return err
}

func defaultBackoff(attempt int) time.Duration {
	base := time.Duration(50*(attempt+1)) * time.Millisecond
	if base > 500*time.Millisecond {
		base = 500 * time.Millisecond
	}
	return time.Duration(rand.Int63n(int64(base)) + int64(base)/2)
}

// isTransientLockError reports whether the error is a PostgreSQL condition
// that a fresh attempt after full rollback can plausibly resolve.
func isTransientLockError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001": // serialization_failure
		return true
	case "40P01": // deadlock_detected
		return true
	case "55P03": // lock_not_available
		return true
	default:
		return false
	}
}
