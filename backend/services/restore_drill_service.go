package services

import (
	"context"
	"fmt"
	"mycorrhizal/atrest"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// EventRestoreDrillFailed is the webhook event fired when a scheduled
// restore drill fails — either the backup/restore itself errored, or the
// restored snapshot's row counts don't match the live database.
const EventRestoreDrillFailed = "db.restore_drill_failed"

// restoreDrillMinInterval is the de-dup window for this configurable-cadence
// job: the shared JobCatchupWindow of the configured period (issue #526, ADR
// 0011 — one margin for the whole fleet).
func restoreDrillMinInterval(cfg config.Config) time.Duration {
	return JobCatchupWindow(time.Duration(cfg.DBRestoreDrillIntervalHours) * time.Hour)
}

type tableNameRow struct {
	Name string `gorm:"column:name"`
}

type countRow struct {
	N int64 `gorm:"column:n"`
}

// excludedFromRestoreDrill are tables deliberately left out of the row-count
// comparison: operational bookkeeping, not user data, and each is written
// *by this job itself* (or by the other scheduled jobs firing alongside it)
// in the window between the snapshot and the live count read, so a live-vs-
// restored delta on them is a guaranteed false positive, not a backup failure.
//   - job_executions: every scheduled job writes its lock row on every fire;
//     at boot they all fire together (each `go safeGo(...)` initial run in
//     main.go). Confirmed live: "job_executions: live=9 restored=7".
//   - system_events (#424) / operational_check_results (#421/#620): the drill
//     records its own restore_test_completed / backup_failed row and its own
//     check-result row while it runs — the live count is always ahead of the
//     snapshot by exactly those.
var excludedFromRestoreDrill = map[string]bool{
	"job_executions":            true,
	"system_events":             true,
	"operational_check_results": true,
	// alert_states (#428): the scheduled alert evaluator fires alongside this
	// drill and upserts a row per condition in the snapshot-vs-live window, so
	// a delta here is a guaranteed false positive — same reasoning as the rows
	// above.
	"alert_states": true,
}

// liveTables lists every real, non-internal, non-excluded table in the
// database — used instead of a hand-maintained model list so this never goes
// stale as entities are added (the same maintenance trap CLAUDE.md documents
// for DeleteContact's cascade list).
func liveTables(db *gorm.DB) ([]string, error) {
	var rows []tableNameRow
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name").Scan(&rows).Error; err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if excludedFromRestoreDrill[r.Name] {
			continue
		}
		names = append(names, r.Name)
	}
	return names, nil
}

// countTableRows returns COUNT(*) for one table. Table names come from
// sqlite_master (this app's own migrations), never user input, but the
// identifier is still quoted and escaped defensively, mirroring
// database/backup.go's VACUUM INTO path escaping.
func countTableRows(db *gorm.DB, table string) (int64, error) {
	escaped := strings.ReplaceAll(table, `"`, `""`)
	var row countRow
	if err := db.Raw(`SELECT COUNT(*) AS n FROM "` + escaped + `"`).Scan(&row).Error; err != nil {
		return 0, fmt.Errorf("count %q: %w", table, err)
	}
	return row.N, nil
}

// runRestoreDrill takes a fresh backup snapshot, restores it into a scratch
// database, and compares per-table row counts against the live database.
//
// "Restore" here means the same proof backup_test.go's assertSeededData
// already established: reopen the snapshot via database.InitDB, which runs
// pending migrations — so a snapshot that is byte-valid SQLite but wouldn't
// actually boot the app fails here as loudly as it would during a real
// disaster, not just a bare integrity_check.
//
// The live counts are read immediately after the snapshot completes, not
// before, to keep the window in which a real write could land between
// "snapshot" and "compare" as small as possible. That window can never be
// fully closed without stopping the server (the snapshot is a live,
// online backup by design — see BackupSnapshot), so on a real self-hosted,
// low-traffic instance running this at most weekly, a false-positive
// mismatch from an in-flight write is an accepted, rare cost.
func runRestoreDrill(db *gorm.DB, cfg config.Config) (ok bool, detail string, err error) {
	scratchDir, err := os.MkdirTemp("", "mycorrhizal-restore-drill-*")
	if err != nil {
		return false, "", fmt.Errorf("create scratch dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(scratchDir); rmErr != nil {
			logger.Warn().Err(rmErr).Str("dir", scratchDir).Msg("restore drill: failed to clean up scratch dir")
		}
	}()

	scratchPath := filepath.Join(scratchDir, "restore-drill.db")
	if err := database.BackupSnapshot(cfg.DBPath, scratchPath); err != nil {
		return false, "", fmt.Errorf("backup snapshot: %w", err)
	}

	liveNames, err := liveTables(db)
	if err != nil {
		return false, "", fmt.Errorf("list live tables: %w", err)
	}

	liveCounts := make(map[string]int64, len(liveNames))
	for _, name := range liveNames {
		n, err := countTableRows(db, name)
		if err != nil {
			return false, "", fmt.Errorf("count live table: %w", err)
		}
		liveCounts[name] = n
	}

	scratchDB, err := database.InitDB(scratchPath)
	if err != nil {
		return false, "", fmt.Errorf("open restored snapshot: %w", err)
	}
	defer func() {
		if sqlDB, dbErr := scratchDB.DB(); dbErr == nil {
			if closeErr := sqlDB.Close(); closeErr != nil {
				logger.Warn().Err(closeErr).Msg("restore drill: failed to close restored snapshot")
			}
		}
	}()

	// A row-count match proves the snapshot boots and holds the same rows,
	// but not that a real restore could *decrypt* it — the encrypted columns
	// count the same regardless of key. Verify the restored snapshot's
	// wrapped DEK unwraps under the current master key, so a rotated or lost
	// key is caught here (weekly, in a throwaway scratch DB) instead of at
	// the moment of need during a real disaster (issue #420).
	kek, err := atrest.EncryptionKey()
	if err != nil {
		return false, "", fmt.Errorf("resolve at-rest master key: %w", err)
	}
	if err := atrest.VerifyBackupDecryptable(scratchDB, kek); err != nil {
		return false, "", fmt.Errorf("restored snapshot is not decryptable under the current master key: %w", err)
	}

	ok, detail, err = compareTableCounts(liveCounts, scratchDB)
	if err != nil || !ok {
		return ok, detail, err
	}

	// A backup is three pieces, not one (BACKUP-02, issue #454): the database
	// snapshot plus the profile-photo and attachment directories. Row counts
	// prove the snapshot restores; they say nothing about whether every
	// attachment and photo row still resolves to a file on disk. Reconcile the
	// fresh snapshot against the *live* directories — what a real backup would
	// copy — so a lost or un-backed-up photo/attachment directory is caught
	// weekly rather than at the moment of need.
	return checkBackupSetCompleteness(scratchPath, cfg)
}

// checkBackupSetCompleteness runs database.VerifyBackupSet for the restore
// drill: a missing file fails the drill (same channel as a row-count
// mismatch); orphan files — a file whose owning row is newer than the
// snapshot — are a routine race against a live directory and only logged.
//
// When the photo/attachment directories are not configured (as in most unit
// tests) there is nothing to reconcile and the drill passes on the row-count
// result alone.
func checkBackupSetCompleteness(snapshotPath string, cfg config.Config) (ok bool, detail string, err error) {
	if cfg.ProfilePhotoDir == "" || cfg.AttachmentsDir == "" {
		return true, "", nil
	}

	report, err := database.VerifyBackupSet(snapshotPath, cfg.ProfilePhotoDir, cfg.AttachmentsDir)
	if err != nil {
		return false, "", fmt.Errorf("verify backup set completeness: %w", err)
	}

	if n := report.TotalOrphans(); n > 0 {
		logger.Info().Int("orphan_files", n).
			Msg("restore drill: files present with no owning row (informational; not a drill failure)")
	}

	if !report.Complete() {
		refs := make([]string, 0, report.TotalMissing())
		for _, m := range report.MissingAttachments {
			refs = append(refs, m.String())
		}
		for _, m := range report.MissingPhotos {
			refs = append(refs, m.String())
		}
		return false, "backup set incomplete: " + strings.Join(refs, "; "), nil
	}
	return true, "", nil
}

// compareTableCounts checks liveCounts (table name -> row count, gathered
// from the live database) against the same tables counted in scratchDB (the
// restored snapshot). Split out from runRestoreDrill so the comparison logic
// itself can be tested directly against two independently-seeded databases,
// without depending on BackupSnapshot's own timing.
func compareTableCounts(liveCounts map[string]int64, scratchDB *gorm.DB) (ok bool, detail string, err error) {
	var mismatches []string
	for name, liveCount := range liveCounts {
		scratchCount, err := countTableRows(scratchDB, name)
		if err != nil {
			return false, "", fmt.Errorf("count restored table: %w", err)
		}
		if scratchCount != liveCount {
			mismatches = append(mismatches, fmt.Sprintf("%s: live=%d restored=%d", name, liveCount, scratchCount))
		}
	}

	if len(mismatches) > 0 {
		return false, strings.Join(mismatches, "; "), nil
	}
	return true, "", nil
}

// RunRestoreDrillScheduled is the scheduled job entry point (issue #275).
// Job-lock guarded like every other scheduled job in this package.
func RunRestoreDrillScheduled(db *gorm.DB, cfg config.Config) {
	if !cfg.DBRestoreDrillEnabled {
		return
	}

	ctx := logger.JobContext(models.JobNameRestoreDrill)

	acquired, err := acquireJobLock(db, models.JobNameRestoreDrill, restoreDrillMinInterval(cfg))
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).Msg("restore drill: failed to check job lock")
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameRestoreDrill, true); err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("restore drill: failed to release job lock")
		}
	}()

	start := time.Now()
	ok, detail, err := runRestoreDrill(db, cfg)
	durMS := time.Since(start).Milliseconds()

	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str(logger.FieldEvent, models.SysEventBackupFailed).
			Str(logger.FieldComponent, logger.ComponentBackup).
			Str(logger.FieldResult, logger.ResultFailure).
			Int64(logger.FieldDurationMS, durMS).
			Msg("restore drill: failed to run")
		// Last-known status per subsystem (#421/#620) and the operational-event
		// timeline (#424) are complementary: the former is "what state is it in
		// now", the latter is the dated history.
		RecordOperationalCheckResult(db, models.JobNameRestoreDrill, models.OpCheckStatusError, err.Error())
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType: models.SysEventBackupFailed, Component: logger.ComponentBackup,
			Operation: models.JobNameRestoreDrill, Result: models.SysResult(logger.ResultFailure),
			DurationMS: &durMS, Error: err.Error(), Detail: "restore drill could not run",
		})
		triggerWebhooksForAllUsers(ctx, db, cfg, EventRestoreDrillFailed, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	if !ok {
		logger.Ctx(ctx).Error().Str("detail", detail).
			Str(logger.FieldEvent, models.SysEventBackupFailed).
			Str(logger.FieldComponent, logger.ComponentBackup).
			Str(logger.FieldResult, logger.ResultFailure).
			Int64(logger.FieldDurationMS, durMS).
			Msg("restore drill: row-count mismatch")
		RecordOperationalCheckResult(db, models.JobNameRestoreDrill, models.OpCheckStatusFailed, detail)
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType: models.SysEventBackupFailed, Component: logger.ComponentBackup,
			Operation: models.JobNameRestoreDrill, Result: models.SysResult(logger.ResultFailure),
			DurationMS: &durMS, Detail: detail,
		})
		triggerWebhooksForAllUsers(ctx, db, cfg, EventRestoreDrillFailed, map[string]interface{}{
			"detail": detail,
		})
		return
	}

	// RTO-budget check (issue #506). The drill has proven the backup restores;
	// this is a separate, non-fatal observation about *how long* the database
	// piece took, so an over-budget run annotates the timeline and logs a WARN
	// rather than failing the drill (which would page an operator for a backup
	// that is, in fact, fine). duration_ms is recorded either way.
	rtoDetail := evaluateRestoreDrillRTOBudget(ctx, cfg, durMS)

	RecordOperationalCheckResult(db, models.JobNameRestoreDrill, models.OpCheckStatusOK, "")
	logger.Ctx(ctx).Info().
		Str(logger.FieldEvent, models.SysEventRestoreTestCompleted).
		Str(logger.FieldComponent, logger.ComponentBackup).
		Str(logger.FieldResult, logger.ResultSuccess).
		Int64(logger.FieldDurationMS, durMS).
		Msg("restore drill: ok")
	models.RecordSystemEvent(ctx, db, models.SystemEvent{
		EventType: models.SysEventRestoreTestCompleted, Component: logger.ComponentBackup,
		Operation: models.JobNameRestoreDrill, Result: models.SysResult(logger.ResultSuccess),
		DurationMS: &durMS, Detail: rtoDetail,
	})
}

// evaluateRestoreDrillRTOBudget compares a completed drill's wall-clock against
// the operator's RTO budget (DB_RESTORE_DRILL_MAX_DURATION_SECONDS). When a
// budget is set (> 0) and the run exceeded it, it logs one WARN and returns the
// note to stamp on the restore_test_completed timeline row. No budget, or a run
// within budget, returns "" and logs nothing — the Info "restore drill: ok"
// line and the recorded duration_ms still happen in the caller regardless.
func evaluateRestoreDrillRTOBudget(ctx context.Context, cfg config.Config, durMS int64) string {
	if cfg.DBRestoreDrillMaxDurationSeconds <= 0 {
		return ""
	}
	budgetMS := int64(cfg.DBRestoreDrillMaxDurationSeconds) * 1000
	if durMS <= budgetMS {
		return ""
	}
	logger.Ctx(ctx).Warn().
		Str(logger.FieldEvent, models.SysEventRestoreTestCompleted).
		Str(logger.FieldComponent, logger.ComponentBackup).
		Int64(logger.FieldDurationMS, durMS).
		Int("budget_seconds", cfg.DBRestoreDrillMaxDurationSeconds).
		Msg("restore drill: over RTO budget")
	return fmt.Sprintf(
		"restore drill took %dms, over the %ds RTO budget (DB_RESTORE_DRILL_MAX_DURATION_SECONDS)",
		durMS, cfg.DBRestoreDrillMaxDurationSeconds,
	)
}
