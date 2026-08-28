package services

import (
	"context"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// jobRunPurgeMinInterval is slightly less than the 24h cron cadence so a
// natural clock-skew or overlap doesn't cause a skipped run (mirrors the
// system-event / audit purge jobs' own constant).
const jobRunPurgeMinInterval = 23 * time.Hour

// PurgeExpiredJobRuns hard-deletes job_runs rows older than the retention
// window (JOB_RUN_RETENTION_DAYS, default 30 — operational diagnostics,
// short-lived by design; long enough to spot a slow-creep trend, short enough
// to bound growth on a single-file SQLite database). This is the only delete
// path for the table.
func PurgeExpiredJobRuns(ctx context.Context, db *gorm.DB, cfg config.Config) {
	if cfg.JobRunRetentionDays <= 0 {
		// Misconfigured to 0/negative: treat as disabled rather than deleting
		// every row.
		return
	}
	cutoff := time.Now().Add(-time.Duration(cfg.JobRunRetentionDays) * 24 * time.Hour)

	result := db.Exec("DELETE FROM job_runs WHERE started_at < ?", cutoff)
	if result.Error != nil {
		logger.Ctx(ctx).Error().Err(result.Error).
			Str(logger.FieldEvent, "job_run_purge_failed").
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Msg("job run purge: failed to delete expired rows")
		return
	}
	if result.RowsAffected > 0 {
		logger.Ctx(ctx).Info().
			Int64("rows", result.RowsAffected).
			Time("cutoff", cutoff).
			Str(logger.FieldEvent, "job_run_purge_completed").
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Msg("Purged expired job runs")
	}
}

// PurgeExpiredJobRunsScheduled is the scheduled cron entry point; it acquires
// a job lock so concurrent runs (multi-instance, rapid restarts) don't
// double-purge.
func PurgeExpiredJobRunsScheduled(db *gorm.DB, cfg config.Config) {
	ctx := logger.JobContext(models.JobNameJobRunPurge)

	acquired, err := acquireJobLock(db, models.JobNameJobRunPurge, jobRunPurgeMinInterval)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str(logger.FieldEvent, "job_failed").
			Str(logger.FieldOperation, models.JobNameJobRunPurge).
			Msg("job run purge: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameJobRunPurge, true); err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str(logger.FieldOperation, models.JobNameJobRunPurge).
				Msg("job run purge: failed to release job lock")
		}
	}()

	PurgeExpiredJobRuns(ctx, db, cfg)
}
