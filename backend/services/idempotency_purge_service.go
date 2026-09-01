package services

import (
	"time"

	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// idempotencyKeyPurgeMinInterval is slightly less than the cron cadence so a
// natural clock-skew or overlap doesn't cause a skipped run (mirrors the other
// purge jobs' constants).
const idempotencyKeyPurgeMinInterval = 55 * time.Minute

// PurgeExpiredIdempotencyKeys hard-deletes idempotency_keys rows older than the
// retention window (IDEMPOTENCY_KEY_RETENTION_HOURS, default 24 — issue #459,
// CON-04, ADR 0010).
//
// The window is short by design: an Idempotency-Key protects a single client
// operation across its retries, which happen within seconds to minutes, not
// days. A day is a generous ceiling. Each row can hold a copy of the created
// entity in response_body, so the window also bounds that copy the way
// WEBHOOK_DELIVERY_RETENTION_DAYS bounds a delivery payload.
//
// The window anchors on created_at (when the key was first claimed). A
// non-positive retention value disables the purge rather than deleting every
// row, matching webhook_delivery_purge_service.go / audit_purge_service.go.
func PurgeExpiredIdempotencyKeys(db *gorm.DB, cfg config.Config) {
	if cfg.IdempotencyKeyRetentionHours <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(cfg.IdempotencyKeyRetentionHours) * time.Hour)

	result := db.Exec("DELETE FROM idempotency_keys WHERE created_at < ?", cutoff)
	if result.Error != nil {
		logger.Error().Err(result.Error).Msg("idempotency key purge: failed to delete expired keys")
		return
	}
	if result.RowsAffected > 0 {
		logger.Info().Int64("rows", result.RowsAffected).Time("cutoff", cutoff).Msg("Purged expired idempotency keys")
	}
}

// PurgeExpiredIdempotencyKeysScheduled is the scheduled cron entry point; it
// acquires a job lock so concurrent runs (multi-instance, rapid restarts)
// don't double-purge.
func PurgeExpiredIdempotencyKeysScheduled(db *gorm.DB, cfg config.Config) {
	acquired, err := acquireJobLock(db, models.JobNameIdempotencyKeyPurge, idempotencyKeyPurgeMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("idempotency key purge: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameIdempotencyKeyPurge, true); err != nil {
			logger.Error().Err(err).Msg("idempotency key purge: failed to release job lock")
		}
	}()

	PurgeExpiredIdempotencyKeys(db, cfg)
}
