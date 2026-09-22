package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SandboxPayoutMode string

const (
	SandboxPayoutDisabled  SandboxPayoutMode = "disabled"
	SandboxPayoutSimulated SandboxPayoutMode = "simulated"
	SandboxPayoutTopic                       = "payout.simulate_offramp"
)

var (
	ErrSandboxPayoutDisabled = errors.New("sandbox payout simulator is disabled")
	ErrSandboxPayoutInvalid  = errors.New("invalid sandbox payout job")
)

type SandboxPayoutRepository interface {
	CompleteSandboxPayout(ctx context.Context, orderID, outboxID, reference string, now time.Time) error
}

type SandboxPayoutWorker struct {
	Repository SandboxPayoutRepository
	Mode       SandboxPayoutMode
	Now        func() time.Time
}

func NewSandboxPayoutWorker(repository SandboxPayoutRepository, mode SandboxPayoutMode, now func() time.Time) (*SandboxPayoutWorker, error) {
	if repository == nil || now == nil || mode != SandboxPayoutDisabled && mode != SandboxPayoutSimulated {
		return nil, errors.New("valid sandbox payout configuration is required")
	}
	return &SandboxPayoutWorker{Repository: repository, Mode: mode, Now: now}, nil
}

func (w SandboxPayoutWorker) RunOnce(ctx context.Context, job Job) error {
	if w.Mode == SandboxPayoutDisabled {
		return ErrSandboxPayoutDisabled
	}
	orderID := strings.TrimSpace(job.IntentID)
	if orderID == "" || strings.TrimSpace(job.OutboxID) == "" {
		return ErrSandboxPayoutInvalid
	}
	reference := SandboxPayoutReference(orderID)
	if err := w.Repository.CompleteSandboxPayout(ctx, orderID, job.OutboxID, reference, w.Now().UTC()); err != nil {
		return fmt.Errorf("completing sandbox payout: %w", err)
	}
	return nil
}

func SandboxPayoutReference(orderID string) string {
	return "sandbox-payout-" + strings.TrimSpace(orderID)
}
