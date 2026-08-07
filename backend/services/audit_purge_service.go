package services

import (
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"time"

	"gorm.io/gorm"
)

// auditPurgeMinInterval is slightly less than the 24h cron cadence so a
// natural clock-skew or overlap doesn't cause a skipped run (mirrors the T26
// purge job's own constant).
const auditPurgeMinInterval = 23 * time.Hour

// PurgeExpiredAuditEvents hard-deletes audit events older than the retention
// window (AUDIT_RETENTION_DAYS, default 90 — the ticket's locked decision:
// long enough for the debugging/undo use case, short enough to control SQLite
// file growth on a single-file database). This is the ONLY sanctioned delete
// path for the append-only audit table: the model has no delete receiver
// method, and the 000016 UPDATE trigger blocks every other mutation.
func PurgeExpiredAuditEvents(db *gorm.DB, cfg config.Config) {
	if cfg.AuditRetentionDays <= 0 {
		// Misconfigured to 0/negative: treat as disabled rather than
		// deleting every audit row.
		return
	}
	cutoff := time.Now().Add(-time.Duration(cfg.AuditRetentionDays) * 24 * time.Hour)

	result := db.Exec("DELETE FROM audit_events WHERE created_at < ?", cutoff)
	if result.Error != nil {
		logger.Error().Err(result.Error).Msg("audit purge: failed to delete expired audit events")
		return
	}
	if result.RowsAffected > 0 {
		logger.Info().Int64("rows", result.RowsAffected).Time("cutoff", cutoff).Msg("Purged expired audit events")
	}
}

// PurgeExpiredAuditEventsScheduled is the scheduled cron entry point; it
// acquires a job lock so concurrent runs (multi-instance, rapid restarts)
// don't double-purge.
func PurgeExpiredAuditEventsScheduled(db *gorm.DB, cfg config.Config) {
	acquired, err := acquireJobLock(db, models.JobNameAuditPurge, auditPurgeMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("audit purge: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameAuditPurge, true); err != nil {
			logger.Error().Err(err).Msg("audit purge: failed to release job lock")
		}
	}()

	PurgeExpiredAuditEvents(db, cfg)
}
