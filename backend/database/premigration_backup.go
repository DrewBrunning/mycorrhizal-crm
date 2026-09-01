package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mycorrhizal/logger"
)

// preMigrationBackupDirEnvVar moves the directory the mandatory pre-migration
// snapshot (issue #530) is written to. It is the ONLY knob: there is no setting
// that turns the backup off. An unset value means "a `pre-migration` sibling of
// the live database" (see preMigrationBackupDir).
const preMigrationBackupDirEnvVar = "MYCORRHIZAL_PRE_MIGRATION_BACKUP_DIR"

// ErrPreMigrationBackupFailed is the error the file-level migrate path
// (migrateFileWithPreBackup, reached from InitDB and MigrateUp) returns when it
// cannot take the mandatory pre-migration backup. It is a stable, assertable
// sentinel in the same family as ErrDirtyMigration / ErrSubFloorMigration: the
// server start path logs it via logger.Fatal ("Failed to initialize database")
// and the migrate CLI prints it, both before any migration runs and with the
// database left exactly as it was.
//
// The backup is load-bearing, not advisory (issue #530): downgrade is
// unsupported, so this snapshot — install the previous release, restore it — is
// the only way back to the pre-upgrade version if the new release is bad. A
// missing or unwritable target therefore fails the upgrade closed rather than
// migrating unbacked.
type ErrPreMigrationBackupFailed struct {
	Dir   string
	cause error
}

func (e *ErrPreMigrationBackupFailed) Error() string {
	return fmt.Sprintf(
		"could not take the mandatory pre-migration backup into %q: %v. "+
			"Downgrade is unsupported, so this snapshot is the only rollback point if the upgrade is bad "+
			"(see docs/operations/migration-recovery.md). Refusing to migrate (fail-closed) — the database is untouched. "+
			"Make that directory writable, or set %s to a writable path, then start again.",
		e.Dir, e.cause, preMigrationBackupDirEnvVar,
	)
}

func (e *ErrPreMigrationBackupFailed) Unwrap() error { return e.cause }

// preMigrationBackupDir resolves where the pre-migration snapshot is written:
// MYCORRHIZAL_PRE_MIGRATION_BACKUP_DIR when set, else a "pre-migration" sibling
// of the live database. The dedicated subdirectory keeps rollback points
// physically apart from routine `make backup` output, so a backup-rotation cron
// pointed at the routine directory cannot sweep the last rollback point
// (issue #530 action 2).
func preMigrationBackupDir(dbPath string) string {
	if d := strings.TrimSpace(os.Getenv(preMigrationBackupDirEnvVar)); d != "" {
		return d
	}
	return filepath.Join(filepath.Dir(dbPath), "pre-migration")
}

// preMigrationBackupPrefix is the filename stem, minus the timestamp, for a
// snapshot taken before the from->to upgrade: `<db-stem>-pre-migration-<from>-to-<to>-`.
// It both names a new snapshot and, globbed with `*.db`, finds an existing one
// on a restart.
func preMigrationBackupPrefix(dbPath string, from, to uint) string {
	base := filepath.Base(dbPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return fmt.Sprintf("%s-pre-migration-%d-to-%d-", stem, from, to)
}

// takePreMigrationBackup writes a verified SQLite snapshot of dbPath before any
// pending migration is applied and returns its path. It is mandatory and
// fail-closed: any failure is an ErrPreMigrationBackupFailed and the caller
// applies no migration.
//
// It is idempotent per (from, to): a container that crash-loops after the
// snapshot but before the migrations finish must not fail closed on the retry,
// nor write a fresh full-size copy each loop. An existing snapshot for the same
// pair that still passes PRAGMA integrity_check is reused.
//
// Scope is the database file only. Migrations never touch PROFILE_PHOTO_DIR or
// ATTACHMENTS_DIR, so a full rollback still restores those from the routine
// three-piece backup — a deliberate boundary, documented in
// docs/operations/migration-recovery.md.
func takePreMigrationBackup(dbPath string, from, to uint) (string, error) {
	dir := preMigrationBackupDir(dbPath)
	prefix := preMigrationBackupPrefix(dbPath, from, to)

	if existing := findReusablePreMigrationBackup(dir, prefix); existing != "" {
		logger.Info().
			Str(logger.FieldEvent, "pre_migration_backup").
			Str(logger.FieldComponent, "migration").
			Uint("from_version", from).
			Uint("to_version", to).
			Str("path", existing).
			Bool("reused", true).
			Msg("reusing existing pre-migration backup")
		return existing, nil
	}

	target := filepath.Join(dir, prefix+time.Now().UTC().Format("20060102-150405")+".db")
	if err := BackupSnapshot(dbPath, target); err != nil {
		return "", &ErrPreMigrationBackupFailed{Dir: dir, cause: err}
	}

	var size int64
	if fi, statErr := os.Stat(target); statErr == nil {
		size = fi.Size()
	}
	logger.Info().
		Str(logger.FieldEvent, "pre_migration_backup").
		Str(logger.FieldComponent, "migration").
		Uint("from_version", from).
		Uint("to_version", to).
		Str("path", target).
		Int64("bytes", size).
		Bool("reused", false).
		Msg("pre-migration backup written")
	recordPreMigrationBackupEvent(dbPath, from, to, target)
	return target, nil
}

// findReusablePreMigrationBackup returns the path of an existing snapshot in dir
// whose name starts with prefix and which still passes an integrity check, or ""
// when there is none to reuse.
func findReusablePreMigrationBackup(dir, prefix string) string {
	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*.db"))
	if err != nil { // # pragma: no cover -- Glob only errors on a malformed pattern, and this one is built from a literal
		return ""
	}
	for _, m := range matches {
		if verifyBackup(m) == nil {
			return m
		}
	}
	return ""
}

// recordPreMigrationBackupEvent inserts one operational-timeline row for the
// snapshot, swallowing every error: the system_events table does not exist
// until migration 000038, so a v0.6.0-schema first hop legitimately has nowhere
// to write it, and a diagnostic write must never be able to fail a boot. It
// reuses the `backup_completed` event_type (the CHECK vocabulary is fixed and
// mirrored in models/system_event.go + the frontend) tagged with
// component=migration so it is distinguishable from a routine backup and does
// not feed the backup subsystem's freshness check. Mirrors recordMigrationEvent.
func recordPreMigrationBackupEvent(dbPath string, from, to uint, path string) {
	defer func() { _ = recover() }()
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil { // # pragma: no cover -- sql.Open is lazy; a file DSN does not fail here
		return
	}
	defer sqlDB.Close()
	now := time.Now().UTC()
	_, err = sqlDB.Exec(
		`INSERT INTO system_events (created_at, occurred_at, event_type, severity, component, operation, result, detail)
		 VALUES (?, ?, 'backup_completed', 'info', 'migration', 'pre_migration_backup', 'success', ?)`,
		now, now, fmt.Sprintf("from_version=%d to_version=%d path=%s", from, to, logger.SanitizeLogField(path)),
	)
	if err != nil {
		logger.Debug().Err(err).Msg("could not record pre-migration backup system event (pre-000038 schema?)")
	}
}
