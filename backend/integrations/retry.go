package integrations

import (
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// This file is INT-03 (issue #466): the retry-safety half of the integration
// surface. Dispositions() (matrix.go) already says whether a *failure mode* is
// transient or permanent; here we classify the *outbound operations* by whether
// repeating one after an ambiguous failure — the request left, the response
// never came back — is safe, and provide the shared backoff / Retry-After /
// status-classification primitives every retry loop should use instead of
// hand-rolling its own.

// IdempotencyClass says whether repeating an outbound operation after an
// ambiguous failure is safe, and what makes it safe (issue #466 action 1).
type IdempotencyClass string

const (
	// NaturallyIdempotent: repeating it converges with no extra guard — a PUT
	// to a known URL, a DELETE. Not used by any operation today, kept for the
	// classification to be complete.
	NaturallyIdempotent IdempotencyClass = "naturally-idempotent"
	// ConditionallyIdempotent: safe only with a precondition (`If-Match`) or a
	// client-supplied key the remote de-duplicates on.
	ConditionallyIdempotent IdempotencyClass = "conditionally-idempotent"
	// NotIdempotent: creates a new remote resource, or has a real-world side
	// effect (sends a message). A retry needs a local delivery record keyed by
	// the logical event to be recognized as a retry rather than a new one.
	NotIdempotent IdempotencyClass = "not-idempotent"
)

var validIdempotencyClass = map[IdempotencyClass]bool{
	NaturallyIdempotent: true, ConditionallyIdempotent: true, NotIdempotent: true,
}

// OutboundOperation is one write / side-effecting call this app makes to an
// external system, classified for retry safety. Reads (Ping, discovery, HIBP
// lookups, update checks) are not listed — there is nothing to double.
type OutboundOperation struct {
	ID          string // stable kebab-case slug
	Integration string // the Registry() Integration.ID it belongs to
	Summary     string // one sentence: what the call does
	Class       IdempotencyClass
	Safeguard   string // the concrete mechanism that makes a retry safe
	RetryPolicy string // attempts / backoff / where retry state lives / restart-survival, in words
	SourceRef   string // file:func that implements it
}

// knownOutboundWriteIntegrations is the curated set of Registry() IDs that
// perform a remote write or side effect and therefore must have at least one
// OutboundOperations() row. A new outbound write adds its integration here and
// a row below; TestOutboundOperationsCoverWriteIntegrations fails on a missing
// or dead entry (same discipline as nonIntegrationClients).
var knownOutboundWriteIntegrations = map[string]string{
	"webhooks":     "POSTs event payloads to operator-configured receivers",
	"caldav":       "PUTs locally edited events back when CALDAV_TWO_WAY_ENABLED",
	"ntfy":         "POSTs reminder notifications to a user ntfy topic",
	"gotify":       "POSTs reminder notifications to a user Gotify server",
	"webpush":      "POSTs reminder notifications to browser push endpoints",
	"email-resend": "sends reminder/notification email via the Resend API",
	"email-smtp":   "sends reminder/notification email via SMTP",
}

// OutboundOperations is the classified list, in the order the generated matrix
// renders them.
func OutboundOperations() []OutboundOperation {
	return []OutboundOperation{
		{
			ID:          "webhook-delivery",
			Integration: "webhooks",
			Summary:     "POST a subscribed event payload to an operator-configured receiver URL.",
			Class:       NotIdempotent,
			Safeguard: "Every attempt replays one immutable event envelope whose `id` (a UUID minted once) is sent as the `Idempotency-Key` request header, so a receiver can de-duplicate a retry; a retry never mints a new event. " +
				"A permanent HTTP status is not retried at all.",
			RetryPolicy: "webhookRetryPolicy — up to maxDeliveryAttempts (3), exponential backoff base 5m ×3 with ±20% jitter capped at 6h; 401/403/404/410 and other request-level 4xx go terminal immediately (failed_permanently, integration_failed event, no next_retry_at); 429/503 wait at least the Retry-After hint. next_retry_at is a column, so ProcessWebhookRetries re-scans after a restart — pending retries survive.",
			SourceRef:   "services/webhook_service.go:deliverWebhook / ProcessWebhookRetries",
		},
		{
			ID:          "caldav-push-event",
			Integration: "caldav",
			Summary:     "PUT a locally edited activity back to the subscribed remote calendar (two-way sync, CALDAV_TWO_WAY_ENABLED, default off).",
			Class:       ConditionallyIdempotent,
			Safeguard:   "The PUT carries the remote object's own UID and an `If-Match` ETag precondition (putCalendarObject), so a replay updates the same object in place and cannot create a duplicate. A 412 means the remote moved on and is resolved by the local-wins conflict policy, not retried blindly.",
			RetryPolicy: "No in-call retry loop. The next scheduled calendar_sync run re-pushes while link.ContentHash still shows an unsent local edit; bounded by the job lock and, on a permanent failure, the subscription's terminal-failure state (#467).",
			SourceRef:   "services/calendar_sync_service.go:pushLocalEdits / putCalendarObject",
		},
		notificationOp("notification-ntfy", "ntfy", "ntfy topic"),
		notificationOp("notification-gotify", "gotify", "Gotify server"),
		notificationOp("notification-webpush", "webpush", "browser Web Push endpoint"),
		emailOp("email-send-resend", "email-resend", "the Resend HTTP API", "services/mailer.go:sendViaResend"),
		emailOp("email-send-smtp", "email-smtp", "SMTP", "services/mailer.go:sendViaSMTP"),
	}
}

func notificationOp(id, integration, dest string) OutboundOperation {
	return OutboundOperation{
		ID:          id,
		Integration: integration,
		Summary:     "POST a reminder notification to a user-configured " + dest + ".",
		Class:       NotIdempotent,
		Safeguard:   "A `sent` NotificationDelivery row for (reminder, channel) suppresses any further send for that pair; a `failed`/`pending` row leaves it due. A retry after an ambiguous send is recognized by that key and does not double-notify.",
		RetryPolicy: "No in-call retry. The reminder stays due for the channel and the next daily reminder run retries — that 24h cadence is itself the backoff. A Retry-After hint is respected where a run would otherwise retry sooner.",
		SourceRef:   "services/notification_service.go:postNotificationJSON / sendPushMessage",
	}
}

func emailOp(id, integration, via, src string) OutboundOperation {
	return OutboundOperation{
		ID:          id,
		Integration: integration,
		Summary:     "Send a reminder/notification email via " + via + ".",
		Class:       NotIdempotent,
		Safeguard:   "Per (reminder, channel='email') NotificationDelivery keying plus the legacy Reminder.EmailSent mirror: a reminder already emailed is skipped on the next run, so a retry after an ambiguous send cannot re-send the same digest. The two transports are backends of one best-effort SendEmail.",
		RetryPolicy: "No in-call retry. A failed send leaves the reminder due for the email channel; the next daily reminder run retries. No tight loop against a broken mail host.",
		SourceRef:   src,
	}
}

// RetryPolicy is a bounded exponential-backoff-with-jitter schedule. INT-03
// (issue #466 action 4): every retry loop needs a maximum attempt count,
// exponential backoff with jitter, and a terminal state — an unbounded retry
// against a permanently broken remote is a self-inflicted denial of service.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first try
	BaseDelay   time.Duration // wait before the first retry
	MaxDelay    time.Duration // ceiling on any single wait (0 = no ceiling)
	Multiplier  float64       // growth factor per attempt (clamped to >= 1)
	JitterFrac  float64       // ± this fraction of the computed delay, [0, 1)
}

// Backoff returns the wait before the given retry. attempt is 1-based: attempt
// 1 is the first retry (the wait after the initial try failed). The delay is
// BaseDelay * Multiplier^(attempt-1), capped at MaxDelay, then spread by
// ±JitterFrac. rnd may be nil for a deterministic (jitter-free) result.
func (p RetryPolicy) Backoff(attempt int, rnd *rand.Rand) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	mult := p.Multiplier
	if mult < 1 {
		mult = 1
	}
	d := float64(p.BaseDelay) * math.Pow(mult, float64(attempt-1))
	if p.MaxDelay > 0 && d > float64(p.MaxDelay) {
		d = float64(p.MaxDelay)
	}
	if rnd != nil && p.JitterFrac > 0 {
		d *= 1 + (rnd.Float64()*2-1)*p.JitterFrac // uniform in [-JitterFrac, +JitterFrac]
	}
	if d < 0 {
		d = 0
	}
	out := time.Duration(d)
	if p.MaxDelay > 0 && out > p.MaxDelay {
		out = p.MaxDelay
	}
	return out
}

// NextDelay is Backoff, but never shorter than a server-supplied Retry-After
// hint (issue #466 action 5 — ignoring it turns a rate limit into a ban).
// retryAfter <= 0 means "no hint".
func (p RetryPolicy) NextDelay(attempt int, retryAfter time.Duration, rnd *rand.Rand) time.Duration {
	d := p.Backoff(attempt, rnd)
	if retryAfter > d {
		return retryAfter
	}
	return d
}

// HasAttemptsLeft reports whether a retry is still within budget after the
// given number of attempts have already been made.
func (p RetryPolicy) HasAttemptsLeft(attemptsMade int) bool {
	return attemptsMade < p.MaxAttempts
}

// DispositionForHTTPStatus classifies a non-2xx HTTP status from an outbound
// call in the same transient/permanent terms as Dispositions(). It composes
// Dispositions() for the statuses that correspond to a FailureMode, and adds
// explicit permanent entries for the request-level 4xx a retry cannot fix
// (issue #466 action 6: "a 400, 401, 403, or 404 will not become a 200 by
// repetition"). An unrecognized 5xx is transient; an unrecognized 4xx is
// permanent (a 4xx is a client-side rejection by definition).
func DispositionForHTTPStatus(code int) Disposition {
	d := Dispositions()
	switch code {
	case http.StatusUnauthorized: // 401
		return d[FailureAuthExpiry]
	case http.StatusForbidden: // 403
		return d[FailureAuthzRevoked]
	case http.StatusNotFound, http.StatusGone: // 404, 410
		return d[FailureRemoteResourceDeleted]
	case http.StatusTooManyRequests, http.StatusServiceUnavailable: // 429, 503
		return d[FailureRateLimited]
	case http.StatusBadRequest, http.StatusMethodNotAllowed,
		http.StatusConflict, http.StatusUnprocessableEntity,
		http.StatusNotImplemented: // 400, 405, 409, 422, 501
		return Disposition{
			Persistence: PermanentUntilHuman, Retryable: false,
			Rationale: "the request itself is rejected; a retry sends the same bytes and gets the same answer — the fix is configuration, not timing.",
		}
	default:
		if code >= 500 {
			return Disposition{
				Persistence: Transient, Retryable: true,
				Rationale: "an unclassified 5xx is a server-side fault that may clear on its own; retry with bounded backoff.",
			}
		}
		return Disposition{
			Persistence: PermanentUntilHuman, Retryable: false,
			Rationale: "an unclassified 4xx is a client-side rejection; retrying the identical request will not change the outcome.",
		}
	}
}

// ClassifyHTTPStatus maps an HTTP status to the FailureMode it represents.
// ok is false for a 2xx, or for a failing status with no corresponding named
// mode (400/405/409/422 — a request-level rejection the matrix has no row for).
func ClassifyHTTPStatus(code int) (FailureMode, bool) {
	switch code {
	case http.StatusUnauthorized:
		return FailureAuthExpiry, true
	case http.StatusForbidden:
		return FailureAuthzRevoked, true
	case http.StatusNotFound, http.StatusGone:
		return FailureRemoteResourceDeleted, true
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return FailureTimeout, true
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return FailureRateLimited, true
	}
	return "", false
}

// ParseRetryAfter parses an HTTP Retry-After header value — either a
// non-negative integer number of seconds, or an HTTP-date. Returns the delay
// and true on success; 0 and false otherwise. A date already in the past
// yields (0, true): the hint was given, it just means "now".
func ParseRetryAfter(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}
