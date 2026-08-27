package services

import (
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

// restoreDrillMinIntervalMargin mirrors dbIntegrityCheckMinIntervalMargin —
// same reasoning, kept as a separate constant because the two jobs are
// independently configurable and there is no shared meaning between them.
const restoreDrillMinIntervalMargin = 30 * time.Minute

func restoreDrillMinInterval(cfg config.Config) time.Duration {
	minInterval := time.Duration(cfg.DBRestoreDrillIntervalHours)*time.Hour - restoreDrillMinIntervalMargin
	if minInterval < restoreDrillMinIntervalMargin {
		minInterval = restoreDrillMinIntervalMargin
	}
	return minInterval
}

type tableNameRow struct {
	Name string `gorm:"column:name"`
}

type countRow struct {
	N int64 `gorm:"column:n"`
}

// excludedFromRestoreDrill are tables deliberately left out of the row-count
// comparison. job_executions is operational bookkeeping, not user data, and
// it is uniquely guaranteed to be under concurrent write pressure at exactly
// the moment this job runs: every other scheduled job (including this one)
// writes its own lock row there on every fire, and at boot they all fire
// together (each registered with its own go safeGo(...) initial run in
// main.go). Confirmed live: a local run reported "job_executions: live=9
// restored=7" purely from other jobs' concurrent initial-run lock rows
// landing in the gap between the snapshot and the live count read — a false
// alarm on data nobody would call a backup failure over.
var excludedFromRestoreDrill = map[string]bool{
	"job_executions": true,
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

	return compareTableCounts(liveCounts, scratchDB)
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
		DurationMS: &durMS,
	})
}
