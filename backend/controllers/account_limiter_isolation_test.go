package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateAccountLimiter gives the calling test a clean process-wide login
// limiter and guarantees the next one gets the same (issue #1492).
//
// middleware.GetAccountRateLimiter() is a package-level singleton keyed by
// username/IP. Several tests deliberately lock an account, and the env helpers
// derive a deterministic username from t.Name(), so under `-count=2` (or a
// shuffle that puts two tests on the same identifier) the second run starts
// locked and fails with 429/lockout instead of the response it is testing.
// Every helper that builds a router capable of reaching a login/2FA/WebAuthn
// handler calls this; tests that build their own router must call it too.
func isolateAccountLimiter(t testing.TB) {
	t.Helper()
	l := middleware.GetAccountRateLimiter()
	l.Reset()
	t.Cleanup(l.Reset)
}

// TestIsolateAccountLimiter_SecondUseStartsUnlocked is the regression test for
// the -count>1 hygiene gap: a test that locks an account must not leave it
// locked for the next test that builds an env with the same identifier.
func TestIsolateAccountLimiter_SecondUseStartsUnlocked(t *testing.T) {
	cfg := config.Config{JWTSecretKey: "mysecretkey", JWTExpiryHours: 24}
	const ip = "203.0.113.77"

	lockAccount := func(t *testing.T) {
		db, router := setupRouter(t)
		router.POST("/login", func(c *gin.Context) { LoginUser(c, &cfg) })
		u := models.User{Username: "iso_target", Email: "iso_target@example.com"}
		u.Password, _ = services.HashPassword(strongPassword)
		require.NoError(t, db.Create(&u).Error)
		for i := 0; i < middleware.MaxLoginAttempts-1; i++ {
			require.Equal(t, http.StatusUnauthorized, loginFrom(router, ip, "iso_target", "nope").Code)
		}
		require.Equal(t, http.StatusTooManyRequests, loginFrom(router, ip, "iso_target", "nope").Code)
		locked, _ := middleware.GetAccountRateLimiter().IsLoginLocked("iso_target", ip)
		require.True(t, locked, "precondition: the account is locked inside this subtest")
	}

	// Two back-to-back subtests, same identifier and IP: without isolation the
	// second one's first failed login would already be 429.
	t.Run("first", lockAccount)
	t.Run("second", lockAccount)

	// And nothing leaks out of the last subtest's cleanup.
	locked, _ := middleware.GetAccountRateLimiter().IsLoginLocked("iso_target", ip)
	assert.False(t, locked, "lockout must not outlive the test that caused it")
}
