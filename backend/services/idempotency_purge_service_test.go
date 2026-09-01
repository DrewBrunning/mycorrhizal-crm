package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// CON-04 (issue #459, ADR 0010): idempotency_keys rows are transient replay
// bookkeeping — a key protects one operation across its retries, which happen
// within seconds to minutes. PurgeExpiredIdempotencyKeys hard-deletes rows
// past IDEMPOTENCY_KEY_RETENTION_HOURS (default 24), anchored on created_at.
//
// Real migrated schema (dbtest), not AutoMigrate — the raw DELETE names the
// real idempotency_keys columns (CLAUDE.md backend trap #1).

func newIdempotencyPurgeDB(t *testing.T) (*gorm.DB, uint) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "idem-purge", Email: "idem-purge@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	return db, user.ID
}

func seedIdemKey(t *testing.T, db *gorm.DB, userID uint, key string, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age)
	require.NoError(t, db.Create(&models.IdempotencyKey{
		UserID: userID, Key: key, Method: "POST", Path: "/api/v1/contacts",
		RequestFingerprint: "fp-" + key, State: models.IdempotencyStateCompleted,
		ResponseStatus: 201, CreatedAt: ts, UpdatedAt: ts,
	}).Error)
}

func countIdemKeys(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Count(&n).Error)
	return n
}

func TestPurgeExpiredIdempotencyKeys_RemovesOnlyExpired(t *testing.T) {
	db, uid := newIdempotencyPurgeDB(t)
	seedIdemKey(t, db, uid, "fresh", 1*time.Hour)
	seedIdemKey(t, db, uid, "borderline", 23*time.Hour)
	seedIdemKey(t, db, uid, "stale", 25*time.Hour)
	seedIdemKey(t, db, uid, "ancient", 30*24*time.Hour)

	PurgeExpiredIdempotencyKeys(db, config.Config{IdempotencyKeyRetentionHours: 24})

	assert.EqualValues(t, 2, countIdemKeys(t, db), "only rows younger than 24h survive")
	var survivors []string
	require.NoError(t, db.Model(&models.IdempotencyKey{}).Order("idempotency_key").Pluck("idempotency_key", &survivors).Error)
	assert.Equal(t, []string{"borderline", "fresh"}, survivors)
}

func TestPurgeExpiredIdempotencyKeys_NonPositiveRetentionDisables(t *testing.T) {
	db, uid := newIdempotencyPurgeDB(t)
	seedIdemKey(t, db, uid, "ancient", 90*24*time.Hour)

	PurgeExpiredIdempotencyKeys(db, config.Config{IdempotencyKeyRetentionHours: 0})
	assert.EqualValues(t, 1, countIdemKeys(t, db), "retention <= 0 disables the purge, it must not wipe the table")

	PurgeExpiredIdempotencyKeys(db, config.Config{IdempotencyKeyRetentionHours: -5})
	assert.EqualValues(t, 1, countIdemKeys(t, db))
}

func TestPurgeExpiredIdempotencyKeysScheduled_JobLockGuards(t *testing.T) {
	db, uid := newIdempotencyPurgeDB(t)
	seedIdemKey(t, db, uid, "ancient", 48*time.Hour)

	cfg := config.Config{IdempotencyKeyRetentionHours: 24}
	PurgeExpiredIdempotencyKeysScheduled(db, cfg)
	assert.EqualValues(t, 0, countIdemKeys(t, db), "first scheduled run purges")

	// Immediately re-running is suppressed by the job lock (min-interval),
	// so a fresh stale row seeded now survives until the interval elapses.
	seedIdemKey(t, db, uid, "ancient-2", 48*time.Hour)
	PurgeExpiredIdempotencyKeysScheduled(db, cfg)
	assert.EqualValues(t, 1, countIdemKeys(t, db), "the job lock suppresses the immediate second run")
}
