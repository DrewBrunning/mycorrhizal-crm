package services

import (
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

// EventDBIntegrityCheckFailed is the webhook event fired when a scheduled
// integrity check finds corruption, so an operator can wire up ntfy/Gotify/a
// generic webhook receiver instead of only relying on log scraping.
const EventDBIntegrityCheckFailed = "db.integrity_check_failed"

// dbIntegrityCheckMinIntervalMargin mirrors calendar_sync_service.go's
// pattern for a configurable-interval job: the job-lock minimum interval is
// the configured cadence minus a margin, so ordinary clock skew between cron
// firings never causes a run to be skipped as "too recent". Floored the same
// way calendar sync floors its own margin.
const dbIntegrityCheckMinIntervalMargin = 30 * time.Minute

func dbIntegrityCheckMinInterval(cfg config.Config) time.Duration {
	minInterval := time.Duration(cfg.DBIntegrityCheckIntervalHours)*time.Hour - dbIntegrityCheckMinIntervalMargin
	if minInterval < dbIntegrityCheckMinIntervalMargin {
		minInterval = dbIntegrityCheckMinIntervalMargin
	}
	return minInterval
}

// integrityCheckRow is the shape of one row PRAGMA integrity_check returns.
// A healthy database returns exactly one row containing "ok"; a corrupted
// one returns one row per problem found, so every row is collected rather
// than only the first (backup.go's verifyBackup takes only the first row,
// which is fine there since VACUUM INTO's own success already implies the
// snapshot round-tripped cleanly — here the *list* of problems is the point).
type integrityCheckRow struct {
	IntegrityCheck string `gorm:"column:integrity_check"`
}

// walCheckpointRow is the shape PRAGMA wal_checkpoint(...) returns.
type walCheckpointRow struct {
	Busy         int `gorm:"column:busy"`
	Log          int `gorm:"column:log"`
	Checkpointed int `gorm:"column:checkpointed"`
}

// checkDBIntegrity runs PRAGMA integrity_check against the live database,
// then a best-effort WAL checkpoint. It reuses the app's own live *gorm.DB
// connection rather than opening a second one — every other scheduled job in
// this package does the same.
//
// PASSIVE, not TRUNCATE: TRUNCATE reports busy=1 the instant any connection
// holds a read transaction, which is exactly the state a live server under
// real traffic is normally in (see database/backup.go's checkpoint(), which
// made the identical call for the identical reason). A checkpoint that
// spuriously fails under load would turn this job into a source of false
// alarms, so the checkpoint step is a best-effort tidy-up: its error is
// logged but never treated as a failed check.
func checkDBIntegrity(db *gorm.DB) (ok bool, detail string, err error) {
	var rows []integrityCheckRow
	if err := db.Raw("PRAGMA integrity_check").Scan(&rows).Error; err != nil {
		return false, "", err
	}

	if len(rows) == 1 && strings.EqualFold(rows[0].IntegrityCheck, "ok") {
		ok = true
	} else {
		lines := make([]string, 0, len(rows))
		for _, r := range rows {
			lines = append(lines, r.IntegrityCheck)
		}
		detail = strings.Join(lines, "; ")
	}

	var cp walCheckpointRow
	if cpErr := db.Raw("PRAGMA wal_checkpoint(PASSIVE)").Scan(&cp).Error; cpErr != nil {
		logger.Warn().Err(cpErr).Msg("db integrity check: best-effort WAL checkpoint failed")
	}

	return ok, detail, nil
}

// CheckDBIntegrityScheduled is the scheduled job entry point (issue #273).
// Job-lock guarded (the T19 pattern) so a multi-instance deploy or a rapid
// restart doesn't re-run it back to back.
func CheckDBIntegrityScheduled(db *gorm.DB, cfg config.Config) {
	ctx := logger.JobContext(models.JobNameDBIntegrityCheck)
	if !cfg.DBIntegrityCheckEnabled {
		return
	}

	acquired, err := acquireJobLock(db, models.JobNameDBIntegrityCheck, dbIntegrityCheckMinInterval(cfg))
	if err != nil {
		logger.Error().Err(err).Msg("db integrity check: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameDBIntegrityCheck, true); err != nil {
			logger.Error().Err(err).Msg("db integrity check: failed to release job lock")
		}
	}()

	ok, detail, err := checkDBIntegrity(db)
	if err != nil {
		logger.Error().Err(err).Msg("db integrity check: failed to run PRAGMA integrity_check")
		RecordOperationalCheckResult(db, models.JobNameDBIntegrityCheck, models.OpCheckStatusError, err.Error())
		triggerWebhooksForAllUsers(ctx, db, cfg, EventDBIntegrityCheckFailed, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	if !ok {
		logger.Error().Str("detail", detail).Msg("db integrity check: corruption detected")
		RecordOperationalCheckResult(db, models.JobNameDBIntegrityCheck, models.OpCheckStatusFailed, detail)
		triggerWebhooksForAllUsers(ctx, db, cfg, EventDBIntegrityCheckFailed, map[string]interface{}{
			"detail": detail,
		})
		return
	}

	RecordOperationalCheckResult(db, models.JobNameDBIntegrityCheck, models.OpCheckStatusOK, "")
	logger.Info().Msg("db integrity check: ok")
}
