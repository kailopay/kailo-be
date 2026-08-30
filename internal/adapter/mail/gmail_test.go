package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

type recordingSMTPClient struct {
	startTLSCalled bool
	authCalled     bool
	from           string
	recipient      string
	message        bytes.Buffer
	quitCalled     bool
	closeCalled    bool
	startTLSErr    error
	authErr        error
}

func (c *recordingSMTPClient) Extension(name string) (bool, string) {
	return name == "STARTTLS", ""
}

func (c *recordingSMTPClient) StartTLS(_ *tls.Config) error {
	c.startTLSCalled = true
	return c.startTLSErr
}

func (c *recordingSMTPClient) Auth(_ smtp.Auth) error {
	c.authCalled = true
	return c.authErr
}

func (c *recordingSMTPClient) Mail(from string) error {
	c.from = from
	return nil
}

func (c *recordingSMTPClient) Rcpt(to string) error {
	c.recipient = to
	return nil
}

func (c *recordingSMTPClient) Data() (io.WriteCloser, error) {
	return nopCloseWriter{Writer: &c.message}, nil
}

func (c *recordingSMTPClient) Quit() error {
	c.quitCalled = true
	return nil
}

func (c *recordingSMTPClient) Close() error {
	c.closeCalled = true
	return nil
}

type nopCloseWriter struct {
	io.Writer
}

func (nopCloseWriter) Close() error { return nil }

func TestGmailSenderSendsVerificationEmailOverAuthenticatedTLS(t *testing.T) {
	client := &recordingSMTPClient{}
	sender, err := newGmailSenderForTest(GmailConfig{
		Username:    "sender@gmail.com",
		AppPassword: "abcdefghijklmnop",
		FromName:    "KailoPay",
		Timeout:     5 * time.Second,
	}, func(context.Context, GmailConfig) (smtpClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("newGmailSenderForTest() error = %v", err)
	}

	err = sender.SendEmailVerification(context.Background(), "user@example.com", "https://app.example.com/verify?token=abc")
	if err != nil {
		t.Fatalf("SendEmailVerification() error = %v", err)
	}
	if !client.startTLSCalled || !client.authCalled {
		t.Fatal("sender did not negotiate STARTTLS and authenticate")
	}
	if client.from != "sender@gmail.com" || client.recipient != "user@example.com" {
		t.Fatalf("SMTP envelope = %q -> %q", client.from, client.recipient)
	}
	message := client.message.String()
	for _, expected := range []string{
		"From: \"KailoPay\" <sender@gmail.com>",
		"To: <user@example.com>",
		"Subject: Verify your KailoPay email",
		"https://app.example.com/verify?token=abc",
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("message does not contain %q: %q", expected, message)
		}
	}
	if strings.Contains(message, "abcdefghijklmnop") {
		t.Fatal("message contains the SMTP app password")
	}
	if !client.quitCalled || !client.closeCalled {
		t.Fatal("SMTP client was not closed cleanly")
	}
}

func TestGmailSenderRejectsSMTPWithoutSTARTTLS(t *testing.T) {
	client := &recordingSMTPClient{}
	sender, err := newGmailSenderForTest(GmailConfig{
		Username:    "sender@gmail.com",
		AppPassword: "abcdefghijklmnop",
		Timeout:     5 * time.Second,
	}, func(context.Context, GmailConfig) (smtpClient, error) {
		return noStartTLSClient{recordingSMTPClient: client}, nil
	})
	if err != nil {
		t.Fatalf("newGmailSenderForTest() error = %v", err)
	}

	err = sender.SendPasswordReset(context.Background(), "user@example.com", "https://app.example.com/reset?token=abc")
	if !errors.Is(err, errSMTPStartTLSUnsupported) {
		t.Fatalf("SendPasswordReset() error = %v, want %v", err, errSMTPStartTLSUnsupported)
	}
	if client.authCalled {
		t.Fatal("sender authenticated before STARTTLS")
	}
}

type noStartTLSClient struct {
	*recordingSMTPClient
}

func (noStartTLSClient) Extension(string) (bool, string) { return false, "" }

func TestNewGmailSenderRejectsIncompleteConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config GmailConfig
	}{
		{name: "missing username", config: GmailConfig{AppPassword: "abcdefghijklmnop", Timeout: time.Second}},
		{name: "missing app password", config: GmailConfig{Username: "sender@gmail.com", Timeout: time.Second}},
		{name: "missing timeout", config: GmailConfig{Username: "sender@gmail.com", AppPassword: "abcdefghijklmnop"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewGmailSender(tt.config); err == nil {
				t.Fatal("NewGmailSender() error = nil")
			}
		})
	}
}

func TestNewSenderSelectsConfiguredProvider(t *testing.T) {
	sender, err := NewSender(SenderConfig{Provider: "gmail", Gmail: GmailConfig{
		Username:    "sender@gmail.com",
		AppPassword: "abcdefghijklmnop",
		Timeout:     time.Second,
	}}, nil)
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if _, ok := sender.(*GmailSender); !ok {
		t.Fatalf("NewSender() type = %T, want *GmailSender", sender)
	}
}

func TestNewSenderDefaultsToConsole(t *testing.T) {
	sender, err := NewSender(SenderConfig{}, nil)
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if _, ok := sender.(*ConsoleSender); !ok {
		t.Fatalf("NewSender() type = %T, want *ConsoleSender", sender)
	}
}

func newGmailSenderForTest(config GmailConfig, factory smtpClientFactory) (*GmailSender, error) {
	normalized, err := normalizeGmailConfig(config)
	if err != nil {
		return nil, err
	}
	return &GmailSender{config: normalized, factory: factory}, nil
}
