package services

import (
	"context"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestNormalizeErrorCause pins the bucket-key normalization directly against
// the issue's examples (issue #426): the same failure across runs must collapse
// to one key, genuinely different failures must not.
func TestNormalizeErrorCause(t *testing.T) {
	t.Run("the issue's four causes each collapse across runs", func(t *testing.T) {
		cases := []struct{ a, b string }{
			{
				"CardDAV authentication failed (HTTP 401) for subscription 4821",
				"CardDAV authentication failed (HTTP 403) for subscription 9137",
			},
			{
				"SMTP timeout after 30s connecting to smtp.example.com:587",
				"SMTP timeout after 5s connecting to mail.acme.org:465",
			},
			{
				"database is locked (SQLITE_BUSY)",
				"database is locked (SQLITE_BUSY)",
			},
			{
				"Gotify returned HTTP 401",
				"Gotify returned HTTP 401",
			},
		}
		for _, c := range cases {
			assert.Equal(t, normalizeErrorCause(c.a), normalizeErrorCause(c.b),
				"%q and %q should share a cause key", c.a, c.b)
		}
	})

	t.Run("high-cardinality fragments are masked", func(t *testing.T) {
		assert.Equal(t,
			normalizeErrorCause("sync failed for 550e8400-e29b-41d4-a716-446655440000 at 2026-08-27T14:03:11Z"),
			normalizeErrorCause("sync failed for 00000000-0000-0000-0000-000000000000 at 2026-01-02T03:04:05Z"),
		)
		assert.Equal(t,
			normalizeErrorCause("POST https://hooks.example.com/abc123 failed: connection refused"),
			normalizeErrorCause("POST https://other.test/zzz failed: connection refused"),
		)
		assert.Equal(t,
			normalizeErrorCause(`open /var/lib/mycorrhizal/backups/2026-08-27.db: no such file`),
			normalizeErrorCause(`open /srv/data/x/y/z.db: no such file`),
		)
		assert.Equal(t,
			normalizeErrorCause("dial tcp 10.0.0.4:5432: i/o timeout"),
			normalizeErrorCause("dial tcp 192.168.1.9:5432: i/o timeout"),
		)
	})

	t.Run("genuinely different messages stay distinct", func(t *testing.T) {
		assert.NotEqual(t,
			normalizeErrorCause("connection refused"),
			normalizeErrorCause("context deadline exceeded"),
		)
		assert.NotEqual(t,
			normalizeErrorCause("CardDAV authentication failed (HTTP 401)"),
			normalizeErrorCause("database is locked (SQLITE_BUSY)"),
		)
	})
}

func seedErr(t *testing.T, db *gorm.DB, component, eventType string, occurredAt time.Time, errMsg string) {
	t.Helper()
	require.NoError(t, db.Create(&models.SystemEvent{
		Component:  component,
		EventType:  eventType,
		OccurredAt: occurredAt.UTC(),
		Error:      errMsg,
	}).Error)
}

// TestAggregateOperationalErrors folds the failure rows of system_events into
// per-cause buckets (issue #426). One real migrated DB is shared across the
// cases — each clears system_events first — mirroring the #427 tests, since the
// services package sits near the CI -race timeout.
func TestAggregateOperationalErrors(t *testing.T) {
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })

	reset := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	}
	since := func() time.Time { return time.Now().Add(-24 * time.Hour) }

	t.Run("the same cause across N runs is one recurring bucket linking N events", func(t *testing.T) {
		reset(t)
		base := time.Now().Add(-6 * time.Hour)
		for i := 0; i < 17; i++ {
			seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed,
				base.Add(time.Duration(i)*time.Minute),
				"CardDAV authentication failed (HTTP 401) for subscription "+strconv.Itoa(4000+i))
		}

		buckets, total, err := AggregateOperationalErrors(context.Background(), db, since())
		require.NoError(t, err)
		require.Len(t, buckets, 1)
		assert.Equal(t, 17, total)

		b := buckets[0]
		assert.Equal(t, logger.ComponentContactSync, b.Component)
		assert.Equal(t, 17, b.Count)
		assert.True(t, b.Recurring)
		assert.Equal(t, []string{models.SysEventSyncFailed}, b.EventTypes)
		assert.Len(t, b.EventIDs, 17)
		assert.False(t, b.EventIDsTruncated)
		assert.Contains(t, b.SampleError, "subscription 4016", "sample is the newest raw error")
		assert.True(t, b.LastSeen.After(b.FirstSeen))
	})

	t.Run("different causes are separate buckets, sorted by count desc", func(t *testing.T) {
		reset(t)
		now := time.Now()
		for i := 0; i < 17; i++ {
			seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-time.Duration(i)*time.Minute),
				"CardDAV authentication failed (HTTP 401) for subscription "+strconv.Itoa(i))
		}
		for i := 0; i < 4; i++ {
			seedErr(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-time.Duration(i)*time.Minute),
				"SMTP timeout after "+strconv.Itoa(i+1)+"s connecting to smtp.example.com:587")
		}
		for i := 0; i < 3; i++ {
			seedErr(t, db, logger.ComponentWebhook, models.SysEventIntegrationFailed, now.Add(-time.Duration(i)*time.Minute),
				"Gotify returned HTTP 401")
		}
		// Two SQLITE_BUSY on the same component as the CardDAV failures — must
		// NOT merge into the CardDAV bucket.
		for i := 0; i < 2; i++ {
			seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-time.Duration(i)*time.Minute),
				"database is locked (SQLITE_BUSY)")
		}

		buckets, total, err := AggregateOperationalErrors(context.Background(), db, since())
		require.NoError(t, err)
		assert.Equal(t, 26, total)
		require.Len(t, buckets, 4)

		assert.Equal(t, 17, buckets[0].Count)
		assert.Equal(t, 4, buckets[1].Count)
		assert.Equal(t, 3, buckets[2].Count)
		assert.Equal(t, 2, buckets[3].Count)

		assert.True(t, buckets[0].Recurring)
		assert.True(t, buckets[1].Recurring)
		assert.True(t, buckets[2].Recurring)
		assert.False(t, buckets[3].Recurring, "2 < threshold of 3")

		busy := buckets[3]
		assert.Equal(t, logger.ComponentContactSync, busy.Component)
		assert.Contains(t, busy.SampleError, "SQLITE_BUSY")
	})

	t.Run("a single transient failure is a non-recurring bucket", func(t *testing.T) {
		reset(t)
		seedErr(t, db, logger.ComponentCalendarSync, models.SysEventSyncFailed, time.Now().Add(-time.Hour), "connection refused")

		buckets, _, err := AggregateOperationalErrors(context.Background(), db, since())
		require.NoError(t, err)
		require.Len(t, buckets, 1)
		assert.Equal(t, 1, buckets[0].Count)
		assert.False(t, buckets[0].Recurring)
	})

	t.Run("rows outside the window and rows with no error string are excluded", func(t *testing.T) {
		reset(t)
		now := time.Now()
		seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-2*time.Hour), "carddav 401")
		seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-48*time.Hour), "carddav 401") // too old
		seedErr(t, db, logger.ComponentBackup, models.SysEventBackupFailed, now.Add(-1*time.Hour), "")                // no cause

		buckets, total, err := AggregateOperationalErrors(context.Background(), db, since())
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, buckets, 1)
		assert.Equal(t, 1, buckets[0].Count)
	})

	t.Run("a tie on count is broken by most recently seen", func(t *testing.T) {
		reset(t)
		now := time.Now()
		seedErr(t, db, logger.ComponentContactSync, models.SysEventSyncFailed, now.Add(-5*time.Hour), "older cause")
		seedErr(t, db, logger.ComponentNotify, models.SysEventNotificationFailed, now.Add(-1*time.Hour), "newer cause")

		buckets, _, err := AggregateOperationalErrors(context.Background(), db, since())
		require.NoError(t, err)
		require.Len(t, buckets, 2)
		assert.Equal(t, "newer cause", buckets[0].Cause)
		assert.Equal(t, "older cause", buckets[1].Cause)
	})
}
