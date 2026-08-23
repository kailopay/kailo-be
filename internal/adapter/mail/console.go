package mail

import (
	"context"
	"log/slog"

	"github.com/febry3/kailopay-be/internal/usecase"
)

// ConsoleSender delivers authentication links by logging them. It is the
// sandbox implementation of the usecase EmailSender port; SMTP or a provider
// API can replace it without touching the usecase layer.
type ConsoleSender struct {
	logger *slog.Logger
}

func NewConsoleSender(logger *slog.Logger) *ConsoleSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConsoleSender{logger: logger}
}

func (s *ConsoleSender) SendEmailVerification(_ context.Context, email, link string) error {
	s.logger.Info("email verification link", slog.String("email", email), slog.String("link", link))
	return nil
}

func (s *ConsoleSender) SendPasswordReset(_ context.Context, email, link string) error {
	s.logger.Info("password reset link", slog.String("email", email), slog.String("link", link))
	return nil
}

var _ usecase.EmailSender = (*ConsoleSender)(nil)
