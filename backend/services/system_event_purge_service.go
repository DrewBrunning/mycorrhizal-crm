package services

import (
	"context"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"time"

	"gorm.io/gorm"
)

// systemEventPurgeMinInterval is slightly less than the 24h cron cadence so a
// natural clock-skew or overlap doesn't cause a skipped run (mirrors the
// audit purge job's own constant).
var systemEventPurgeMinInterval = JobCatchupWindow(24 * time.Hour)

// PurgeExpiredSystemEvents hard-deletes system_events rows older than the
// retention window (SYSTEM_EVENT_RETENTION_DAYS, default 30 — operational
// diagnostics, short-lived by design; long enough to investigate an incident,
// short enough to bound growth on a single-file SQLite database). This is the
// only delete path for the table.
func PurgeExpiredSystemEvents(ctx context.Context, db *gorm.DB, cfg config.Config) {
	if cfg.SystemEventRetentionDays <= 0 {
		// Misconfigured to 0/negative: treat as disabled rather than deleting
		// every row.
		return
	}
	cutoff := time.Now().Add(-time.Duration(cfg.SystemEventRetentionDays) * 24 * time.Hour)

	result := db.Exec("DELETE FROM system_events WHERE occurred_at < ?", cutoff)
	if result.Error != nil {
		logger.Ctx(ctx).Error().Err(result.Error).
			Str(logger.FieldEvent, "system_event_purge_failed").
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Msg("system event purge: failed to delete expired rows")
		return
	}
	if result.RowsAffected > 0 {
		logger.Ctx(ctx).Info().
			Int64("rows", result.RowsAffected).
			Time("cutoff", cutoff).
			Str(logger.FieldEvent, "system_event_purge_completed").
			Str(logger.FieldComponent, logger.ComponentScheduler).
			Msg("Purged expired system events")
	}
}

// PurgeExpiredSystemEventsScheduled is the scheduled cron entry point; it
// acquires a job lock so concurrent runs (multi-instance, rapid restarts)
// don't double-purge.
func PurgeExpiredSystemEventsScheduled(db *gorm.DB, cfg config.Config) {
	ctx := logger.JobContext(models.JobNameSystemEventPurge)

	acquired, err := acquireJobLock(db, models.JobNameSystemEventPurge, systemEventPurgeMinInterval)
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str(logger.FieldEvent, "job_failed").
			Str(logger.FieldOperation, models.JobNameSystemEventPurge).
			Msg("system event purge: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameSystemEventPurge, true); err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str(logger.FieldOperation, models.JobNameSystemEventPurge).
				Msg("system event purge: failed to release job lock")
		}
	}()

	PurgeExpiredSystemEvents(ctx, db, cfg)
}
