package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mycorrhizal/config"
	"mycorrhizal/httputil"
	"mycorrhizal/internal/faults"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxDeliveryAttempts = 3

// webhookGoroutines tracks the fire-and-forget goroutines TriggerWebhooksAsync
// launches and the per-delivery goroutines TriggerWebhooks spawns. Production
// never waits on it — webhook fan-out stays asynchronous — but a test can call
// WaitForWebhookGoroutines in cleanup so a leaked delivery goroutine's DB
// access cannot race t.TempDir() teardown ("TempDir RemoveAll cleanup:
// directory not empty", issue #703, follow-up to #264).
var webhookGoroutines = fireandforget.Run

// WaitForWebhookGoroutines blocks until every in-flight TriggerWebhooks* call
// and the deliveries it spawned have returned. Test-only.
func WaitForWebhookGoroutines() { fireandforget.Wait() }

// TriggerWebhooksAsync runs TriggerWebhooks in a tracked goroutine — the
// fire-and-forget entry point for job/handler code that must not block on
// webhook fan-out.
func TriggerWebhooksAsync(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint, eventType string, data interface{}) {
	webhookGoroutines(func() {
		TriggerWebhooks(ctx, db, cfg, userID, eventType, data)
	})
}

// Sentinels surfaced by the SSRF-guarded dialer. They are returned as ordinary
// dial errors and end up in the stored delivery record's Error field.
var (
	ErrWebhookUnreachable    = errors.New("webhook host could not be resolved")
	ErrWebhookPrivateAddress = errors.New("webhook URL resolves to a private or loopback address")
)

var (
	// deliveryClient is used when WEBHOOK_BLOCK_PRIVATE_URLS is off, which is
	// the default: self-hosted installs legitimately point webhooks at other
	// services on the same box or LAN, so no address filtering is applied.
	deliveryClient = &http.Client{Timeout: 15 * time.Second}

	// guardedDeliveryClient is used when WEBHOOK_BLOCK_PRIVATE_URLS is on. The
	// filtering lives in the dialer rather than in a pre-flight URL check
	// because every connection the transport opens — including ones for
	// redirect targets — must be validated. A pre-flight check alone is
	// bypassable two ways: a 302 to an internal address (the redirect target
	// was never checked) and DNS rebinding (the dial re-resolves and can get a
	// different answer than the check did).
	guardedDeliveryClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: httputil.SafeDialContext(ErrWebhookUnreachable, ErrWebhookPrivateAddress),
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}

	// semaphore limits concurrent outbound webhook HTTP calls
	deliverySem = make(chan struct{}, 10)
)

// clientFor picks the delivery client matching the configured SSRF policy.
func clientFor(cfg config.Config) *http.Client {
	if cfg.WebhookBlockPrivateURLs {
		return guardedDeliveryClient
	}
	return deliveryClient
}

var retryDelays = []time.Duration{5 * time.Minute, 15 * time.Minute}

type webhookPayload struct {
	ID        string      `json:"id"`
	Event     string      `json:"event"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

func buildPayloadBody(eventType string, data interface{}) ([]byte, error) {
	payload := webhookPayload{
		ID:        uuid.New().String(),
		Event:     eventType,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Data:      data,
	}
	return json.Marshal(payload)
}

// trimSuccessfulDeliveryPayload drops the entity body from a webhook delivery
// record once the delivery has succeeded. This is the issue #622 decision: a
// successful (2xx) delivery is never re-sent (NextRetryAt is nil), so the
// stored receipt only needs the event envelope — id, event, timestamp — not
// the full serialized entity that triggered it. Before this, a `contact.created`
// delivery left a plaintext copy of the whole contact record in
// webhook_deliveries.payload forever, and the table's only other bound was the
// retention window added alongside this. Failed/retrying rows keep the full
// body because ProcessWebhookRetries replays it verbatim.
//
// The webhook-deliveries API response (`toDeliveryResponse`) never exposes
// payload, so trimming is invisible to the UI; the retry loop only reads
// payload for rows that still have a retry scheduled, which successful rows
// never do.
//
// Fails safe: if the payload is not the known envelope shape (missing `data`),
// the full body is kept rather than risking a mangled receipt.
func trimSuccessfulDeliveryPayload(body []byte) []byte {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(body, &env); err != nil {
		return body
	}
	if _, ok := env["data"]; !ok {
		return body
	}
	delete(env, "data")
	trimmed, err := json.Marshal(env)
	if err != nil {
		return body
	}
	return trimmed
}

// isPrivateURL reports whether rawURL resolves to any non-public address. It
// is only a fast pre-flight so the stored delivery record carries a clear
// error; the authoritative check is the pinning dialer on
// guardedDeliveryClient, which also covers redirects and DNS rebinding.
//
// Fails closed: an unparseable URL or a failed lookup counts as private, since
// we cannot show it is safe.
func isPrivateURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return true
	}
	for _, ip := range ips {
		if !httputil.IsPublicIP(ip) {
			return true
		}
	}
	return false
}

// TriggerWebhooks fires webhooks for all active subscriptions matching eventType for the user.
// Runs each delivery in its own goroutine (non-blocking). The correlation ID
// on ctx (if any) rides along to the delivery so a webhook receiver's log can
// be tied back to the action that fired it (issue #425); pass
// context.Background() from a fire-and-forget path with no correlation ID.
func TriggerWebhooks(ctx context.Context, db *gorm.DB, cfg config.Config, userID uint, eventType string, data interface{}) {
	// The goroutines below outlive the caller (a handler returns as soon as
	// this function does), so detach from ctx's cancellation/deadline while
	// keeping its values — the correlation ID rides along, but the request
	// ending does not abort an in-flight delivery.
	deliveryCtx := context.WithoutCancel(ctx)

	var webhooks []models.Webhook
	if err := db.Where("user_id = ? AND is_active = ? AND deleted_at IS NULL", userID, true).Find(&webhooks).Error; err != nil {
		logger.Error().Err(err).Uint("user_id", userID).Msg("Failed to load webhooks for triggering")
		return
	}

	body, err := buildPayloadBody(eventType, data)
	if err != nil {
		logger.Error().Err(err).Str("event", eventType).Msg("Failed to build webhook payload")
		return
	}

	for _, wh := range webhooks {
		subscribed := false
		for _, e := range wh.Events {
			if e == eventType {
				subscribed = true
				break
			}
		}
		if !subscribed {
			continue
		}

		wh := wh
		webhookGoroutines(func() {
			deliverySem <- struct{}{}
			defer func() { <-deliverySem }()
			deliverWebhook(deliveryCtx, db, cfg, wh, eventType, body, 1)
		})
	}
}

// TestWebhookDelivery delivers a test payload directly to the given webhook, ignoring event subscriptions.
func TestWebhookDelivery(db *gorm.DB, cfg config.Config, wh models.Webhook) models.WebhookDelivery {
	testData := map[string]interface{}{
		"message": "This is a test webhook delivery from Mycorrhizal CRM.",
	}
	body, err := buildPayloadBody("test", testData)
	if err != nil {
		errStr := err.Error()
		d := models.WebhookDelivery{WebhookID: wh.ID, EventType: "test", Payload: "{}", Error: &errStr, Attempts: 1}
		db.Create(&d)
		return d
	}
	return deliverWebhook(context.Background(), db, cfg, wh, "test", body, 1)
}

func deliverWebhook(ctx context.Context, db *gorm.DB, cfg config.Config, wh models.Webhook, eventType string, body []byte, attempt int) models.WebhookDelivery {
	start := time.Now()
	finish := func(d models.WebhookDelivery, errMsg string) models.WebhookDelivery {
		// integration_failed once a delivery has exhausted its retries and is
		// still failing — the point at which an operator needs to know an
		// external integration is down (issue #424). Detail carries the event
		// type only, never the webhook URL (#424 non-goal).
		if errMsg != "" && attempt >= maxDeliveryAttempts {
			durMS := time.Since(start).Milliseconds()
			logger.Ctx(ctx).Warn().
				Str(logger.FieldEvent, models.SysEventIntegrationFailed).
				Str(logger.FieldComponent, logger.ComponentWebhook).
				Str(logger.FieldResult, logger.ResultFailure).
				Int64(logger.FieldDurationMS, durMS).
				Uint("webhook_id", wh.ID).
				Str(logger.FieldError, logger.SanitizeLogField(errMsg)).
				Msg("webhook delivery exhausted its retries")
			models.RecordSystemEvent(ctx, db, models.SystemEvent{
				EventType: models.SysEventIntegrationFailed, Component: logger.ComponentWebhook,
				Operation: eventType, Result: models.SysResult(logger.ResultFailure),
				DurationMS: &durMS, Error: errMsg,
				Detail: fmt.Sprintf("event=%s attempts=%d", eventType, attempt),
			})
		}
		return d
	}

	if cfg.WebhookBlockPrivateURLs && isPrivateURL(wh.URL) {
		errStr := "webhook URL resolves to a private or loopback address"
		return finish(saveDelivery(db, wh.ID, eventType, string(body), nil, &errStr, attempt, nil), errStr)
	}

	sig := computeSignature(wh.Secret, body)
	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(body))
	if err != nil {
		errStr := err.Error()
		return finish(saveDelivery(db, wh.ID, eventType, string(body), nil, &errStr, attempt, retryAt(attempt)), errStr)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	req.Header.Set("X-Mycorrhizal-Event", eventType)
	if id := logger.CorrelationID(ctx); id != "" {
		req.Header.Set("X-Correlation-ID", id)
	}

	// Issue #434 failure-injection seam: an armed fault takes the same path as
	// a transport error — a failed delivery row with the next retry scheduled,
	// bounded by maxDeliveryAttempts.
	if ferr := faults.Hook(faultWebhookDelivery); ferr != nil {
		errStr := ferr.Error()
		return finish(saveDelivery(db, wh.ID, eventType, string(body), nil, &errStr, attempt, retryAt(attempt)), errStr)
	}

	resp, err := clientFor(cfg).Do(req)
	if err != nil {
		errStr := err.Error()
		return finish(saveDelivery(db, wh.ID, eventType, string(body), nil, &errStr, attempt, retryAt(attempt)), errStr)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	statusCode := resp.StatusCode
	if statusCode >= 200 && statusCode < 300 {
		// A successful delivery is never re-sent, so the stored receipt only
		// needs the event envelope — not the full entity body that triggered
		// it (issue #622). Failed/retrying rows keep the full body because
		// ProcessWebhookRetries replays it.
		trimmed := trimSuccessfulDeliveryPayload(body)
		return saveDelivery(db, wh.ID, eventType, string(trimmed), &statusCode, nil, attempt, nil)
	}
	errStr := fmt.Sprintf("unexpected status %d", statusCode)
	return finish(saveDelivery(db, wh.ID, eventType, string(body), &statusCode, &errStr, attempt, retryAt(attempt)), errStr)
}

func computeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func retryAt(attempt int) *time.Time {
	if attempt > len(retryDelays) {
		return nil
	}
	t := time.Now().Add(retryDelays[attempt-1])
	return &t
}

func saveDelivery(db *gorm.DB, webhookID uint, eventType, payload string, statusCode *int, errMsg *string, attempts int, nextRetryAt *time.Time) models.WebhookDelivery {
	d := models.WebhookDelivery{
		WebhookID:   webhookID,
		EventType:   eventType,
		Payload:     payload,
		StatusCode:  statusCode,
		Error:       errMsg,
		Attempts:    attempts,
		NextRetryAt: nextRetryAt,
	}
	if err := db.Create(&d).Error; err != nil {
		logger.Error().Err(err).Uint("webhook_id", webhookID).Msg("Failed to save webhook delivery")
	}
	return d
}

// ProcessWebhookRetries retries failed deliveries where NextRetryAt <= now and Attempts < maxDeliveryAttempts.
// Guarded by a job lock (matching reminders/calendar sync) so multiple
// instances don't double-process the same retry window.
func ProcessWebhookRetries(db *gorm.DB, cfg config.Config) {
	ctx := logger.JobContext(models.JobNameWebhookRetries)
	// Shorter than the 5-minute cron cadence (main.go) so the lock doesn't
	// suppress every other tick, unlike reminders/calendar sync which run far
	// less often.
	const minInterval = 4 * time.Minute
	acquired, err := acquireJobLock(db, models.JobNameWebhookRetries, minInterval)
	if err != nil {
		logger.Error().Err(err).Msg("Error checking webhook retry job lock")
		return
	}
	if !acquired {
		logger.Info().Msg("Skipping webhook retry job - rate limited")
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameWebhookRetries, true); err != nil {
			logger.Error().Err(err).Msg("Error releasing webhook retry job lock")
		}
	}()

	now := time.Now()
	var deliveries []models.WebhookDelivery
	if err := db.Where("next_retry_at <= ? AND attempts < ? AND deleted_at IS NULL", now, maxDeliveryAttempts).Find(&deliveries).Error; err != nil {
		logger.Error().Err(err).Msg("Failed to load webhook deliveries for retry")
		return
	}

	for _, d := range deliveries {
		var wh models.Webhook
		if err := db.Where("id = ? AND is_active = ?", d.WebhookID, true).First(&wh).Error; err != nil {
			logger.Warn().Err(err).Uint("webhook_id", d.WebhookID).Msg("Webhook not found or inactive for retry")
			db.Model(&d).Update("next_retry_at", nil)
			continue
		}

		// Clear next_retry_at so it won't be picked up again until the new delivery is saved
		if err := db.Model(&d).Update("next_retry_at", nil).Error; err != nil {
			logger.Error().Err(err).Uint("delivery_id", d.ID).Msg("Failed to clear next_retry_at")
			continue
		}

		d, wh := d, wh
		go func() {
			deliverySem <- struct{}{}
			defer func() { <-deliverySem }()
			// Replay the original payload so retries share the same event ID and timestamp
			deliverWebhook(ctx, db, cfg, wh, d.EventType, []byte(d.Payload), d.Attempts+1)
		}()
	}
}
