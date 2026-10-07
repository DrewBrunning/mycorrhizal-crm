package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1494: time-dependent controller decisions pinned at exact instants on
// a clock.Fake installed on the router (the same seam routes.go installs the
// system clock through). 2030 is far from the real wall clock, so a handler
// that falls back to time.Now() fails these instead of passing by coincidence.

var ctrlT0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

// withClock installs clk on every request of router; call it before
// registering the routes under test.
func withClock(router *gin.Engine, clk clock.Clock) {
	router.Use(func(c *gin.Context) {
		clock.Install(c, clk)
		c.Next()
	})
}

// The reset token is stamped, and expires, on the request clock: still
// redeemable at exactly expires_at, rejected (and cleared) 1ns after.
func TestPasswordReset_ExpiryBoundaryOnClock(t *testing.T) {
	cfg := config.Config{FrontendURL: "http://localhost:3000"}
	db, router := setupRouter(t)
	clk := clock.NewFake(ctrlT0)
	withClock(router, clk)

	hashed, _ := services.HashPassword(strongPassword)
	user := models.User{Username: "resetclock", Email: "resetclock@example.com", Password: hashed}
	require.NoError(t, db.Create(&user).Error)

	router.POST("/password-reset/request", func(c *gin.Context) {
		c.Set("validated", &models.PasswordResetRequestInput{Email: "resetclock@example.com"})
		RequestPasswordReset(c, &cfg)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/password-reset/request", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var stored models.User
	require.NoError(t, db.First(&stored, user.ID).Error)
	require.NotNil(t, stored.PasswordResetRequestedAt)
	require.NotNil(t, stored.PasswordResetExpiresAt)
	assert.True(t, stored.PasswordResetRequestedAt.Equal(ctrlT0))
	assert.True(t, stored.PasswordResetExpiresAt.Equal(ctrlT0.Add(time.Hour)), "expires_at = %v", stored.PasswordResetExpiresAt)

	// Seed a known token so the confirm leg can redeem it.
	token, tokenHash, err := services.GeneratePasswordResetToken()
	require.NoError(t, err)
	expires := ctrlT0.Add(time.Hour)
	requested := ctrlT0
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_reset_token_hash":   tokenHash,
		"password_reset_expires_at":   expires,
		"password_reset_requested_at": requested,
	}).Error)

	router.POST("/password-reset/confirm", func(c *gin.Context) {
		c.Set("validated", &models.PasswordResetConfirmInput{Token: token, Password: strongPasswordAlt})
		ConfirmPasswordReset(c, &config.Config{})
	})
	confirm := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/password-reset/confirm", nil)
		router.ServeHTTP(w, req)
		return w.Code
	}

	clk.Set(expires.Add(time.Nanosecond))
	assert.NotEqual(t, http.StatusOK, confirm(), "1ns past expires_at is rejected")
	var cleared models.User
	require.NoError(t, db.First(&cleared, user.ID).Error)
	assert.Nil(t, cleared.PasswordResetTokenHash, "the expired token is cleared")

	// Re-arm and redeem at exactly expires_at.
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_reset_token_hash": tokenHash,
		"password_reset_expires_at": expires,
	}).Error)
	clk.Set(expires)
	assert.Equal(t, http.StatusOK, confirm(), "valid at exactly expires_at (rejection is strictly after)")
}

// GET /sessions lists a session until the instant it expires, on the clock.
func TestListSessions_ExpiryBoundaryOnClock(t *testing.T) {
	db, router := setupRouter(t)
	clk := clock.NewFake(ctrlT0)
	withClock(router, clk)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	require.NoError(t, db.Create(&models.Session{
		ID: "ends-soon", UserID: user.ID, CreatedAt: ctrlT0, LastSeenAt: ctrlT0, ExpiresAt: ctrlT0.Add(time.Hour),
	}).Error)
	router.GET("/sessions", ListSessions)

	count := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/sessions", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Sessions []map[string]any `json:"sessions"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return len(body.Sessions)
	}

	clk.Set(ctrlT0.Add(time.Hour - time.Nanosecond))
	assert.Equal(t, 1, count(), "listed 1ns before expiry")
	clk.Set(ctrlT0.Add(time.Hour))
	assert.Equal(t, 0, count(), "gone exactly at expiry")
}

// A created API token expires expiresInDays after the request clock's now, and
// revocation is stamped with it.
func TestApiToken_ExpiryAndRevokeStampedFromClock(t *testing.T) {
	db, router := setupRouter(t)
	clk := clock.NewFake(ctrlT0)
	withClock(router, clk)
	// Revocation goes through services.RevokeAllAPITokens, which reads the
	// services-layer clock; pin both seams to the same Fake.
	defer services.SetClock(clk)()

	days := 3
	router.POST("/tokens", func(c *gin.Context) {
		c.Set("validated", &models.ApiTokenInput{Name: "clocked", ExpiresInDays: &days})
		CreateApiToken(c)
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/tokens", strings.NewReader(""))
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var tok models.ApiToken
	require.NoError(t, db.Where("name = ?", "clocked").First(&tok).Error)
	require.NotNil(t, tok.ExpiresAt)
	assert.True(t, tok.ExpiresAt.Equal(ctrlT0.Add(72*time.Hour)), "expires_at = %v", tok.ExpiresAt)

	router.DELETE("/tokens", RevokeAllApiTokens)
	clk.Advance(time.Hour)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/tokens", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.NoError(t, db.First(&tok, tok.ID).Error)
	require.NotNil(t, tok.RevokedAt)
	assert.True(t, tok.RevokedAt.Equal(ctrlT0.Add(time.Hour)), "revoked_at = %v", tok.RevokedAt)
}

// A locked second-factor step reports retry_after_at relative to the request
// clock (the same instant a client countdown is anchored to).
func TestComplete2FALogin_LockedRetryAfterAtOnClock(t *testing.T) {
	_, router := setupRouter(t)
	clk := clock.NewFake(ctrlT0)
	withClock(router, clk)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!"}

	const ip = "203.0.113.77"
	user := models.User{Username: "locked2fa"}
	user.ID = 5
	limiter := middleware.GetAccountRateLimiter()
	defer limiter.RecordLoginSuccess(user.Username, ip)
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		limiter.RecordLoginFailure(user.Username, ip)
	}

	challenge, err := services.Generate2FAChallengeToken(user, cfg)
	require.NoError(t, err)
	router.POST("/login/2fa", func(c *gin.Context) { Complete2FALogin(c, cfg) })

	req, _ := http.NewRequest("POST", "/login/2fa", strings.NewReader(`{"code":"123456"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "2fa_pending", Value: challenge})
	req.RemoteAddr = ip + ":4242"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	var body struct {
		RetryAfter   int    `json:"retry_after"`
		RetryAfterAt string `json:"retry_after_at"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Positive(t, body.RetryAfter)
	assert.Equal(t, ctrlT0.Add(time.Duration(body.RetryAfter)*time.Second).Format(time.RFC3339), body.RetryAfterAt)
}
