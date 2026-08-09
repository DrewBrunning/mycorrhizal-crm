package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupSnapshot writes a consistent online snapshot of the SQLite database at
// srcPath to outPath, using VACUUM INTO so the copy is a single self-contained
// file (no -wal/-shm sidecars) that is valid even while the server is running.
//
// Two things make a plain `cp` of the .db file unsafe for a live instance
// (docs/deployment.md, ticket N6), and this function exists so the documented
// `make backup` path never does either:
//
//   - journal_mode is WAL (openDSN). A running server has committed frames
//     sitting in the -wal file that a file copy never sees; restoring such a
//     copy silently loses every write since the last checkpoint.
//   - VACUUM INTO itself can miss uncheckpointed WAL frames, so the backup
//     procedure checkpoints first (PRAGMA wal_checkpoint(TRUNCATE), which
//     folds the whole WAL into the main file and truncates it), then snapshots.
//
// The backup file is verified with PRAGMA integrity_check before it is
// returned as success; a corrupt backup is deleted rather than left on disk
// pretending to be one.
func BackupSnapshot(srcPath, outPath string) error {
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup source %q does not exist", srcPath)
		}
		return fmt.Errorf("stat backup source %q: %w", srcPath, err)
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("backup output %q already exists; refusing to overwrite", outPath)
	}

	if dir := filepath.Dir(outPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create backup output directory %q: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", openDSN(srcPath))
	if err != nil {
		return fmt.Errorf("open %q for backup: %w", srcPath, err)
	}
	defer sqlDB.Close()

	if err := backupSnapshot(sqlDB, outPath); err != nil {
		return err
	}
	return verifyBackup(outPath)
}

// backupSnapshot runs the checkpoint-then-VACUUM-INTO sequence on an
// already-open connection. Split out so tests can drive it without re-deriving
// the DSN.
func backupSnapshot(sqlDB *sql.DB, outPath string) error {
	if err := checkpointTruncate(sqlDB); err != nil {
		return err
	}

	// VACUUM INTO takes a filename literal, not a bound parameter, so the path
	// is inlined and single quotes are escaped (the only escaping SQLite needs
	// inside a string literal). outPath is operator-supplied, never user input.
	escaped := strings.ReplaceAll(outPath, "'", "''")
	if _, err := sqlDB.Exec("VACUUM INTO '" + escaped + "'"); err != nil {
		return fmt.Errorf("VACUUM INTO %q: %w", outPath, err)
	}
	return nil
}

// checkpointTruncate folds the WAL into the main database file so the
// subsequent VACUUM INTO sees every committed write. TRUNCATE (not PASSIVE)
// is deliberate: it fails with busy=1 instead of leaving a partial checkpoint,
// which is exactly the state we must not snapshot. A busy result means another
// connection is mid-read; retry briefly, then give up rather than produce a
// backup that is missing recent writes.
func checkpointTruncate(sqlDB *sql.DB) error {
	const attempts = 5
	for i := 0; i < attempts; i++ {
		var busy, log, checkpointed int
		err := sqlDB.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &checkpointed)
		if err != nil {
			return fmt.Errorf("wal_checkpoint(TRUNCATE): %w", err)
		}
		if busy == 0 {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return fmt.Errorf("wal_checkpoint(TRUNCATE) stayed busy (%d attempts): the database has an active reader; retry when read load is lower", attempts)
}

// verifyBackup opens the just-written snapshot and runs PRAGMA integrity_check,
// deleting it on any failure so a bad backup is never mistaken for a good one.
func verifyBackup(outPath string) error {
	sqlDB, err := sql.Open("sqlite", openDSN(outPath))
	if err != nil {
		os.Remove(outPath)
		return fmt.Errorf("open backup %q for verification: %w", outPath, err)
	}
	defer sqlDB.Close()

	var result string
	if err := sqlDB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		os.Remove(outPath)
		return fmt.Errorf("integrity check of backup %q failed: %w", outPath, err)
	}
	if result != "ok" {
		os.Remove(outPath)
		return fmt.Errorf("backup %q failed integrity check: %s", outPath, result)
	}
	return nil
}

// DefaultBackupPath derives the output path used by cmd/backup when neither an
// argument nor BACKUP_PATH is given: a timestamped sibling of the source, so a
// Docker deployment's `make backup` lands the snapshot inside the same bind
// mount as the live database.
func DefaultBackupPath(srcPath string) string {
	dir := filepath.Dir(srcPath)
	base := filepath.Base(srcPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	ts := time.Now().Format("20060102-150405")
	return filepath.Join(dir, stem+"-"+ts+".db")
}
