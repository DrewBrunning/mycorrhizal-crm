package controllers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
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
	db, router := setupRouter(t)
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
