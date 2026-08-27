package services

import (
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// RecordOperationalCheckResult upserts the latest outcome of a named
// operational self-check (issue #421). One row per checkName: the scheduled
// DB-integrity-check (issue #273) and restore-drill (issue #275) jobs call
// this on every terminal path so the deep GET /health endpoint can report the
// last actual pass/fail rather than only "did it run recently".
//
// A failure to persist the result is logged, not returned: it must never turn
// a passing check into a failing job. Follows the find-then-save idiom used by
// acquireJobLock rather than a DB-specific upsert clause (this repo has no
// clause.OnConflict precedent).
func RecordOperationalCheckResult(db *gorm.DB, checkName, status, detail string) {
	now := time.Now()

	err := db.Transaction(func(tx *gorm.DB) error {
		var row models.OperationalCheckResult
		err := tx.Where("check_name = ?", checkName).First(&row).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&models.OperationalCheckResult{
				CheckName: checkName,
				Status:    status,
				Detail:    detail,
				CheckedAt: now,
			}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]interface{}{
			"status":     status,
			"detail":     detail,
			"checked_at": now,
		}).Error
	})
	if err != nil {
		logger.Error().Err(err).Str("check", checkName).Str("status", status).
			Msg("failed to record operational check result")
	}
}
