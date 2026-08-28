package models

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newJobRunTestDB builds a real migrated schema (CLAUDE.md backend trap 1) so
// migration 000041's CHECK constraints and column names are exercised, not
// GORM's AutoMigrate guess.
func newJobRunTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	require.NoError(t, db.Exec("DELETE FROM job_runs").Error)
	return db
}

func TestRecordJobRun_PersistsWithDefaults(t *testing.T) {
	db := newJobRunTestDB(t)

	ctx := logger.WithCorrelationID(context.Background(), "job:daily_reminders:abc")
	start := time.Now().Add(-2 * time.Second)
	RecordJobRun(ctx, db, JobRun{
		JobName:    JobNameDailyReminders,
		StartedAt:  start,
		FinishedAt: start.Add(1500 * time.Millisecond),
	})

	var got JobRun
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.Equal(t, JobNameDailyReminders, got.JobName)
	require.Equal(t, "job:daily_reminders:abc", got.CorrelationID, "correlation id filled from context")
	require.Equal(t, JobTriggerScheduled, got.Trigger, "trigger defaulted")
	require.Equal(t, logger.ResultSuccess, got.Result, "result defaulted")
	require.EqualValues(t, 1500, got.DurationMS, "duration derived from start/finish")
	require.Nil(t, got.ItemsProcessed)
}

func TestRecordJobRun_ItemsProcessedAndTrigger(t *testing.T) {
	db := newJobRunTestDB(t)

	n := 7
	RecordJobRun(context.Background(), db, JobRun{
		JobName:        JobNameReachOutDetection,
		Trigger:        JobTriggerManual,
		Result:         logger.ResultSuccess,
		ItemsProcessed: &n,
		Detail:         "suggestions=7",
	})

	var got JobRun
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.Equal(t, JobTriggerManual, got.Trigger)
	require.NotNil(t, got.ItemsProcessed)
	require.Equal(t, 7, *got.ItemsProcessed)
}

func TestRecordJobRun_SanitizesAndTruncates(t *testing.T) {
	db := newJobRunTestDB(t)

	long := make([]rune, maxJobRunFieldLen+500)
	for i := range long {
		long[i] = 'a'
	}
	RecordJobRun(context.Background(), db, JobRun{
		JobName: JobNameCalendarSync,
		Result:  logger.ResultFailure,
		Error:   "boom\nwith newline",
		Detail:  string(long),
	})

	var got JobRun
	require.NoError(t, db.Order("id desc").First(&got).Error)
	require.Equal(t, `boom\nwith newline`, got.Error, "error string sanitized")
	require.LessOrEqual(t, len([]rune(got.Detail)), maxJobRunFieldLen)
}

func TestRecordJobRun_UnknownResultRejectedByCheckConstraint(t *testing.T) {
	db := newJobRunTestDB(t)

	// The emitter swallows the error (best-effort), so assert on the row
	// count: the CHECK constraint in migration 000041 must reject it.
	RecordJobRun(context.Background(), db, JobRun{
		JobName: JobNameAlertEval,
		Result:  "bogus",
	})

	var count int64
	require.NoError(t, db.Model(&JobRun{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestRecordJobRun_NilDBIsNoop(t *testing.T) {
	require.NotPanics(t, func() {
		RecordJobRun(context.Background(), nil, JobRun{JobName: JobNameDailyReminders})
	})
}

func TestKnownJobNames_UniqueAndNonEmpty(t *testing.T) {
	require.NotEmpty(t, KnownJobNames)
	seen := map[string]bool{}
	for _, name := range KnownJobNames {
		require.NotEmpty(t, name)
		require.False(t, seen[name], "duplicate job name %q", name)
		seen[name] = true
	}
}
