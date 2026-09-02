package services

import (
	"errors"
	"net"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/faults"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResendHTTPClient_HasExplicitTimeout pins the INT-02 (#465) finding fix:
// the Resend path no longer inherits the SDK's 60s default; it uses a client
// this project sized (resendRequestTimeout).
func TestResendHTTPClient_HasExplicitTimeout(t *testing.T) {
	assert.Equal(t, resendRequestTimeout, resendHTTPClient().Timeout)
	assert.NotZero(t, resendRequestTimeout)
}

// startStallingSMTPServer accepts one TCP connection and then never speaks —
// no 220 greeting, ever. net/smtp.SendMail would block on this forever; the
// bounded path must not.
func startStallingSMTPServer(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Hold the connection open, silent, until the test tears down.
		<-t.Context().Done()
		_ = conn.Close()
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

// TestSendViaSMTP_StalledServerIsBounded pins the INT-02 (#465) finding fix: a
// server that accepts the socket and then stalls mid-dialogue makes the send
// fail on smtpDeadline rather than hang the delivery goroutine.
func TestSendViaSMTP_StalledServerIsBounded(t *testing.T) {
	orig := smtpDeadline
	smtpDeadline = 300 * time.Millisecond
	t.Cleanup(func() { smtpDeadline = orig })

	host, port := startStallingSMTPServer(t)
	cfg := config.Config{SMTPHost: host, SMTPPort: port, SMTPFromEmail: "noreply@example.com"}

	done := make(chan error, 1)
	go func() {
		done <- sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	}()

	select {
	case err := <-done:
		require.Error(t, err, "a stalled server must produce an error, not a success")
	case <-time.After(5 * time.Second):
		t.Fatal("sendViaSMTP did not return — the SMTP conversation is not bounded by smtpDeadline")
	}
}

// TestSendViaSMTP_DialTimeoutIsBounded pins that the connect itself is bounded:
// dialing a routable-but-unreachable address returns within smtpDialTimeout.
func TestSendViaSMTP_DialTimeoutIsBounded(t *testing.T) {
	orig := smtpDialTimeout
	smtpDialTimeout = 300 * time.Millisecond
	t.Cleanup(func() { smtpDialTimeout = orig })

	// 198.51.100.0/24 (TEST-NET-2) is reserved and unroutable — a connect
	// there hangs until the dial timeout fires.
	cfg := config.Config{SMTPHost: "198.51.100.1", SMTPPort: 2525, SMTPFromEmail: "noreply@example.com"}

	done := make(chan error, 1)
	go func() {
		done <- sendViaSMTP(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	}()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("sendViaSMTP did not return — the connect is not bounded by smtpDialTimeout")
	}
}

// TestSendEmail_InjectedSendFaultSurfaces pins the services.email.send seam
// (issue #434): an armed fault crosses both transports' entry points and
// SendEmail's combined-error path reports it, without a swallow or panic.
func TestSendEmail_InjectedSendFaultSurfaces(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	sentinel := errors.New("injected email transport failure")
	faults.ArmError(faultEmailSend, sentinel)
	t.Cleanup(func() { faults.Disarm(faultEmailSend) })

	// SMTP-only, so exactly one channel is attempted and its failure is the
	// whole outcome.
	cfg := config.Config{UseSMTP: true, SMTPHost: "127.0.0.1", SMTPPort: 2525, SMTPFromEmail: "noreply@example.com"}

	err := SendEmail(cfg, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), sentinel.Error(), "the armed fault must reach the caller")
	assert.Contains(t, err.Error(), "all email channels failed")
}

// TestSendViaResend_InjectedFaultSurfaces pins the same seam on the Resend
// entry point in isolation.
func TestSendViaResend_InjectedFaultSurfaces(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	sentinel := errors.New("injected resend failure")
	faults.ArmError(faultEmailSend, sentinel)
	t.Cleanup(func() { faults.Disarm(faultEmailSend) })

	err := sendViaResend(config.Config{ResendAPIKey: "test"}, EmailMessage{To: "user@example.com", Subject: "hi", HTML: "<p>hi</p>"})
	require.ErrorIs(t, err, sentinel)
}
