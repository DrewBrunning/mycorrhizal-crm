package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

// Issue #1494: time-dependent middleware decisions pinned at exact instants on
// an injected clock.Fake. Every instant is far from the real wall clock
// (2030), so a regression that falls back to time.Now() fails these tests
// instead of passing by coincidence — and none of them sleeps.

var clockT0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

// clockAuthHarness is a real AuthMiddleware router on a Fake clock.
type clockAuthHarness struct {
	db     *gorm.DB
	router *gin.Engine
	clk    *clock.Fake
	user   models.User
}

func newClockAuthHarness(t *testing.T, idleHours int) *clockAuthHarness {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "clockauth", Email: "clockauth@example.com", Password: "password"}
	require.NoError(t, db.Create(&user).Error)

	h := &clockAuthHarness{db: db, clk: clock.NewFake(clockT0), user: user}
	cfg := &config.Config{JWTSecretKey: testJWTSecret, SessionIdleTimeoutHours: idleHours}
	h.router = gin.New()
	h.router.Use(func(c *gin.Context) {
		c.Set("db", db)
		clock.Install(c, h.clk)
		c.Next()
	})
	h.router.Use(AuthMiddleware(cfg))
	h.router.GET("/protected", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return h
}

func (h *clockAuthHarness) seedSession(t *testing.T, id string, lastSeen, expires time.Time) {
	t.Helper()
	require.NoError(t, h.db.Create(&models.Session{
		ID: id, UserID: h.user.ID, CreatedAt: clockT0, LastSeenAt: lastSeen, ExpiresAt: expires,
	}).Error)
}

func (h *clockAuthHarness) token(t *testing.T, sid string, extra jwt.MapClaims) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":       h.user.ID,
		"username":      h.user.Username,
		"token_version": h.user.TokenVersion,
		"sid":           sid,
		"exp":           clockT0.Add(48 * time.Hour).Unix(),
	}
	for k, v := range extra {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return signJWT(t, claims)
}

func (h *clockAuthHarness) status(tok string) int { return jwtRequest(h.router, tok).Code }

// A session row is valid at expires_at-1ns and expired at expires_at.
func TestAuthMiddleware_SessionExpiryBoundary(t *testing.T) {
	h := newClockAuthHarness(t, 0)
	expires := clockT0.Add(time.Hour)
	h.seedSession(t, "s-exp", clockT0, expires)
	tok := h.token(t, "s-exp", nil)

	h.clk.Set(expires.Add(-time.Nanosecond))
	assert.Equal(t, http.StatusOK, h.status(tok), "valid 1ns before expires_at")

	h.clk.Set(expires)
	assert.Equal(t, http.StatusUnauthorized, h.status(tok), "expired exactly at expires_at")
}

// Idle timeout: still valid when idle == the timeout exactly, expired 1ns past.
func TestAuthMiddleware_IdleTimeoutBoundary(t *testing.T) {
	h := newClockAuthHarness(t, 1) // 1h idle timeout
	// Two sessions: an accepted request touches last_seen_at in the
	// background, which would reset the idle clock for a reused session.
	h.seedSession(t, "s-idle-at", clockT0, clockT0.Add(24*time.Hour))
	h.seedSession(t, "s-idle-past", clockT0, clockT0.Add(24*time.Hour))

	h.clk.Set(clockT0.Add(time.Hour))
	assert.Equal(t, http.StatusOK, h.status(h.token(t, "s-idle-at", nil)), "idle == timeout is still valid (comparison is strictly greater)")

	h.clk.Set(clockT0.Add(time.Hour + time.Nanosecond))
	assert.Equal(t, http.StatusUnauthorized, h.status(h.token(t, "s-idle-past", nil)), "idle 1ns past the timeout is rejected")
}

// last_seen_at is only rewritten once sessionTouchInterval has passed, and is
// stamped with the injected clock's instant.
func TestAuthMiddleware_SessionTouchUsesClock(t *testing.T) {
	h := newClockAuthHarness(t, 0)
	h.seedSession(t, "s-touch", clockT0, clockT0.Add(24*time.Hour))
	tok := h.token(t, "s-touch", nil)

	lastSeen := func() time.Time {
		var s models.Session
		require.NoError(t, h.db.First(&s, "id = ?", "s-touch").Error)
		return s.LastSeenAt
	}

	h.clk.Set(clockT0.Add(sessionTouchInterval - time.Nanosecond))
	require.Equal(t, http.StatusOK, h.status(tok))
	// Not stale enough: nothing was queued, so the value is unchanged. (The
	// touch is a background goroutine; the positive case below polls.)
	assert.True(t, lastSeen().Equal(clockT0))

	touchAt := clockT0.Add(sessionTouchInterval)
	h.clk.Set(touchAt)
	require.Equal(t, http.StatusOK, h.status(tok))
	require.Eventually(t, func() bool { return lastSeen().Equal(touchAt) }, 2*time.Second, 5*time.Millisecond,
		"last_seen_at must be stamped with the injected clock")
}

// JWT exp is judged on the injected clock (whole unix seconds, like the jwt
// library): valid until the exp second begins, "Token expired" from then on.
func TestAuthMiddleware_JWTExpiryOnInjectedClock(t *testing.T) {
	h := newClockAuthHarness(t, 0)
	h.seedSession(t, "s-jwt", clockT0, clockT0.Add(72*time.Hour))
	exp := clockT0.Add(time.Hour)
	tok := h.token(t, "s-jwt", jwt.MapClaims{"exp": exp.Unix()})

	h.clk.Set(exp.Add(-time.Nanosecond))
	assert.Equal(t, http.StatusOK, h.status(tok), "valid 1ns before exp")

	h.clk.Set(exp)
	w := jwtRequest(h.router, tok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Token expired")
}

// iat in the clock's future and nbf not yet reached are rejected, evaluated on
// the injected clock too.
func TestAuthMiddleware_JWTIssuedAtAndNotBeforeOnInjectedClock(t *testing.T) {
	h := newClockAuthHarness(t, 0)
	h.seedSession(t, "s-iat", clockT0, clockT0.Add(72*time.Hour))

	future := h.token(t, "s-iat", jwt.MapClaims{"iat": clockT0.Add(time.Hour).Unix()})
	assert.Equal(t, http.StatusUnauthorized, h.status(future), "iat after the clock's now")
	h.clk.Advance(time.Hour)
	assert.Equal(t, http.StatusOK, h.status(future), "valid once the clock reaches iat")

	h.clk.Set(clockT0)
	nbf := h.token(t, "s-iat", jwt.MapClaims{"nbf": clockT0.Add(time.Minute).Unix()})
	assert.Equal(t, http.StatusUnauthorized, h.status(nbf), "nbf not yet reached")
	h.clk.Advance(time.Minute)
	assert.Equal(t, http.StatusOK, h.status(nbf))
}

func TestValidateTimeClaims(t *testing.T) {
	now := clockT0
	ok := func(c jwt.Claims) { t.Helper(); assert.NoError(t, ValidateTimeClaims(c, now)) }
	bad := func(c jwt.Claims, want error) {
		t.Helper()
		err := ValidateTimeClaims(c, now)
		require.Error(t, err)
		assert.ErrorIs(t, err, want)
	}
	ok(jwt.MapClaims{}) // no time claims at all is accepted, as the jwt library does
	ok(jwt.MapClaims{"exp": float64(now.Unix() + 1)})
	bad(jwt.MapClaims{"exp": float64(now.Unix())}, jwt.ErrTokenExpired)
	bad(jwt.MapClaims{"iat": float64(now.Unix() + 1)}, jwt.ErrTokenUsedBeforeIssued)
	bad(jwt.MapClaims{"nbf": float64(now.Unix() + 1)}, jwt.ErrTokenNotValidYet)
	ok(jwt.StandardClaims{ExpiresAt: 1}) // non-MapClaims are left to the caller
}

// An API token is valid up to (excluding) expires_at on the injected clock.
func TestLookupAPIToken_ExpiryBoundary(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tok", Email: "tok@example.com", Password: "password"}
	require.NoError(t, db.Create(&user).Error)
	raw := "mycorrhizal_boundary_token"
	expires := clockT0.Add(time.Hour)
	require.NoError(t, db.Create(&models.ApiToken{
		UserID: user.ID, Name: "t", TokenHash: hashToken(raw), Scope: "full",
		ExpiresAt: &expires,
	}).Error)

	_, ok := LookupAPIToken(db, raw, expires.Add(-time.Nanosecond))
	assert.True(t, ok, "valid 1ns before expires_at")
	_, ok = LookupAPIToken(db, raw, expires)
	assert.False(t, ok, "expired exactly at expires_at")
}

// The pending-key window is "<= idempotencyPendingTimeout" on the injected
// clock: in flight at exactly the timeout, terminal 1ns after.
func TestIdempotency_PendingTimeoutBoundaryOnClock(t *testing.T) {
	r, db, se, clk := idempotencyTestRouterWithClock(t, clockT0)
	var user models.User
	require.NoError(t, db.Where("username = ?", "idem").First(&user).Error)

	body := []byte(`{"n":1}`)
	require.NoError(t, db.Create(&models.IdempotencyKey{
		UserID: user.ID, Key: "pend", Method: "POST", Path: "/api/v1/things",
		RequestFingerprint: fingerprintRequest("POST", "/api/v1/things", body),
		State:              models.IdempotencyStatePending,
		CreatedAt:          clockT0, UpdatedAt: clockT0,
	}).Error)
	post := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader(body))
		req.Header.Set("Idempotency-Key", "pend")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	clk.Set(clockT0.Add(idempotencyPendingTimeout))
	w := post()
	require.Equal(t, http.StatusConflict, w.Code)
	assertIdempotencyErrorCode(t, w, "IDEMPOTENCY_IN_PROGRESS")

	clk.Set(clockT0.Add(idempotencyPendingTimeout + time.Nanosecond))
	w = post()
	require.Equal(t, http.StatusConflict, w.Code)
	assertIdempotencyErrorCode(t, w, "IDEMPOTENCY_RESULT_UNAVAILABLE")
	assert.EqualValues(t, 0, atomic.LoadInt64(se), "the handler never re-runs for an existing key")
}

// A freshly claimed key is stamped, and later completed, with the injected
// clock — not the wall clock.
func TestIdempotency_ClaimAndCompletionStampedWithClock(t *testing.T) {
	r, db, _, _ := idempotencyTestRouterWithClock(t, clockT0)

	req, _ := http.NewRequest("POST", "/api/v1/things", bytes.NewReader([]byte(`{"n":2}`)))
	req.Header.Set("Idempotency-Key", "stamp")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var row models.IdempotencyKey
	require.NoError(t, db.First(&row).Error)
	assert.True(t, row.CreatedAt.Equal(clockT0), "created_at = %v", row.CreatedAt)
	assert.True(t, row.UpdatedAt.Equal(clockT0), "updated_at = %v", row.UpdatedAt)
	assert.Equal(t, models.IdempotencyStateCompleted, row.State)
}

// --- rate limiter / lockout windows on a Fake clock ---------------------------

// The per-account lockout lasts BaseLockoutDuration, doubles with each further
// failure (capped), and is released exactly at LockedUntil.
func TestAccountRateLimiter_LockoutWindowOnClock(t *testing.T) {
	clk := clock.NewFake(clockT0)
	a := NewAccountRateLimiter(time.Hour)
	a.SetClock(clk)

	for i := 0; i < MaxLoginAttempts-1; i++ {
		locked, _ := a.RecordFailedAttempt("u")
		require.False(t, locked, "attempt %d must not lock yet", i+1)
	}
	locked, secs := a.RecordFailedAttempt("u")
	require.True(t, locked)
	assert.Equal(t, int(BaseLockoutDuration.Seconds()), secs)

	clk.Set(clockT0.Add(BaseLockoutDuration - time.Nanosecond))
	isLocked, remaining := a.IsLocked("u")
	assert.True(t, isLocked, "still locked 1ns before LockedUntil")
	assert.Equal(t, 0, remaining, "sub-second remainder truncates to 0s")

	clk.Set(clockT0.Add(BaseLockoutDuration))
	isLocked, _ = a.IsLocked("u")
	assert.False(t, isLocked, "released exactly at LockedUntil")

	// The next failure doubles the lockout, still measured from the clock.
	locked, secs = a.RecordFailedAttempt("u")
	require.True(t, locked)
	assert.Equal(t, int((2 * BaseLockoutDuration).Seconds()), secs)
}

// The login-key lockout, the global backstop and the known-good-IP exemption
// all read the limiter's clock.
func TestAccountRateLimiter_LoginLockoutAndKnownGoodTTLOnClock(t *testing.T) {
	clk := clock.NewFake(clockT0)
	a := NewAccountRateLimiter(time.Hour)
	a.SetClock(clk)

	for i := 0; i < MaxLoginAttempts; i++ {
		a.RecordLoginFailure("victim", "10.0.0.1")
	}
	locked, _ := a.IsLoginLocked("victim", "10.0.0.1")
	require.True(t, locked)
	clk.Advance(BaseLockoutDuration)
	locked, _ = a.IsLoginLocked("victim", "10.0.0.1")
	assert.False(t, locked, "per-pair lock released after BaseLockoutDuration on the clock")

	// RecordLoginSuccess stamps the known-good IP with the clock; the
	// exemption holds through KnownGoodIPTTL inclusive and not a nanosecond more.
	a.RecordLoginSuccess("victim", "10.0.0.2")
	seen := a.knownGoodIPs["victim"]["10.0.0.2"]
	assert.True(t, seen.Equal(clk.Now()))
	clk.Advance(KnownGoodIPTTL)
	a.mu.RLock()
	assert.True(t, a.knownGoodIPLocked("victim", "10.0.0.2", clk.Now()))
	assert.True(t, a.globalKnownGoodIPLocked("10.0.0.2", clk.Now()))
	a.mu.RUnlock()
	clk.Advance(time.Nanosecond)
	a.mu.RLock()
	assert.False(t, a.knownGoodIPLocked("victim", "10.0.0.2", clk.Now()))
	assert.False(t, a.globalKnownGoodIPLocked("10.0.0.2", clk.Now()))
	a.mu.RUnlock()
}

// CleanupStaleAccountEntries removes an entry only once its lockout has lapsed
// AND the last attempt is more than the TTL old.
func TestAccountRateLimiter_CleanupBoundaryOnClock(t *testing.T) {
	clk := clock.NewFake(clockT0)
	a := NewAccountRateLimiter(10 * time.Minute)
	a.SetClock(clk)
	a.RecordFailedAttempt("u") // LastAttempt = T0, not locked

	clk.Set(clockT0.Add(10 * time.Minute))
	a.CleanupStaleAccountEntries()
	assert.Equal(t, 1, a.EntryCount(), "entry survives at exactly the TTL (strictly greater is stale)")

	clk.Set(clockT0.Add(10*time.Minute + time.Nanosecond))
	a.CleanupStaleAccountEntries()
	assert.Equal(t, 0, a.EntryCount(), "entry removed 1ns past the TTL")
}

// The velocity tracker follows the limiter's clock (no real sleep needed to
// age its windows).
func TestAccountRateLimiter_VelocityFollowsClock(t *testing.T) {
	clk := clock.NewFake(clockT0)
	a := NewAccountRateLimiter(time.Hour)
	a.SetClock(clk)
	a.RecordLoginFailure("someone", "10.9.9.9")
	snap := a.AuthVelocity()
	assert.Equal(t, 1, snap.Failures)
	clk.Advance(24 * time.Hour)
	assert.Equal(t, 0, a.AuthVelocity().Failures, "failures age out of the window on the injected clock")
}

func TestConfigureAuthVelocity_UsesAccountLimiterClock(t *testing.T) {
	orig := accountLimiter.velocity
	origClk := accountLimiter.clk
	t.Cleanup(func() {
		accountLimiter.mu.Lock()
		accountLimiter.velocity = orig
		accountLimiter.clk = origClk
		accountLimiter.mu.Unlock()
	})
	clk := clock.NewFake(clockT0)
	accountLimiter.SetClock(clk)
	ConfigureAuthVelocity(DefaultAuthVelocityConfig())
	accountLimiter.RecordLoginFailure("cfg-velocity", "10.8.8.8")
	assert.Equal(t, 1, accountLimiter.AuthVelocity().Failures)
	clk.Advance(24 * time.Hour)
	assert.Equal(t, 0, accountLimiter.AuthVelocity().Failures)
}

// IP limiter entries expire by the injected clock: kept at exactly the TTL,
// dropped 1ns later, and a touch refreshes lastAccess.
func TestIPRateLimiter_CleanupBoundaryOnClock(t *testing.T) {
	clk := clock.NewFake(clockT0)
	l := NewIPRateLimiterWithTTL(rate.Every(time.Second), 10, time.Minute)
	l.SetClock(clk)
	l.GetLimiter("1.1.1.1")
	l.GetLimiter("2.2.2.2")

	clk.Advance(30 * time.Second)
	l.GetLimiter("2.2.2.2") // refresh lastAccess

	clk.Set(clockT0.Add(time.Minute))
	l.CleanupStaleEntries()
	assert.Equal(t, 2, l.EntryCount(), "nothing is stale at exactly the TTL")

	clk.Set(clockT0.Add(time.Minute + time.Nanosecond))
	l.CleanupStaleEntries()
	assert.Equal(t, 1, l.EntryCount(), "the untouched IP is dropped; the refreshed one stays")

	clk.Set(clockT0.Add(90*time.Second + time.Nanosecond))
	l.CleanupStaleEntries()
	assert.Equal(t, 0, l.EntryCount())
}

// A limiter with no clock set uses the real one (the production default).
func TestRateLimiters_DefaultToSystemClock(t *testing.T) {
	a := NewAccountRateLimiter(time.Hour)
	assert.WithinDuration(t, time.Now(), a.now(), time.Second)
	l := NewIPRateLimiter(rate.Every(time.Second), 1)
	assert.WithinDuration(t, time.Now(), l.now(), time.Second)
}

// The velocity tracker falls back to the system clock when handed no clock.
func TestNewAuthVelocityState_NilClockDefaultsToSystem(t *testing.T) {
	v := newAuthVelocityState(DefaultAuthVelocityConfig(), nil)
	assert.WithinDuration(t, time.Now(), v.now(), time.Second)
}
