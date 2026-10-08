package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1473: POST /password-reset/request must be indistinguishable
// (status, body, headers) for a known and an unknown email no matter what
// fails for the known account, otherwise a broken mail transport turns the
// endpoint into an account-existence oracle.

// faultEmailSendSeam mirrors services.faultEmailSend (unexported).
const faultEmailSendSeam = "services.email.send"

const resetKnownEmail = "known1473@example.com"

type resetResp struct {
	code   int
	body   string
	header http.Header
}

func resetHarness(t *testing.T, cfg *config.Config) (*gorm.DB, func(email string) resetResp) {
	t.Helper()
	return resetHarnessWithClock(t, cfg, nil)
}

// resetHarnessWithClock is resetHarness with the request clock fixed to clk
// (nil: the system clock). The clock middleware must be installed before the
// route is registered to apply to it.
func resetHarnessWithClock(t *testing.T, cfg *config.Config, clk clock.Clock) (*gorm.DB, func(email string) resetResp) {
	t.Helper()
	db, router := setupRouter(t)
	if clk != nil {
		withClock(router, clk)
	}
	hashed, err := services.HashPassword(strongPassword)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.User{Username: "known1473", Email: resetKnownEmail, Password: hashed}).Error)

	router.POST("/password-reset/request", func(c *gin.Context) {
		c.Set("validated", &models.PasswordResetRequestInput{Email: c.Query("email")})
		RequestPasswordReset(c, cfg)
	})
	return db, func(email string) resetResp {
		req, _ := http.NewRequest("POST", "/password-reset/request?email="+email, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return resetResp{w.Code, w.Body.String(), w.Header()}
	}
}

// assertIndistinguishable compares status, body and the deterministic response
// headers (request IDs / dates legitimately differ per request).
func assertIndistinguishable(t *testing.T, want, got resetResp, msg string) {
	t.Helper()
	assert.Equal(t, want.code, got.code, msg+": status")
	assert.Equal(t, want.body, got.body, msg+": body")
	for _, h := range []string{"Content-Type", "Content-Length", "Cache-Control"} {
		assert.Equal(t, want.header.Values(h), got.header.Values(h), msg+": header "+h)
	}
}

func armedMailCfg() *config.Config {
	return &config.Config{UseSMTP: true, SMTPHost: "127.0.0.1", SMTPPort: 2525, SMTPFromEmail: "noreply@example.com"}
}

func countResetFailureEvents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.SystemEvent{}).
		Where("event_type = ? AND component = ? AND operation = ?",
			models.SysEventIntegrationFailed, logger.ComponentEmail, "password_reset_request").
		Count(&n).Error)
	return n
}

func TestPasswordResetRequest_UnknownEmail_Baseline(t *testing.T) {
	_, do := resetHarness(t, &config.Config{})
	got := do("nobody1473@example.com")
	assert.Equal(t, http.StatusOK, got.code)
	assert.Contains(t, got.body, "If an account exists")
}

func TestPasswordResetRequest_KnownEmail_SendSucceeds_IdenticalToUnknown(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs: No email channel configured; password reset email not sent")
	faults.Reset()
	t.Cleanup(faults.Reset)
	// No mail channel configured: SendPasswordResetEmail is a successful no-op.
	db, do := resetHarness(t, &config.Config{})

	unknown := do("nobody1473@example.com")
	known := do(resetKnownEmail)
	assertIndistinguishable(t, unknown, known, "send ok")
	fireandforget.Wait() // the send runs in the background (issue #1554)
	assert.Zero(t, countResetFailureEvents(t, db))

	var u models.User
	require.NoError(t, db.Where("email = ?", resetKnownEmail).First(&u).Error)
	assert.NotNil(t, u.PasswordResetTokenHash, "token is still persisted for the known account")
}

func TestPasswordResetRequest_KnownEmail_SendFails_IdenticalToUnknown(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs: Failed to send email via SMTP, Failed to send password reset email")
	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmError(faultEmailSendSeam, errors.New("injected smtp outage"))

	db, do := resetHarness(t, armedMailCfg())

	unknown := do("nobody1473@example.com")
	known := do(resetKnownEmail)
	assert.Equal(t, http.StatusOK, known.code, "a mail failure must never surface as 5xx")
	assertIndistinguishable(t, unknown, known, "send fails")

	// The send runs in the background (issue #1554); drain it before asserting.
	fireandforget.Wait()

	// The operator still gets the signal.
	var ev models.SystemEvent
	require.NoError(t, db.Where("component = ? AND operation = ?", logger.ComponentEmail, "password_reset_request").First(&ev).Error)
	assert.Equal(t, models.SysEventIntegrationFailed, ev.EventType)
	assert.Equal(t, "failure", *ev.Result)
	assert.Equal(t, "stage=send_email", ev.Detail)
	assert.Contains(t, ev.Error, "injected smtp outage")
	require.NotNil(t, ev.UserID)

	// The unknown-email request must not have produced an event.
	assert.EqualValues(t, 1, countResetFailureEvents(t, db))
}

func TestPasswordResetRequest_KnownEmail_SaveFails_IdenticalToUnknown(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs: Failed to persist password reset token")
	faults.Reset()
	t.Cleanup(faults.Reset)
	db, do := resetHarness(t, &config.Config{})

	require.NoError(t, db.Exec(`CREATE TRIGGER fail_user_update_1473 BEFORE UPDATE ON users
		BEGIN SELECT RAISE(ABORT, 'forced save failure'); END`).Error)

	unknown := do("nobody1473@example.com")
	known := do(resetKnownEmail)
	assert.Equal(t, http.StatusOK, known.code, "a token-persist failure must not be distinguishable")
	assertIndistinguishable(t, unknown, known, "save fails")
	// The token write runs in the background (issues #1554/#1569); drain it.
	fireandforget.Wait()

	var ev models.SystemEvent
	require.NoError(t, db.Where("component = ? AND operation = ?", logger.ComponentEmail, "password_reset_request").First(&ev).Error)
	assert.Equal(t, "stage=persist_token", ev.Detail)
	assert.Contains(t, ev.Error, "forced save failure")
}

// A lookup error hits known and unknown emails identically (it happens before
// the account is resolved), so it is not an oracle and stays a 5xx.
func TestPasswordResetRequest_LookupFailure_UniformForKnownAndUnknown(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs: Failed to lookup user for password reset, Request error")
	db, do := resetHarness(t, &config.Config{})
	dbtest.HideTable(t, db, "users")

	known := do(resetKnownEmail)
	unknown := do("nobody1473@example.com")
	assert.GreaterOrEqual(t, known.code, 500)
	assert.Equal(t, known.code, unknown.code)
}

// Issue #1554: a slow mail transport must not make a known-email request
// slower than an unknown one, or latency becomes the account-existence oracle
// that #1473 removed from the body. The send runs off the request path, so the
// handler answers while the transport is still stuck.
func TestPasswordResetRequest_KnownEmail_SlowTransport_DoesNotBlockResponse(t *testing.T) {
	// The guard is off for the whole test (AllowWarnings has no per-message
	// filter): the seam logs its pause marker, and after the pause the real
	// send dials the unreachable armedMailCfg host and logs the SMTP failure
	// plus "Failed to send password reset email" from the background
	// goroutine. TestPasswordResetRequest_KnownEmail_SendFails_IdenticalToUnknown
	// pins that failure path, so nothing here goes unasserted.
	logtest.AllowWarnings(t, "the paused seam logs its marker, and the post-pause background send to the unreachable test SMTP host logs its failure")
	faults.Reset()
	t.Cleanup(faults.Reset)
	// A synchronous send would take at least slowTransport to answer; the
	// deadline is shorter, so it can only pass if the send is off the request
	// path. dbtest's cleanup drains the background send, so the pause is also
	// this test's whole extra runtime (keep it short: it runs on every PR).
	const slowTransport, deadline = 3 * time.Second, 2 * time.Second
	faults.ArmPause(faultEmailSendSeam, slowTransport)

	_, do := resetHarness(t, armedMailCfg())

	done := make(chan resetResp, 1)
	go func() { done <- do(resetKnownEmail) }()

	select {
	case got := <-done:
		assert.Equal(t, http.StatusOK, got.code)
		assert.Contains(t, got.body, "If an account exists")
	case <-time.After(deadline):
		t.Fatal("known-email reset request blocked on the mail transport (latency oracle, issue #1554)")
	}
}

// knownResetTokenHash reads the stored reset token hash for the known account
// ("" when none is stored).
func knownResetTokenHash(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var u models.User
	require.NoError(t, db.Where("email = ?", resetKnownEmail).First(&u).Error)
	if u.PasswordResetTokenHash == nil {
		return ""
	}
	return *u.PasswordResetTokenHash
}

// Issue #1569: one reset email per account per cooldown. Every send is armed
// to fail so each one that happens leaves exactly one send_email
// system_event, which counts the sends; the response stays identical
// whether the request is fresh, throttled or for an unknown email.
func TestPasswordResetRequest_PerAccountCooldown(t *testing.T) {
	logtest.AllowWarnings(t, "every send is armed to fail: the path logs Failed to send email via SMTP / Failed to send password reset email")
	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmError(faultEmailSendSeam, errors.New("injected smtp outage"))
	clk := clock.NewFake(ctrlT0)
	db, do := resetHarnessWithClock(t, armedMailCfg(), clk)

	unknown := do("nobody1569@example.com")
	first := do(resetKnownEmail)
	fireandforget.Wait()
	assertIndistinguishable(t, unknown, first, "fresh request")
	require.EqualValues(t, 1, countResetFailureEvents(t, db), "the first request sends")
	firstHash := knownResetTokenHash(t, db)
	require.NotEmpty(t, firstHash)

	// Inside the cooldown: same answer, no new token, no send.
	clk.Advance(time.Minute)
	throttled := do(resetKnownEmail)
	fireandforget.Wait()
	assertIndistinguishable(t, unknown, throttled, "throttled request")
	assert.EqualValues(t, 1, countResetFailureEvents(t, db), "a request inside the cooldown must not send")
	assert.Equal(t, firstHash, knownResetTokenHash(t, db), "a throttled request must not rotate the token")

	// Past the cooldown: a new token and a new send.
	clk.Advance(time.Minute + time.Second)
	third := do(resetKnownEmail)
	fireandforget.Wait()
	assertIndistinguishable(t, unknown, third, "request after the cooldown")
	assert.EqualValues(t, 2, countResetFailureEvents(t, db), "the cooldown has passed, so this one sends")
	assert.NotEqual(t, firstHash, knownResetTokenHash(t, db), "the token rotates once the cooldown has passed")
}

// Concurrent requests for one account all pass the handler's early check
// (none has written yet); the background transaction re-checks under the
// write lock, so exactly one claims the reset and sends. The test holds
// SQLite's write lock from another connection while all requests are
// answered, so every handler really does read "not yet requested" and every
// background transaction queues behind the lock. Only the in-transaction
// re-check can then keep it to one send.
func TestPasswordResetRequest_ConcurrentRequestsSendOnce(t *testing.T) {
	logtest.AllowWarnings(t, "every send is armed to fail: the path logs Failed to send email via SMTP / Failed to send password reset email")
	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmError(faultEmailSendSeam, errors.New("injected smtp outage"))
	db, do := resetHarnessWithClock(t, armedMailCfg(), clock.NewFake(ctrlT0))

	ctx := context.Background()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	lock, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = lock.Close() }()
	_, err = lock.ExecContext(ctx, "BEGIN IMMEDIATE")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.Equal(t, http.StatusOK, do(resetKnownEmail).code)
		}()
	}
	wg.Wait() // every handler has answered; no background write could land yet

	_, err = lock.ExecContext(ctx, "COMMIT")
	require.NoError(t, err)
	fireandforget.Wait()
	assert.EqualValues(t, 1, countResetFailureEvents(t, db), "exactly one of the concurrent requests may send")
}
