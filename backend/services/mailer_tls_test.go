package services

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// selfSignedTLSConfig builds a throwaway cert for 127.0.0.1 so a fake SMTP
// server can offer real TLS to net/smtp (which pins ServerName). The client
// side uses InsecureSkipVerify only in the tests that need it.
func selfSignedTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

// fakeSMTPOpts configures the fake SMTP server's behavior.
type fakeSMTPOpts struct {
	advertiseSTARTTLS bool
	implicitTLS       bool
	failAt            string // "" | "MAIL FROM" | "DATA" — reply 5xx at that verb
}

// startConfigurableFakeSMTP speaks enough SMTP for net/smtp: EHLO (optionally
// advertising STARTTLS + AUTH), STARTTLS upgrade, AUTH PLAIN, MAIL/RCPT/DATA,
// QUIT. Serves exactly one connection.
func startConfigurableFakeSMTP(t *testing.T, opts fakeSMTPOpts) (host string, port int) {
	t.Helper()
	tlsCfg := selfSignedTLSConfig(t)

	var ln net.Listener
	var err error
	if opts.implicitTLS {
		ln, err = tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		serveFakeSMTP(conn, tlsCfg, opts)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

func serveFakeSMTP(conn net.Conn, tlsCfg *tls.Config, opts fakeSMTPOpts) {
	r := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }

	w("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			if opts.failAt == "EHLO" {
				w("500 command not recognized")
				continue
			}
			w("250-fake")
			if opts.advertiseSTARTTLS {
				w("250-STARTTLS")
			}
			w("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "STARTTLS"):
			w("220 go ahead")
			if opts.failAt == "STARTTLS" {
				_ = conn.Close() // client's TLS handshake fails
				return
			}
			tc := tls.Server(conn, tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			r = bufio.NewReader(conn)
			w = func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		case strings.HasPrefix(cmd, "AUTH"):
			if opts.failAt == "AUTH" {
				w("535 auth failed")
			} else {
				w("235 ok")
			}
		case strings.HasPrefix(cmd, "MAIL FROM"):
			if opts.failAt == "MAIL FROM" {
				w("550 no")
			} else {
				w("250 ok")
			}
		case strings.HasPrefix(cmd, "RCPT TO"):
			if opts.failAt == "RCPT TO" {
				w("550 no such user")
			} else {
				w("250 ok")
			}
		case strings.HasPrefix(cmd, "DATA"):
			if opts.failAt == "DATA" {
				w("554 no")
				continue
			}
			w("354 go")
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dl, "\r\n") == "." {
					break
				}
			}
			w("250 ok")
		case strings.HasPrefix(cmd, "QUIT"):
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

// trustSelfSignedSMTP swaps smtpTLSConfig for one that skips verification for
// the duration of a test — the fake server presents a throwaway cert.
func trustSelfSignedSMTP(t *testing.T) {
	t.Helper()
	orig := smtpTLSConfig
	smtpTLSConfig = func(serverName string) *tls.Config {
		return &tls.Config{ServerName: serverName, InsecureSkipVerify: true} //nolint:gosec // test-only fake cert
	}
	t.Cleanup(func() { smtpTLSConfig = orig })
}

// TestSendViaSMTP_STARTTLSNegotiated covers sendSMTPStartTLS's STARTTLS branch:
// when the server advertises it, the client upgrades before AUTH and completes.
func TestSendViaSMTP_STARTTLSNegotiated(t *testing.T) {
	trustSelfSignedSMTP(t)
	host, port := startConfigurableFakeSMTP(t, fakeSMTPOpts{advertiseSTARTTLS: true})
	cfg := config.Config{
		SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com",
		SMTPUsername: "u", SMTPPassword: "p",
	}
	err := sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	assert.NoError(t, err)
}

// TestSendViaSMTP_ImplicitTLSSuccess covers sendSMTPImplicitTLS's success path
// against a TLS listener.
func TestSendViaSMTP_ImplicitTLSSuccess(t *testing.T) {
	trustSelfSignedSMTP(t)
	host, port := startConfigurableFakeSMTP(t, fakeSMTPOpts{implicitTLS: true})
	cfg := config.Config{
		SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com",
		SMTPUseTLS: true, SMTPUsername: "u", SMTPPassword: "p",
	}
	err := sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	assert.NoError(t, err)
}

// TestSendViaSMTP_ImplicitTLSAuthRejected covers sendSMTPImplicitTLS's auth
// error branch.
func TestSendViaSMTP_ImplicitTLSAuthRejected(t *testing.T) {
	trustSelfSignedSMTP(t)
	host, port := startConfigurableFakeSMTP(t, fakeSMTPOpts{implicitTLS: true, failAt: "AUTH"})
	cfg := config.Config{
		SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com",
		SMTPUseTLS: true, SMTPUsername: "u", SMTPPassword: "p",
	}
	err := sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth")
}

// TestSendViaSMTP_ServerRejectsAtEachStep covers the error branch of every
// step in sendSMTPStartTLS / smtpDeliver.
func TestSendViaSMTP_ServerRejectsAtEachStep(t *testing.T) {
	trustSelfSignedSMTP(t)
	for _, tc := range []struct {
		failAt  string
		wantMsg string
	}{
		{"EHLO", "hello"},
		{"STARTTLS", "starttls"},
		{"AUTH", "auth"},
		{"MAIL FROM", "mail from"},
		{"RCPT TO", "rcpt to"},
		{"DATA", "data"},
	} {
		t.Run(tc.failAt, func(t *testing.T) {
			host, port := startConfigurableFakeSMTP(t, fakeSMTPOpts{advertiseSTARTTLS: true, failAt: tc.failAt})
			cfg := config.Config{
				SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com",
				SMTPUsername: "u", SMTPPassword: "p",
			}
			err := sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// TestSendViaResend_SuccessAndError points the resend-go SDK at a local server
// via RESEND_BASE_URL, covering newResendClient and sendViaResend's success and
// non-2xx paths.
func TestSendViaResend_SuccessAndError(t *testing.T) {
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "email_123"})
	}))
	t.Cleanup(srv.Close)
	orig := resendBaseURLOverride
	resendBaseURLOverride = srv.URL + "/"
	t.Cleanup(func() { resendBaseURLOverride = orig })

	cfg := config.Config{ResendAPIKey: "test", ResendFromEmail: "noreply@example.com"}

	status = http.StatusOK
	require.NoError(t, sendViaResend(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"}))

	status = http.StatusUnprocessableEntity
	require.Error(t, sendViaResend(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"}))
}

// TestSendEmail_PartialChannelFailure covers SendEmail's "delivered on some but
// not all channels" path: Resend fails, SMTP succeeds, overall success.
func TestSendEmail_PartialChannelFailure(t *testing.T) {
	trustSelfSignedSMTP(t)

	resendSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"down"}`))
	}))
	t.Cleanup(resendSrv.Close)
	orig := resendBaseURLOverride
	resendBaseURLOverride = resendSrv.URL + "/"
	t.Cleanup(func() { resendBaseURLOverride = orig })

	host, port := startConfigurableFakeSMTP(t, fakeSMTPOpts{advertiseSTARTTLS: true})

	cfg := config.Config{
		UseResend: true, ResendAPIKey: "test", ResendFromEmail: "noreply@example.com",
		UseSMTP: true, SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com",
	}
	buf := captureLoggerOutput(t)

	err := SendEmail(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	require.NoError(t, err, "one working channel is enough for overall success")
	assert.Contains(t, buf.String(), "some but not all")
}

// TestSendEmail_ResendOnlySuccess covers SendEmail's Resend success branch.
func TestSendEmail_ResendOnlySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "email_ok"})
	}))
	t.Cleanup(srv.Close)
	orig := resendBaseURLOverride
	resendBaseURLOverride = srv.URL + "/"
	t.Cleanup(func() { resendBaseURLOverride = orig })

	cfg := config.Config{UseResend: true, ResendAPIKey: "test", ResendFromEmail: "noreply@example.com"}
	require.NoError(t, SendEmail(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"}))
}
