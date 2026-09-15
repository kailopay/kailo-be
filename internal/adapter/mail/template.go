package mail

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	stdmail "net/mail"
	"net/textproto"
	"strings"
)

const (
	emailLogoContentID       = "kailopay-logo"
	emailLogoFilename        = "kailopay-logo.png"
	emailRelatedBoundary     = "kailopay-related-v1"
	emailAlternativeBoundary = "kailopay-alternative-v1"
)

//go:embed assets/kailopay-logo.png
var kailoPayLogo []byte

type authEmailCopy struct {
	heading    string
	intro      string
	action     string
	link       string
	expiration string
	warning    string
}

type renderedEmail struct {
	plainText string
	html      string
}

func renderAuthEmail(copy authEmailCopy) renderedEmail {
	return renderedEmail{
		plainText: renderAuthPlainText(copy),
		html:      renderAuthHTML(copy),
	}
}

func renderAuthPlainText(copy authEmailCopy) string {
	return strings.Join([]string{
		"Hello,",
		"",
		copy.heading,
		"",
		copy.intro,
		"",
		copy.action + ":",
		copy.link,
		"",
		copy.expiration,
		copy.warning,
		"",
		"KailoPay",
	}, "\r\n") + "\r\n"
}

func renderAuthHTML(copy authEmailCopy) string {
	escapedHeading := html.EscapeString(copy.heading)
	escapedIntro := html.EscapeString(copy.intro)
	escapedAction := html.EscapeString(copy.action)
	escapedLink := html.EscapeString(copy.link)
	escapedExpiration := html.EscapeString(copy.expiration)
	escapedWarning := html.EscapeString(copy.warning)

	var builder strings.Builder
	builder.WriteString(`<!doctype html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="x-apple-disable-message-reformatting">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>KailoPay</title>
<style>
@media screen and (max-width: 600px) {
  .email-frame { width: 100% !important; }
  .email-header, .email-content, .email-footer { padding-left: 22px !important; padding-right: 22px !important; }
  .email-header { padding-top: 24px !important; padding-bottom: 20px !important; }
  .email-content { padding-top: 32px !important; padding-bottom: 30px !important; }
  .email-footer { padding-top: 20px !important; padding-bottom: 24px !important; }
  .email-logo { width: 160px !important; }
  .email-title { font-size: 28px !important; }
}
</style>
</head>
<body style="margin:0;padding:0;background-color:#f4f6ff;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;background-color:#f4f6ff;">
  <tr>
    <td align="center" style="padding:32px 16px;">
      <table role="presentation" class="email-frame" width="600" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:600px;background-color:#ffffff;border:1px solid #e5e8f5;border-radius:20px;overflow:hidden;">
        <tr>
          <td style="height:7px;background-color:#5362ff;font-size:0;line-height:0;">&nbsp;</td>
        </tr>
        <tr>
          <td class="email-header" style="padding:28px 32px 23px;border-bottom:1px solid #eef0f8;background-color:#ffffff;">
            <img class="email-logo" src="cid:`)
	builder.WriteString(emailLogoContentID)
	builder.WriteString(`" alt="KailoPay" width="180" style="display:block;width:180px;max-width:100%;height:auto;border:0;">
          </td>
        </tr>
        <tr>
          <td class="email-content" style="padding:40px 32px 34px;">
			<h1 class="email-title" style="margin:0;color:#171a2d;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:32px;line-height:1.2;font-weight:700;letter-spacing:-0.02em;">`)
	builder.WriteString(escapedHeading)
	builder.WriteString(`</h1>
			<p style="margin:16px 0 0;color:#5d6479;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:16px;line-height:1.65;">`)
	builder.WriteString(escapedIntro)
	builder.WriteString(`</p>
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:30px 0 25px;">
              <tr>
				<td align="center" bgcolor="#5362ff" style="border-radius:10px;background-color:#5362ff;">
				  <a href="`)
	builder.WriteString(escapedLink)
	builder.WriteString(`" style="display:inline-block;padding:15px 23px;color:#ffffff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:15px;line-height:1;font-weight:700;text-decoration:none;">`)
	builder.WriteString(escapedAction)
	builder.WriteString(`</a>
                </td>
              </tr>
            </table>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;background-color:#f3f5ff;border:1px solid #e2e6ff;border-radius:12px;">
              <tr>
					<td style="padding:16px 18px;">
					  <p style="margin:0;color:#3f4770;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:13px;line-height:1.55;font-weight:700;">`)
	builder.WriteString(escapedExpiration)
	builder.WriteString(`</p>
                </td>
              </tr>
            </table>
			<p style="margin:22px 0 0;color:#737a8d;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:13px;line-height:1.6;">`)
	builder.WriteString(escapedWarning)
	builder.WriteString(`</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;margin-top:30px;border-top:1px solid #eef0f8;">
              <tr>
                <td style="padding-top:22px;">
				  <p style="margin:0;color:#737a8d;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:12px;line-height:1.6;">If the button does not work, copy and paste this link into your browser.</p>
				  <p style="margin:8px 0 0;color:#4654d9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:12px;line-height:1.6;word-break:break-all;">
					<a href="`)
	builder.WriteString(escapedLink)
	builder.WriteString(`" style="color:#4654d9;text-decoration:underline;word-break:break-all;">`)
	builder.WriteString(escapedLink)
	builder.WriteString(`</a>
                  </p>
                </td>
              </tr>
            </table>
          </td>
        </tr>
        <tr>
          <td class="email-footer" style="padding:22px 32px 26px;border-top:1px solid #eef0f8;background-color:#fafbff;">
			<p style="margin:0;color:#777e91;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:12px;line-height:1.5;font-weight:700;">KailoPay account security</p>
			<p style="margin:6px 0 0;color:#9aa0b1;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:12px;line-height:1.5;">You received this email because an action was requested for your account.</p>
          </td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>
`)
	return builder.String()
}

func buildMessage(config GmailConfig, recipient, subject string, content renderedEmail) (string, error) {
	var body bytes.Buffer
	related := multipart.NewWriter(&body)
	if err := related.SetBoundary(emailRelatedBoundary); err != nil {
		return "", fmt.Errorf("setting related MIME boundary: %w", err)
	}

	alternativeHeader := make(textproto.MIMEHeader)
	alternativeHeader.Set("Content-Type", fmt.Sprintf(
		`multipart/alternative; boundary="%s"`,
		emailAlternativeBoundary,
	))
	alternativePart, err := related.CreatePart(alternativeHeader)
	if err != nil {
		return "", fmt.Errorf("creating alternative MIME part: %w", err)
	}
	alternative := multipart.NewWriter(alternativePart)
	if err := alternative.SetBoundary(emailAlternativeBoundary); err != nil {
		return "", fmt.Errorf("setting alternative MIME boundary: %w", err)
	}

	plainHeader := make(textproto.MIMEHeader)
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainHeader.Set("Content-Transfer-Encoding", "8bit")
	plainPart, err := alternative.CreatePart(plainHeader)
	if err != nil {
		return "", fmt.Errorf("creating plain-text MIME part: %w", err)
	}
	if _, err := io.WriteString(plainPart, content.plainText); err != nil {
		return "", fmt.Errorf("writing plain-text MIME part: %w", err)
	}

	htmlHeader := make(textproto.MIMEHeader)
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHeader.Set("Content-Transfer-Encoding", "8bit")
	htmlPart, err := alternative.CreatePart(htmlHeader)
	if err != nil {
		return "", fmt.Errorf("creating HTML MIME part: %w", err)
	}
	if _, err := io.WriteString(htmlPart, content.html); err != nil {
		return "", fmt.Errorf("writing HTML MIME part: %w", err)
	}
	if err := alternative.Close(); err != nil {
		return "", fmt.Errorf("closing alternative MIME part: %w", err)
	}

	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Type", fmt.Sprintf(`image/png; name="%s"`, emailLogoFilename))
	imageHeader.Set("Content-Transfer-Encoding", "base64")
	imageHeader.Set("Content-ID", "<"+emailLogoContentID+">")
	imageHeader.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, emailLogoFilename))
	imagePart, err := related.CreatePart(imageHeader)
	if err != nil {
		return "", fmt.Errorf("creating logo MIME part: %w", err)
	}
	if err := writeMIMEBase64(imagePart, kailoPayLogo); err != nil {
		return "", fmt.Errorf("writing logo MIME part: %w", err)
	}
	if err := related.Close(); err != nil {
		return "", fmt.Errorf("closing related MIME part: %w", err)
	}

	fromAddress := (&stdmail.Address{Name: config.FromName, Address: config.Username}).String()
	toAddress := (&stdmail.Address{Address: recipient}).String()
	headers := []string{
		"From: " + fromAddress,
		"To: " + toAddress,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		fmt.Sprintf(
			`Content-Type: multipart/related; boundary="%s"; type="multipart/alternative"`,
			emailRelatedBoundary,
		),
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body.String(), nil
}

func writeMIMEBase64(writer io.Writer, data []byte) error {
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	base64.StdEncoding.Encode(encoded, data)
	for len(encoded) > 0 {
		lineLength := 76
		if len(encoded) < lineLength {
			lineLength = len(encoded)
		}
		if _, err := writer.Write(encoded[:lineLength]); err != nil {
			return err
		}
		if _, err := io.WriteString(writer, "\r\n"); err != nil {
			return err
		}
		encoded = encoded[lineLength:]
	}
	return nil
}
