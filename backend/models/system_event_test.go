package models

import (
	"context"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/logger"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSystemEventTestDB builds a real migrated schema (CLAUDE.md backend trap
// 1) so the CHECK constraints and column names in migration 000037 are
// exercised, not GORM's AutoMigrate guess.
func newSystemEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "sysevent-test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { RegisterAuditDB(nil) })
	// InitDB's migration run records its own migration_completed event; clear
	// it so each test controls the full row set it asserts on.
	require.NoError(t, db.Exec("DELETE FROM system_events").Error)
	return db
}

func TestRecordSystemEvent_PersistsWithDefaults(t *testing.T) {
	db := newSystemEventTestDB(t)

	ctx := logger.WithCorrelationID(context.Background(), "corr-77")
	RecordSystemEvent(ctx, db, SystemEvent{
		EventType: SysEventJobCompleted,
		Component: logger.ComponentScheduler,
		Operation: "purge_deleted",
		Result:    SysResult(logger.ResultSuccess),
		Detail:    "rows=3",
	})

	var got SystemEvent
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.Equal(t, SysEventJobCompleted, got.EventType)
	require.Equal(t, "corr-77", got.CorrelationID, "correlation id filled from context")
	require.Equal(t, logger.SeverityInfo, got.Severity, "severity defaulted from result")
	require.False(t, got.OccurredAt.IsZero(), "occurred_at defaulted")
	require.NotNil(t, got.Result)
	require.Equal(t, logger.ResultSuccess, *got.Result)
}

func TestRecordSystemEvent_FailureResultDefaultsSeverityError(t *testing.T) {
	db := newSystemEventTestDB(t)

	RecordSystemEvent(context.Background(), db, SystemEvent{
		EventType: SysEventSyncFailed,
		Component: logger.ComponentContactSync,
		Result:    SysResult(logger.ResultFailure),
		Error:     "boom\nwith newline",
	})

	var got SystemEvent
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.Equal(t, logger.SeverityError, got.Severity)
	require.Equal(t, `boom\nwith newline`, got.Error, "error string sanitized")
}

func TestRecordSystemEvent_UnknownTypeRejectedByCheckConstraint(t *testing.T) {
	db := newSystemEventTestDB(t)

	// The emitter swallows the error (best-effort), so assert on the row
	// count: the CHECK constraint in migration 000037 must reject it.
	RecordSystemEvent(context.Background(), db, SystemEvent{
		EventType: "not_a_real_event",
		Component: "x",
	})

	var count int64
	require.NoError(t, db.Model(&SystemEvent{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestRecordSystemEvent_NilDBIsNoop(t *testing.T) {
	require.NotPanics(t, func() {
		RecordSystemEvent(context.Background(), nil, SystemEvent{EventType: SysEventApplicationStarted})
	})
}

func TestRecordSystemEvent_TruncatesLongDetail(t *testing.T) {
	db := newSystemEventTestDB(t)

	long := make([]rune, maxSystemEventFieldLen+500)
	for i := range long {
		long[i] = 'a'
	}
	RecordSystemEvent(context.Background(), db, SystemEvent{
		EventType: SysEventJobFailed,
		Detail:    string(long),
	})

	var got SystemEvent
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.LessOrEqual(t, len([]rune(got.Detail)), maxSystemEventFieldLen)
}

func TestSystemEventTypes_MatchConsts(t *testing.T) {
	// Guards against a token being added to one list but not the other.
	require.Len(t, SystemEventTypes, 17)
	seen := map[string]bool{}
	for _, tok := range SystemEventTypes {
		require.False(t, seen[tok], "duplicate token %q", tok)
		seen[tok] = true
	}
}
