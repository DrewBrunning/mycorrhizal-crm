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
// file (VACUUM INTO produces a journal_mode=delete database — no -wal/-shm
// sidecars) that is valid even while the server is running.
//
// A plain `cp` of the .db file is unsafe for a live instance, because the
// database runs in WAL mode: committed writes can sit in a -wal sidecar that a
// file copy never sees, and a copy taken mid-write can be torn. VACUUM INTO
// reads through the WAL, so the snapshot it produces is complete and
// transactionally consistent regardless of checkpoint state. The procedure
// still runs a best-effort `wal_checkpoint(PASSIVE)` first (see checkpoint) —
// it is a tidy-up, not a correctness requirement.
//
// The snapshot is built at a unique temp name beside outPath and only moved
// into place once it has passed PRAGMA integrity_check. That guarantees:
//
//   - a failed run never leaves a partial file at outPath, and never deletes
//     a file this invocation did not create (the only file ever removed is
//     the temp this call made);
//   - an existing output is never overwritten: the pre-check below and the
//     final no-overwrite link both refuse instead.
func BackupSnapshot(srcPath, outPath string) error {
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup source %q does not exist", srcPath)
		}
		return fmt.Errorf("stat backup source %q: %w", srcPath, err)
	}
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("backup output %q already exists; refusing to overwrite (pick a fresh BACKUP_PATH)", outPath)
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create backup output directory %q: %w", dir, err)
	}

	// VACUUM INTO refuses to overwrite, so it cannot target outPath directly.
	// CreateTemp reserves a unique name that is guaranteed free the moment we
	// remove it; the only writer to that name afterwards is this invocation.
	tmpPath, err := reserveTempPath(dir, filepath.Base(outPath))
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	sqlDB, err := sql.Open("sqlite", openDSN(srcPath))
	if err != nil {
		return fmt.Errorf("open %q for backup: %w", srcPath, err)
	}
	defer sqlDB.Close()

	if err := backupSnapshot(sqlDB, tmpPath); err != nil {
		return err
	}
	if err := verifyBackup(tmpPath); err != nil {
		return err
	}

	// os.Link fails with EEXIST rather than overwriting, so a file that
	// appeared at outPath after the pre-check above is left alone.
	if err := os.Link(tmpPath, outPath); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("backup output %q already exists; refusing to overwrite (pick a fresh BACKUP_PATH)", outPath)
		}
		return fmt.Errorf("place backup at %q: %w", outPath, err)
	}
	return nil
}

// backupSnapshot runs the checkpoint-then-VACUUM-INTO sequence on an
// already-open connection.
func backupSnapshot(sqlDB *sql.DB, outPath string) error {
	if err := checkpoint(sqlDB); err != nil {
		return err
	}

	// VACUUM INTO takes a filename literal, not a bound parameter, so the path
	// is inlined and single quotes are escaped (the only escaping SQLite needs
	// inside a string literal). The path is operator-supplied, never user input.
	escaped := strings.ReplaceAll(outPath, "'", "''")
	if _, err := sqlDB.Exec("VACUUM INTO '" + escaped + "'"); err != nil { // #nosec G202 -- operator-supplied path, quotes escaped; not user input
		return fmt.Errorf("VACUUM INTO %q: %w", outPath, err)
	}
	return nil
}

// checkpoint runs wal_checkpoint(PASSIVE), folding as much of the WAL into the
// main file as it can without blocking. VACUUM INTO reads through the WAL, so
// the snapshot is complete even if the checkpoint folds nothing — this is a
// tidy-up, not a correctness requirement.
//
// PASSIVE is deliberate: TRUNCATE (the ticket's original suggestion) reports
// busy=1 the moment any connection holds a read transaction, which would make
// the "online backup" fail exactly when a live server is under load — the one
// situation it exists for. PASSIVE never returns busy.
func checkpoint(sqlDB *sql.DB) error {
	var busy, log, checkpointed int
	if err := sqlDB.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &log, &checkpointed); err != nil {
		return fmt.Errorf("wal_checkpoint(PASSIVE): %w", err)
	}
	return nil
}

// verifyBackup runs PRAGMA integrity_check on the just-written snapshot. The
// caller owns the temp file's lifecycle, so this never deletes anything
// itself. A plain (non-DSN) open is used: the VACUUM INTO output is a
// journal_mode=delete database, so no WAL pragma is needed and no sidecars
// are created next to it.
func verifyBackup(tmpPath string) error {
	sqlDB, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return fmt.Errorf("open backup %q for verification: %w", tmpPath, err)
	}
	defer sqlDB.Close()

	var result string
	if err := sqlDB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("integrity check of backup %q failed: %w", tmpPath, err)
	}
	if result != "ok" {
		return fmt.Errorf("backup %q failed integrity check: %s", tmpPath, result)
	}
	return nil
}

// reserveTempPath returns a unique file path in dir that is free for the
// caller to use, by asking the OS for a fresh name and immediately releasing
// it. The name stays unique in practice because no other process can guess
// the random suffix in the gap.
func reserveTempPath(dir, base string) (string, error) {
	f, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("reserve backup temp name in %q: %w", dir, err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
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
