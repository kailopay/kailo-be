package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/febry3/kailopay-be/internal/usecase"
)

const (
	gmailSMTPHost = "smtp.gmail.com"
	gmailSMTPPort = 587
)

var errSMTPStartTLSUnsupported = errors.New("gmail smtp server does not support starttls")

// GmailConfig contains the credentials and delivery settings for Gmail SMTP.
// AppPassword must be a Google App Password, not the account password.
type GmailConfig struct {
	Username    string
	AppPassword string
	FromName    string
	Timeout     time.Duration
}

// SenderConfig selects the authentication email delivery implementation.
type SenderConfig struct {
	Provider string
	Gmail    GmailConfig
}

// GmailSender delivers authentication emails through Gmail SMTP with
// STARTTLS and application-password authentication.
type GmailSender struct {
	config  GmailConfig
	factory smtpClientFactory
}

type smtpClient interface {
	Extension(ext string) (bool, string)
	StartTLS(config *tls.Config) error
	Auth(auth smtp.Auth) error
	Mail(from string) error
	Rcpt(to string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpClientFactory func(context.Context, GmailConfig) (smtpClient, error)

// NewGmailSender validates the Gmail configuration and returns a sender.
func NewGmailSender(config GmailConfig) (*GmailSender, error) {
	normalized, err := normalizeGmailConfig(config)
	if err != nil {
		return nil, err
	}
	return &GmailSender{config: normalized, factory: gmailSMTPClientFactory}, nil
}

// NewSender returns the configured authentication email sender. Console
// delivery remains the default for local development.
func NewSender(config SenderConfig, logger *slog.Logger) (usecase.EmailSender, error) {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	switch provider {
	case "", "console":
		return NewConsoleSender(logger), nil
	case "gmail":
		return NewGmailSender(config.Gmail)
	default:
		return nil, fmt.Errorf("unsupported email provider %q", provider)
	}
}

func (s *GmailSender) SendEmailVerification(ctx context.Context, email, link string) error {
	body := authEmailBody("Verify your KailoPay email by opening this link:", link,
		"If you did not create this account, you can ignore this email.")
	return s.send(ctx, email, "Verify your KailoPay email", body)
}

func (s *GmailSender) SendPasswordReset(ctx context.Context, email, link string) error {
	body := authEmailBody("Reset your KailoPay password by opening this link:", link,
		"If you did not request a password reset, you can ignore this email.")
	return s.send(ctx, email, "Reset your KailoPay password", body)
}

func (s *GmailSender) send(ctx context.Context, recipient, subject, body string) error {
	if ctx == nil {
		return errors.New("email context is nil")
	}
	if !validMailbox(recipient) {
		return errors.New("recipient email address is invalid")
	}

	operationContext, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	client, err := s.factory(operationContext, s.config)
	if err != nil {
		return fmt.Errorf("connecting to gmail smtp: %w", err)
	}
	defer func() { _ = client.Close() }()

	if supported, _ := client.Extension("STARTTLS"); !supported {
		return errSMTPStartTLSUnsupported
	}
	if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: gmailSMTPHost}); err != nil {
		return fmt.Errorf("starting gmail smtp tls: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.AppPassword, gmailSMTPHost)); err != nil {
		return fmt.Errorf("authenticating with gmail smtp: %w", err)
	}
	if err := client.Mail(s.config.Username); err != nil {
		return fmt.Errorf("setting gmail sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("setting email recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("opening email body: %w", err)
	}
	if _, err := io.WriteString(writer, buildMessage(s.config, recipient, subject, body)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("writing email body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing email body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("closing gmail smtp session: %w", err)
	}
	return nil
}

func normalizeGmailConfig(config GmailConfig) (GmailConfig, error) {
	config.Username = strings.TrimSpace(config.Username)
	config.AppPassword = strings.ReplaceAll(strings.TrimSpace(config.AppPassword), " ", "")
	config.FromName = strings.TrimSpace(config.FromName)
	if config.FromName == "" {
		config.FromName = "KailoPay"
	}
	if !validMailbox(config.Username) {
		return GmailConfig{}, errors.New("gmail username must be a valid email address")
	}
	if config.AppPassword == "" || strings.ContainsAny(config.AppPassword, "\r\n") {
		return GmailConfig{}, errors.New("gmail app password is required")
	}
	if strings.ContainsAny(config.FromName, "\r\n") {
		return GmailConfig{}, errors.New("gmail from name contains invalid characters")
	}
	if config.Timeout <= 0 {
		return GmailConfig{}, errors.New("gmail smtp timeout must be positive")
	}
	return config, nil
}

func validMailbox(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return false
	}
	parsed, err := stdmail.ParseAddress(raw)
	return err == nil && strings.EqualFold(parsed.Address, raw)
}

func buildMessage(config GmailConfig, recipient, subject, body string) string {
	from := (&stdmail.Address{Name: config.FromName, Address: config.Username}).String()
	to := (&stdmail.Address{Address: recipient}).String()
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}

func authEmailBody(instruction, link, warning string) string {
	return strings.Join([]string{
		"Hello,",
		"",
		instruction,
		link,
		"",
		warning,
		"",
		"KailoPay",
	}, "\r\n") + "\r\n"
}

func gmailSMTPClientFactory(ctx context.Context, config GmailConfig) (smtpClient, error) {
	address := net.JoinHostPort(gmailSMTPHost, strconv.Itoa(gmailSMTPPort))
	dialer := net.Dialer{Timeout: config.Timeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(config.Timeout)
	}
	if err := connection.SetDeadline(deadline); err != nil {
		_ = connection.Close()
		return nil, err
	}
	client, err := smtp.NewClient(connection, gmailSMTPHost)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return client, nil
}

var _ usecase.EmailSender = (*GmailSender)(nil)
