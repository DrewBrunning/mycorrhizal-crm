package database

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"mycorrhizal/logger"
)

// RecordOperatorBackupCompleted writes the operational-timeline heartbeat for a
// successful operator backup — `make backup` / cmd/backup (issue #943).
//
// It exists so the `backup_stale` alert measures the operator's own backup
// cadence rather than the app's weekly restore drill. The subsystem fold
// (services.ComputeSubsystemHealth) treats `restore_test_completed` and
// `backup_completed` tagged component=backup as backup-subsystem successes;
// before this, only the drill emitted one, so a dead backup cron stayed green.
// The pre-migration snapshot deliberately tags its `backup_completed` event
// component=migration (see recordPreMigrationBackupEvent) and therefore does
// NOT satisfy the freshness check — an upgrade is not a routine backup.
//
// The write is best-effort from the caller's point of view: a backup that has
// been written and signed is not undone because a diagnostic row could not be
// recorded, so cmd/backup logs a warning on a non-nil error rather than
// failing. The error is returned (rather than swallowed as in the migration
// path) so the operator sees that the freshness signal is stale.
func RecordOperatorBackupCompleted(dbPath, snapshotPath string) error {
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return fmt.Errorf("record backup heartbeat: open %q: %w", dbPath, err)
	}
	defer sqlDB.Close()

	var size int64
	if fi, statErr := os.Stat(snapshotPath); statErr == nil {
		size = fi.Size()
	}

	now := time.Now().UTC()
	_, err = sqlDB.Exec(
		`INSERT INTO system_events (created_at, occurred_at, event_type, severity, component, operation, result, detail)
		 VALUES (?, ?, 'backup_completed', 'info', 'backup', 'operator_backup', 'success', ?)`,
		now, now, fmt.Sprintf("path=%s bytes=%d", logger.SanitizeLogField(snapshotPath), size),
	)
	if err != nil {
		return fmt.Errorf("record backup heartbeat: insert system event: %w", err)
	}
	return nil
}
