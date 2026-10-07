package services

import (
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1494: the services layer reads "now" from SetClock/Now, so every
// expiry / window decision below is pinned at an exact instant on a
// clock.Fake — 2030, far from the real wall clock, so a call that bypasses the
// seam fails instead of passing by coincidence. None of these tests sleeps.

var svcT0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func useFakeClock(t *testing.T, at time.Time) *clock.Fake {
	t.Helper()
	clk := clock.NewFake(at)
	t.Cleanup(SetClock(clk))
	return clk
}

func TestSetClock_NowAndRestore(t *testing.T) {
	assert.WithinDuration(t, time.Now(), Now(), time.Second, "default is the wall clock")

	clk := clock.NewFake(svcT0)
	restore := SetClock(clk)
	assert.True(t, Now().Equal(svcT0))
	clk.Advance(time.Hour)
	assert.True(t, Now().Equal(svcT0.Add(time.Hour)))

	restore()
	assert.WithinDuration(t, time.Now(), Now(), time.Second, "restore puts the previous (system) clock back")

	// A nil Clock means the system clock; nested restores unwind in order.
	outer := SetClock(clock.NewFake(svcT0))
	inner := SetClock(nil)
	assert.WithinDuration(t, time.Now(), Now(), time.Second)
	inner()
	assert.True(t, Now().Equal(svcT0))
	outer()
}

// resetServiceClock simulates a fresh process (no clock ever stored) for the
// test and puts the real state back afterwards.
func resetServiceClock(t *testing.T) {
	t.Helper()
	prev := serviceClock.Load()
	serviceClock = atomic.Value{}
	t.Cleanup(func() {
		serviceClock = atomic.Value{}
		if prev != nil {
			serviceClock.Store(prev)
		}
	})
}

func TestSetClock_RestoreWithNothingPreviouslyInstalled(t *testing.T) {
	resetServiceClock(t)
	restore := SetClock(clock.NewFake(svcT0))
	assert.True(t, Now().Equal(svcT0))
	restore()
	assert.WithinDuration(t, time.Now(), Now(), time.Second)
}

func TestNow_BeforeAnyClockInstalled(t *testing.T) {
	resetServiceClock(t)
	assert.WithinDuration(t, time.Now(), Now(), time.Second)
}

func parseUnvalidated(t *testing.T, tok string) jwt.MapClaims {
	t.Helper()
	parsed, err := jwt.NewParser(jwt.WithoutClaimsValidation()).Parse(tok, func(*jwt.Token) (any, error) {
		return []byte("test-secret-key-32-chars-minimum!"), nil
	})
	require.NoError(t, err)
	return parsed.Claims.(jwt.MapClaims)
}

func TestGenerateToken_ClaimsStampedFromClock(t *testing.T) {
	useFakeClock(t, svcT0)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", JWTExpiryHours: 3}

	tok, err := GenerateToken(models.User{Username: "u"}, cfg, "sid-1")
	require.NoError(t, err)
	claims := parseUnvalidated(t, tok)
	assert.EqualValues(t, svcT0.Unix(), claims["iat"])
	assert.EqualValues(t, svcT0.Add(3*time.Hour).Unix(), claims["exp"])
}

func TestSessionIDFromToken_ExpiryIsNotADisqualifier(t *testing.T) {
	clk := useFakeClock(t, svcT0)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", JWTExpiryHours: 1}
	tok, err := GenerateToken(models.User{Username: "u"}, cfg, "sid-keep")
	require.NoError(t, err)

	assert.Equal(t, "sid-keep", SessionIDFromToken(tok, cfg))

	clk.Set(svcT0.Add(time.Hour)) // exactly exp: expired, but still names a real row
	assert.Equal(t, "sid-keep", SessionIDFromToken(tok, cfg))
	clk.Set(svcT0.Add(30 * 24 * time.Hour))
	assert.Equal(t, "sid-keep", SessionIDFromToken(tok, cfg))

	// A token issued in the clock's future (iat) is not trusted.
	clk.Set(svcT0.Add(-time.Hour))
	assert.Equal(t, "", SessionIDFromToken(tok, cfg))
	// Garbage still yields "".
	assert.Equal(t, "", SessionIDFromToken("not-a-jwt", cfg))
}

func TestSessionLifecycle_StampedFromClock(t *testing.T) {
	db := dbtest.New(t)
	clk := useFakeClock(t, svcT0)
	user := models.User{Username: "sess", Email: "sess@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", JWTExpiryHours: 2}

	sid, err := CreateSession(db, user.ID, cfg, "ua", "1.2.3.4")
	require.NoError(t, err)
	var s models.Session
	require.NoError(t, db.First(&s, "id = ?", sid).Error)
	assert.True(t, s.CreatedAt.Equal(svcT0))
	assert.True(t, s.LastSeenAt.Equal(svcT0))
	assert.True(t, s.ExpiresAt.Equal(svcT0.Add(2*time.Hour)))

	clk.Advance(30 * time.Minute)
	require.NoError(t, RevokeSession(db, sid))
	require.NoError(t, db.First(&s, "id = ?", sid).Error)
	require.NotNil(t, s.RevokedAt)
	assert.True(t, s.RevokedAt.Equal(svcT0.Add(30*time.Minute)))

	sid2, err := CreateSession(db, user.ID, cfg, "ua", "1.2.3.4")
	require.NoError(t, err)
	clk.Advance(time.Minute)
	n, err := RevokeAllSessions(db, user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the still-active session is revoked")
	var s2 models.Session
	require.NoError(t, db.First(&s2, "id = ?", sid2).Error)
	require.NotNil(t, s2.RevokedAt)
	assert.True(t, s2.RevokedAt.Equal(svcT0.Add(31*time.Minute)))
}

func TestIssueSession_TokenAndRowShareTheClockInstant(t *testing.T) {
	db := dbtest.New(t)
	useFakeClock(t, svcT0)
	user := models.User{Username: "issue", Email: "issue@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", JWTExpiryHours: 4}

	tok, err := IssueSession(db, user, cfg, "ua", "ip")
	require.NoError(t, err)
	claims := parseUnvalidated(t, tok)
	var s models.Session
	require.NoError(t, db.First(&s, "id = ?", claims["sid"]).Error)
	assert.EqualValues(t, s.ExpiresAt.Unix(), claims["exp"], "the row ceiling equals the token's exp")
}

// PurgeExpiredSessions: a row is purged strictly after expires_at, and a
// revoked row strictly after the grace window — kept at the exact boundary.
func TestPurgeExpiredSessions_BoundaryOnClock(t *testing.T) {
	db := dbtest.New(t)
	useFakeClock(t, svcT0)
	user := models.User{Username: "purge-sess", Email: "ps@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	mk := func(id string, expires time.Time, revoked *time.Time) {
		require.NoError(t, db.Create(&models.Session{
			ID: id, UserID: user.ID, CreatedAt: svcT0.Add(-100 * time.Hour), LastSeenAt: svcT0.Add(-100 * time.Hour),
			ExpiresAt: expires, RevokedAt: revoked,
		}).Error)
	}
	future := svcT0.Add(time.Hour)
	graceEdge := svcT0.Add(-sessionRevokedGrace)
	graceOver := graceEdge.Add(-time.Nanosecond)
	mk("exp-at-now", svcT0, nil)
	mk("exp-before", svcT0.Add(-time.Nanosecond), nil)
	mk("rev-at-edge", future, &graceEdge)
	mk("rev-over-edge", future, &graceOver)
	mk("active", future, nil)

	require.NoError(t, PurgeExpiredSessions(db))

	var ids []string
	require.NoError(t, db.Model(&models.Session{}).Order("id").Pluck("id", &ids).Error)
	assert.Equal(t, []string{"active", "exp-at-now", "rev-at-edge"}, ids)
}

func TestApiTokenAndPasswordResetStampedFromClock(t *testing.T) {
	db := dbtest.New(t)
	useFakeClock(t, svcT0)
	user := models.User{Username: "tokrev", Email: "tr@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.ApiToken{UserID: user.ID, Name: "t", TokenHash: "h1"}).Error)

	n, err := RevokeAllAPITokens(db, user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	var tok models.ApiToken
	require.NoError(t, db.First(&tok).Error)
	require.NotNil(t, tok.RevokedAt)
	assert.True(t, tok.RevokedAt.Equal(svcT0))

	assert.True(t, PasswordResetExpiry().Equal(svcT0.Add(time.Hour)))
	assert.True(t, PasswordResetExpiryFrom(svcT0.Add(5*time.Minute)).Equal(svcT0.Add(65*time.Minute)))
}

// The 2FA challenge token is valid until its exp second begins (10 min).
func TestChallengeToken_ExpiryBoundaryOnClock(t *testing.T) {
	clk := useFakeClock(t, svcT0)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!"}
	user := models.User{Username: "u2fa"}
	user.ID = 7
	tok, err := Generate2FAChallengeToken(user, cfg)
	require.NoError(t, err)

	clk.Set(svcT0.Add(twoFactorChallengeTTL - time.Nanosecond))
	id, name, ok := Parse2FAChallengeToken(tok, cfg)
	assert.True(t, ok)
	assert.EqualValues(t, 7, id)
	assert.Equal(t, "u2fa", name)

	clk.Set(svcT0.Add(twoFactorChallengeTTL))
	_, _, ok = Parse2FAChallengeToken(tok, cfg)
	assert.False(t, ok, "expired exactly at exp")
}

// TOTP accepts the current 30s step plus one either side, judged on the clock.
func TestTOTP_StepWindowOnClock(t *testing.T) {
	clk := useFakeClock(t, svcT0)
	secret, _, err := GenerateTOTPSecret("clock@example.com")
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, svcT0)
	require.NoError(t, err)

	assert.True(t, ValidateTOTP(secret, code))
	step, ok := ValidateTOTPStep(secret, code)
	require.True(t, ok)
	assert.Equal(t, svcT0.Unix()/30, step)

	clk.Set(svcT0.Add(30 * time.Second)) // one step later: still inside the skew
	assert.True(t, ValidateTOTP(secret, code))
	clk.Set(svcT0.Add(60 * time.Second)) // two steps later: outside
	assert.False(t, ValidateTOTP(secret, code))
	_, ok = ValidateTOTPStep(secret, code)
	assert.False(t, ok)
	clk.Set(svcT0.Add(-30 * time.Second)) // one step earlier: inside
	assert.True(t, ValidateTOTP(secret, code))
}

func TestOIDCNativeExchangeCode_ExpiryBoundaryOnClock(t *testing.T) {
	clk := useFakeClock(t, svcT0)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!"}
	user := models.User{Username: "native"}
	user.ID = 9
	code, err := MintOIDCNativeExchangeCode(user, "challenge", cfg)
	require.NoError(t, err)

	clk.Set(svcT0.Add(oidcNativeExchangeTTL - time.Nanosecond))
	id, challenge, ok := ParseOIDCNativeExchangeCode(code, cfg)
	assert.True(t, ok)
	assert.EqualValues(t, 9, id)
	assert.Equal(t, "challenge", challenge)

	clk.Set(svcT0.Add(oidcNativeExchangeTTL))
	_, _, ok = ParseOIDCNativeExchangeCode(code, cfg)
	assert.False(t, ok, "expired exactly at exp")
}

// The job de-dup window (#526): a run exactly minInterval after the last is
// allowed; 1ns earlier it is suppressed.
func TestAcquireJobLock_MinIntervalBoundaryOnClock(t *testing.T) {
	db := dbtest.New(t)
	clk := useFakeClock(t, svcT0)
	const job = models.JobNameDailyReminders

	ok, err := acquireJobLock(db, job, time.Hour)
	require.NoError(t, err)
	require.True(t, ok, "first run acquires")
	require.NoError(t, releaseJobLock(db, job, true))

	clk.Set(svcT0.Add(time.Hour - time.Nanosecond))
	ok, err = acquireJobLock(db, job, time.Hour)
	require.NoError(t, err)
	assert.False(t, ok, "1ns inside the window is suppressed")

	clk.Set(svcT0.Add(time.Hour))
	ok, err = acquireJobLock(db, job, time.Hour)
	require.NoError(t, err)
	assert.True(t, ok, "exactly minInterval later runs")
}

// CalculateNextReminderTime: a reminder due exactly now counts as "past" (the
// base becomes today's midnight); one nanosecond in the future keeps its own
// remind-at as the base.
func TestCalculateNextReminderTime_NowBoundaryOnClock(t *testing.T) {
	now := time.Date(2030, 3, 15, 9, 30, 0, 0, time.UTC)
	useFakeClock(t, now)

	atNow := models.Reminder{RemindAt: now, Recurrence: "weekly"}
	assert.True(t, CalculateNextReminderTime(atNow).Equal(time.Date(2030, 3, 22, 0, 0, 0, 0, time.UTC)))

	future := models.Reminder{RemindAt: now.Add(time.Nanosecond), Recurrence: "weekly"}
	assert.True(t, CalculateNextReminderTime(future).Equal(now.Add(time.Nanosecond).AddDate(0, 0, 7)))
}

// Month-end and leap-day recurrence resolved against the clock's "today".
func TestCalculateNextReminderTime_MonthEndAndLeapDayOnClock(t *testing.T) {
	tests := []struct {
		name       string
		now        time.Time
		recurrence string
		remindAt   time.Time
		want       time.Time
	}{
		{"monthly from Jan 31 clamps to Feb 28", time.Date(2030, 1, 31, 8, 0, 0, 0, time.UTC), "monthly", time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 2, 28, 0, 0, 0, 0, time.UTC)},
		{"monthly from Jan 31 clamps to Feb 29 in a leap year", time.Date(2028, 1, 31, 8, 0, 0, 0, time.UTC), "monthly", time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)},
		{"yearly from Feb 29 lands on Mar 1", time.Date(2028, 2, 29, 8, 0, 0, 0, time.UTC), "yearly", time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2029, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"quarterly across year end", time.Date(2030, 11, 30, 23, 59, 59, 999_000_000, time.UTC), "quarterly", time.Date(2030, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2031, 2, 28, 0, 0, 0, 0, time.UTC)},
		{"six-months", time.Date(2030, 8, 31, 0, 0, 0, 0, time.UTC), "six-months", time.Date(2030, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2031, 2, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useFakeClock(t, tt.now)
			got := CalculateNextReminderTime(models.Reminder{RemindAt: tt.remindAt, Recurrence: tt.recurrence})
			assert.True(t, got.Equal(tt.want), "got %v want %v", got, tt.want)
		})
	}
}

// Retention purges anchor on the injected clock: a row exactly at the cutoff
// stays (the DELETE is "< cutoff"), one nanosecond older goes.
func TestPurgeExpiredIdempotencyKeys_CutoffBoundaryOnClock(t *testing.T) {
	db, uid := newIdempotencyPurgeDB(t)
	useFakeClock(t, svcT0)
	cutoff := svcT0.Add(-24 * time.Hour)
	for key, ts := range map[string]time.Time{"at-cutoff": cutoff, "older": cutoff.Add(-time.Nanosecond), "newer": cutoff.Add(time.Nanosecond)} {
		require.NoError(t, db.Create(&models.IdempotencyKey{
			UserID: uid, Key: key, Method: "POST", Path: "/p", RequestFingerprint: key,
			State: models.IdempotencyStateCompleted, ResponseStatus: 201, CreatedAt: ts, UpdatedAt: ts,
		}).Error)
	}

	require.NoError(t, PurgeExpiredIdempotencyKeys(db, config.Config{IdempotencyKeyRetentionHours: 24}))

	var survivors []string
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Order("idempotency_key").Pluck("idempotency_key", &survivors).Error)
	assert.Equal(t, []string{"at-cutoff", "newer"}, survivors)
}

// An in-memory import wizard session is retrievable through its ExpiresAt
// instant and gone strictly after; the post-confirm tombstone follows the same
// rule and is stamped sessionExpiry out from the clock.
func TestImportSession_ExpiryBoundaryOnClock(t *testing.T) {
	clk := useFakeClock(t, svcT0)
	m := NewImportSessionManager()
	expires := svcT0.Add(sessionExpiry)
	m.sessions["s1"] = &importSessionData{session: models.ImportSession{UserID: 1, ExpiresAt: expires}}

	clk.Set(expires)
	_, appErr := m.get("s1", 1)
	assert.Nil(t, appErr, "still valid at exactly ExpiresAt")

	clk.Set(expires.Add(time.Nanosecond))
	_, appErr = m.get("s1", 1)
	require.NotNil(t, appErr, "gone 1ns after ExpiresAt")
	assert.NotContains(t, m.sessions, "s1", "an expired session is evicted on read")

	// CleanupExpired uses the same comparison.
	m.sessions["s2"] = &importSessionData{session: models.ImportSession{UserID: 1, ExpiresAt: expires}}
	clk.Set(expires)
	m.CleanupExpired()
	assert.Contains(t, m.sessions, "s2", "kept at exactly ExpiresAt")
	clk.Set(expires.Add(time.Nanosecond))
	m.CleanupExpired()
	assert.NotContains(t, m.sessions, "s2")

	// Confirmed-result tombstone: stamped from the clock, replayable through
	// its expiry, evicted strictly after.
	clk.Set(svcT0)
	m.rememberConfirmed("c1", 1, models.ImportResult{})
	assert.True(t, m.confirmedResults["c1"].expiresAt.Equal(svcT0.Add(sessionExpiry)))
	clk.Set(svcT0.Add(sessionExpiry))
	_, ok := m.replayConfirmed("c1", 1)
	assert.True(t, ok, "replayable at exactly the tombstone expiry")
	clk.Set(svcT0.Add(sessionExpiry + time.Nanosecond))
	_, ok = m.replayConfirmed("c1", 1)
	assert.False(t, ok)
	assert.NotContains(t, m.confirmedResults, "c1", "an expired tombstone is evicted on read")

	m.confirmedResults["c2"] = confirmedImport{userID: 1, expiresAt: svcT0}
	m.CleanupExpired()
	assert.NotContains(t, m.confirmedResults, "c2")
}

// An Immich asset with no parseable timestamp is ordered at the clock's now.
func TestAssetOccurredAt_FallsBackToClock(t *testing.T) {
	useFakeClock(t, svcT0)
	assert.True(t, assetOccurredAt(&ImmichAsset{FileCreatedAt: "garbage", CreatedAt: "also garbage"}).Equal(svcT0))
	want := time.Date(2020, 5, 5, 5, 5, 5, 0, time.UTC)
	assert.True(t, assetOccurredAt(&ImmichAsset{CreatedAt: want.Format(time.RFC3339)}).Equal(want))
}
