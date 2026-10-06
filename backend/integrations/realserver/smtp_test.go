package realserver

import (
	"fmt"
	"net/url"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	envMailpitAPI  = "MYCORRHIZAL_RS_MAILPIT_API_URL"
	envMailpitSMTP = "MYCORRHIZAL_RS_MAILPIT_SMTP_ADDR" // host:port
)

type mailpitMessage struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
	From    struct{ Address string }
	To      []struct{ Address string }
}

// TestSMTP_SendEmailIsReceivedByRealServer sends through services.SendEmail's
// SMTP transport (EHLO / opportunistic STARTTLS / MAIL / RCPT / DATA with our
// hand-built RFC 5322 message) to a real SMTP server and reads the stored
// message back through its API: envelope + headers must be what we meant, the
// non-ASCII subject must survive Q-encoding, and the HTML body must arrive
// intact. The in-repo fake SMTP server only proves we speak the dialect we
// wrote it to expect.
func TestSMTP_SendEmailIsReceivedByRealServer(t *testing.T) {
	api := serverURL(t, envMailpitAPI)
	smtpAddr := requireEnv(t, envMailpitSMTP)
	waitReady(t, api+"/api/v1/info", 60*time.Second)

	u, err := url.Parse("smtp://" + smtpAddr)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	cfg := config.Config{UseSMTP: true, SMTPHost: u.Hostname(), SMTPPort: port, SMTPFromEmail: "noreply@mycorrhizal.test"}
	subject := fmt.Sprintf("Erinnerung für Zoë %d", time.Now().UnixNano())
	html := "<p>Hello — <b>contract</b> test</p>"
	to := fmt.Sprintf("rcpt-%d@example.com", time.Now().UnixNano())

	require.NoError(t, services.SendEmail(cfg, services.EmailMessage{To: to, Subject: subject, HTML: html}))

	var list struct {
		Messages []mailpitMessage `json:"messages"`
	}
	getJSON(t, api+"/api/v1/search?query="+url.QueryEscape("to:"+to), "", "", &list)
	require.Len(t, list.Messages, 1, "exactly one message must have been accepted for the recipient")
	m := list.Messages[0]
	assert.Equal(t, subject, m.Subject, "Q-encoded UTF-8 subject must decode")
	assert.Equal(t, "noreply@mycorrhizal.test", m.From.Address)
	require.Len(t, m.To, 1)
	assert.Equal(t, to, m.To[0].Address)

	var full struct {
		HTML        string `json:"HTML"`
		ContentType string
	}
	getJSON(t, api+"/api/v1/message/"+m.ID, "", "", &full)
	assert.Contains(t, full.HTML, "<b>contract</b>", "the HTML part must arrive and be parsed as HTML")
}
