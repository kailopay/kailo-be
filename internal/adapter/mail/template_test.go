package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	stdmail "net/mail"
	"strings"
	"testing"
	"time"
)

func TestGmailSenderRendersBrandedAuthenticationEmails(t *testing.T) {
	t.Parallel()

	const link = "https://app.example.com/auth/action?token=abc"
	tests := []struct {
		name       string
		send       func(*GmailSender) error
		subject    string
		action     string
		expiration string
	}{
		{
			name: "email verification",
			send: func(sender *GmailSender) error {
				return sender.SendEmailVerification(context.Background(), "user@example.com", link)
			},
			subject:    "Verify your KailoPay email",
			action:     "Confirm your email",
			expiration: "This link expires in 24 hours.",
		},
		{
			name: "password reset",
			send: func(sender *GmailSender) error {
				return sender.SendPasswordReset(context.Background(), "user@example.com", link)
			},
			subject:    "Reset your KailoPay password",
			action:     "Reset your password",
			expiration: "This link expires in 1 hour.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingSMTPClient{}
			sender, err := newGmailSenderForTest(testGmailConfig(), func(context.Context, GmailConfig) (smtpClient, error) {
				return client, nil
			})
			if err != nil {
				t.Fatalf("newGmailSenderForTest() error = %v", err)
			}

			if err := tt.send(sender); err != nil {
				t.Fatalf("send() error = %v", err)
			}

			message := client.message.String()
			for _, expected := range []string{
				"Subject: " + tt.subject,
				"Content-Type: multipart/related;",
				"Content-Type: text/plain; charset=UTF-8",
				"Content-Type: text/html; charset=UTF-8",
				"Content-Type: image/png; name=\"kailopay-logo.png\"",
				"Content-Id: <kailopay-logo>",
				"cid:kailopay-logo",
				tt.action,
				tt.expiration,
				link,
				"#5362ff",
			} {
				if !strings.Contains(message, expected) {
					t.Errorf("message does not contain %q: %q", expected, message)
				}
			}
		})
	}
}

func TestGmailSenderEscapesAuthenticationLinkInHTML(t *testing.T) {
	client := &recordingSMTPClient{}
	sender, err := newGmailSenderForTest(testGmailConfig(), func(context.Context, GmailConfig) (smtpClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("newGmailSenderForTest() error = %v", err)
	}

	link := "https://app.example.com/auth?token=a&next=<account>"
	if err := sender.SendEmailVerification(context.Background(), "user@example.com", link); err != nil {
		t.Fatalf("SendEmailVerification() error = %v", err)
	}

	message := client.message.String()
	if !strings.Contains(message, `href="https://app.example.com/auth?token=a&amp;next=&lt;account&gt;"`) {
		t.Fatalf("HTML link was not escaped: %q", message)
	}
	if !strings.Contains(message, link) {
		t.Fatalf("plain-text fallback lost the original link: %q", message)
	}
}

func TestGmailSenderMessageHasParseableInlineLogo(t *testing.T) {
	client := &recordingSMTPClient{}
	sender, err := newGmailSenderForTest(testGmailConfig(), func(context.Context, GmailConfig) (smtpClient, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("newGmailSenderForTest() error = %v", err)
	}
	if err := sender.SendEmailVerification(context.Background(), "user@example.com", "https://app.example.com/verify?token=abc"); err != nil {
		t.Fatalf("SendEmailVerification() error = %v", err)
	}

	message, err := stdmail.ReadMessage(strings.NewReader(client.message.String()))
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "multipart/related" {
		t.Fatalf("root media type = %q, want multipart/related", mediaType)
	}

	related := multipart.NewReader(message.Body, params["boundary"])
	alternativePart, err := related.NextPart()
	if err != nil {
		t.Fatalf("reading alternative part = %v", err)
	}
	alternativeType, alternativeParams, err := mime.ParseMediaType(alternativePart.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parsing alternative part = %v", err)
	}
	if alternativeType != "multipart/alternative" {
		t.Fatalf("alternative media type = %q, want multipart/alternative", alternativeType)
	}
	alternative := multipart.NewReader(alternativePart, alternativeParams["boundary"])
	plainPart, err := alternative.NextPart()
	if err != nil {
		t.Fatalf("reading plain-text part = %v", err)
	}
	if plainPart.Header.Get("Content-Type") != "text/plain; charset=UTF-8" {
		t.Fatalf("plain-text content type = %q", plainPart.Header.Get("Content-Type"))
	}
	htmlPart, err := alternative.NextPart()
	if err != nil {
		t.Fatalf("reading HTML part = %v", err)
	}
	if htmlPart.Header.Get("Content-Type") != "text/html; charset=UTF-8" {
		t.Fatalf("HTML content type = %q", htmlPart.Header.Get("Content-Type"))
	}
	if _, err := io.ReadAll(plainPart); err != nil {
		t.Fatalf("reading plain-text body = %v", err)
	}
	if _, err := io.ReadAll(htmlPart); err != nil {
		t.Fatalf("reading HTML body = %v", err)
	}

	logoPart, err := related.NextPart()
	if err != nil {
		t.Fatalf("reading logo part = %v", err)
	}
	if got := logoPart.Header.Get("Content-ID"); got != "<kailopay-logo>" {
		t.Fatalf("logo content ID = %q", got)
	}
	encodedLogo, err := io.ReadAll(logoPart)
	if err != nil {
		t.Fatalf("reading logo body = %v", err)
	}
	decodedLogo, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encodedLogo)), ""))
	if err != nil {
		t.Fatalf("decoding logo body = %v", err)
	}
	if !bytes.Equal(decodedLogo, kailoPayLogo) {
		t.Fatal("inline logo does not match the embedded KailoPay asset")
	}
}

func testGmailConfig() GmailConfig {
	return GmailConfig{
		Username:    "sender@gmail.com",
		AppPassword: "abcdefghijklmnop",
		FromName:    "KailoPay",
		Timeout:     time.Second,
	}
}
