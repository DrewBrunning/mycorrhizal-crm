package services

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/faults"
	"mycorrhizal/logger"

	"github.com/resend/resend-go/v2"
)

// Outbound email is bounded by explicit timeouts on both transports (INT-02,
// issue #465). Before this, the Resend path inherited the SDK's 60s default and
// the SMTP path had no deadline at all — a hung MX could block a delivery
// goroutine indefinitely.
const (
	// resendRequestTimeout bounds a single Resend API call.
	resendRequestTimeout = 30 * time.Second
)

// smtpDialTimeout / smtpDeadline are vars (not consts) so a test can shrink
// them to keep the stalled-server assertion fast — same pattern as
// hibp_service.go's hibpAPIBaseURL / update_check.go's updateCheckCacheTTL.
var (
	// smtpDialTimeout bounds establishing the TCP (or TLS) connection.
	smtpDialTimeout = 15 * time.Second
	// smtpDeadline bounds the whole SMTP conversation once connected, so a
	// server that accepts the socket and then stalls mid-dialogue cannot hang
	// the send.
	smtpDeadline = 30 * time.Second
)

// faultEmailSend is the issue #434 failure-injection seam for the outbound
// email path. Unarmed it is a no-op map lookup; armed, the sentinel crosses
// both transports' entry points so SendEmail's best-effort / combined-error
// handling is what the injection test exercises.
const faultEmailSend = "services.email.send"

// EmailMessage is a transport-agnostic, already-rendered email ready for delivery.
type EmailMessage struct {
	To      string
	Subject string
	HTML    string
}

// SendEmail delivers msg through every configured channel (Resend and/or SMTP).
// Delivery is best-effort: returns nil if at least one configured channel
// succeeds, and a combined error only if all configured channels fail. If no
// channel is configured it logs a warning and returns nil (no-op)
func SendEmail(cfg config.Config, msg EmailMessage) error {
	if msg.To == "" {
		logger.Warn().Msg("Skipping email because recipient address is empty")
		return nil
	}

	if !cfg.EmailEnabled() {
		logger.Warn().Str("to", logger.MaskEmail(msg.To)).Msg("No email channel configured; email not sent")
		return nil
	}

	var (
		attempted int
		succeeded int
		errs      []string
	)

	if cfg.UseResend {
		attempted++
		if err := sendViaResend(cfg, msg); err != nil {
			logger.Error().Err(err).Str("to", logger.MaskEmail(msg.To)).Msg("Failed to send email via Resend")
			errs = append(errs, fmt.Sprintf("resend: %v", err))
		} else {
			succeeded++
		}
	}

	if cfg.UseSMTP {
		attempted++
		if err := sendViaSMTP(cfg, msg); err != nil {
			logger.Error().Err(err).Str("to", logger.MaskEmail(msg.To)).Msg("Failed to send email via SMTP")
			errs = append(errs, fmt.Sprintf("smtp: %v", err))
		} else {
			succeeded++
		}
	}

	if succeeded == 0 {
		return fmt.Errorf("all email channels failed (%d attempted): %s", attempted, strings.Join(errs, "; "))
	}

	if len(errs) > 0 {
		logger.Warn().Str("to", logger.MaskEmail(msg.To)).Int("succeeded", succeeded).Int("attempted", attempted).Msg("Email delivered on some but not all channels")
	}

	return nil
}

// resendHTTPClient is the explicit HTTP client for the Resend SDK. Extracted so
// a test can assert the timeout without a live call — the SDK's own default
// client is 60s (resend-go resend.go), which is not a value this project chose.
func resendHTTPClient() *http.Client {
	return &http.Client{Timeout: resendRequestTimeout}
}

// newResendClient builds the Resend SDK client with resendHTTPClient.
func newResendClient(apiKey string) *resend.Client {
	return resend.NewCustomClient(resendHTTPClient(), apiKey)
}

// delivers the message through the Resend API.
func sendViaResend(cfg config.Config, msg EmailMessage) error {
	if err := faults.Hook(faultEmailSend); err != nil {
		return err
	}

	client := newResendClient(cfg.ResendAPIKey)

	params := &resend.SendEmailRequest{
		From:    cfg.ResendFromEmail,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Html:    msg.HTML,
	}

	sent, err := client.Emails.Send(params)
	if err != nil {
		return err
	}

	logger.Info().Str("email_id", sent.Id).Str("to", logger.MaskEmail(msg.To)).Msg("Email sent via Resend")
	return nil
}

// delivers the message through an SMTP server
func sendViaSMTP(cfg config.Config, msg EmailMessage) error {
	if err := faults.Hook(faultEmailSend); err != nil {
		return err
	}

	addr := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	body := buildSMTPMessage(cfg.SMTPFromEmail, msg)

	var auth smtp.Auth
	if cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}

	if cfg.SMTPUseTLS {
		if err := sendSMTPImplicitTLS(cfg, addr, auth, msg.To, body); err != nil {
			return err
		}
	} else {
		if err := sendSMTPStartTLS(cfg, addr, auth, msg.To, body); err != nil {
			return err
		}
	}

	logger.Info().Str("to", logger.MaskEmail(msg.To)).Msg("Email sent via SMTP")
	return nil
}

// sendSMTPStartTLS is the plaintext-with-opportunistic-STARTTLS path (the
// default, typically port 587/25). It replaces net/smtp.SendMail so the dial
// and the conversation are both bounded: SendMail sets no deadline, so a
// stalled server hangs the caller forever. The STARTTLS-when-advertised and
// PlainAuth behavior is preserved exactly.
func sendSMTPStartTLS(cfg config.Config, addr string, auth smtp.Auth, to string, body []byte) error {
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpDeadline))

	client, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Hello(smtpHelloName()); err != nil {
		return fmt.Errorf("smtp hello: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: cfg.SMTPHost}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return smtpDeliver(client, cfg.SMTPFromEmail, to, body)
}

// sendSMTPImplicitTLS sends a message over a connection that is wrapped in TLS (implicit TLS, typically port 465).
func sendSMTPImplicitTLS(cfg config.Config, addr string, auth smtp.Auth, to string, body []byte) error {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: smtpDialTimeout}, "tcp", addr, &tls.Config{ServerName: cfg.SMTPHost})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpDeadline))

	client, err := smtp.NewClient(conn, cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	return smtpDeliver(client, cfg.SMTPFromEmail, to, body)
}

// smtpDeliver runs the MAIL/RCPT/DATA/QUIT tail shared by both SMTP paths.
func smtpDeliver(client *smtp.Client, from, to string, body []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}

	return client.Quit()
}

// smtpHelloName is the EHLO/HELO name. net/smtp defaults to "localhost" when
// Hello is never called; we call Hello explicitly (so STARTTLS negotiation
// happens before AUTH) and keep the same name.
func smtpHelloName() string { return "localhost" }

// buildSMTPMessage assembles a minimal RFC 5322 HTML email.
func buildSMTPMessage(from string, msg EmailMessage) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + msg.To + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", msg.Subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(msg.HTML)
	return []byte(b.String())
}
