// Command backup writes a consistent online snapshot of the SQLite database
// using VACUUM INTO, without stopping the server, and signs it (issue #943). It
// exists for the operator workflow that `make backup` drives (see
// backend/Makefile and docs/deployment.md's Backups section, ticket N6).
//
// It owns no backup logic of its own: VACUUM INTO and the integrity check live
// in database.BackupSnapshot, the detached manifest and HMAC live in
// database.SignBackup, and the freshness heartbeat lives in
// database.RecordOperatorBackupCompleted. This CLI is argument/env plumbing
// plus the ordering of those steps.
//
// Source is read from SQLITE_DB_PATH (same variable the server and the rest
// of the Makefile read, so this CLI can never drift onto a different file).
// The output path is, in precedence order: a positional argument, BACKUP_PATH,
// or a timestamped sibling of the source database.
//
// The snapshot is signed with a key derived from the at-rest master key
// (atrest.BackupSigningKey — DATA_ENCRYPTION_KEY, DATA_ENCRYPTION_KEY_FILE, or
// the JWT_SECRET_KEY fallback), so the command must run with the same key
// environment the server uses. With no key configured it refuses rather than
// writing an unauthenticated snapshot: `make backup-verify` will not accept an
// unsigned set without an explicit opt-out.
package main

import (
	"fmt"
	"io"
	"os"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so its exit paths are testable (mirrors
// cmd/backupverify and cmd/migrate).
func run(args []string, out, errOut io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintf(errOut, "backup: usage: backup [OUTPUT_PATH] (or set BACKUP_PATH); got unexpected extra argument %q\n", args[1])
		return 2
	}

	src := dbPath()
	outPath := resolveOutputPath(args, src)

	// Resolve the signing key before writing anything: an unsigned snapshot is
	// the authenticity gap issue #943 closes, so fail closed rather than leave
	// one behind.
	signingKey, err := atrest.BackupSigningKey()
	if err != nil {
		fmt.Fprintln(errOut, "backup:", err)
		return 2
	}
	if len(signingKey) == 0 {
		fmt.Fprintln(errOut, "backup: no at-rest master key configured — set DATA_ENCRYPTION_KEY (preferred) or JWT_SECRET_KEY to the same value the server uses, or the snapshot cannot be signed (issue #943)")
		return 2
	}

	if err := database.BackupSnapshot(src, outPath); err != nil {
		fmt.Fprintln(errOut, "backup:", err)
		return 1
	}
	if err := database.SignBackup(outPath, signingKey); err != nil {
		fmt.Fprintln(errOut, "backup:", err)
		return 1
	}

	// The snapshot is written and signed — the backup itself succeeded. The
	// heartbeat only feeds the backup_stale freshness alert (issue #943), so a
	// failure to record it is a warning, not a failure of the backup.
	if err := database.RecordOperatorBackupCompleted(src, outPath); err != nil {
		fmt.Fprintf(errOut, "backup: warning: snapshot written and signed, but the backup heartbeat could not be recorded: %v\n", err)
	}

	fmt.Fprintf(out, "Backed up %s to %s\n", src, outPath)
	return 0
}

// resolveOutputPath applies the documented precedence: positional argument,
// then BACKUP_PATH, then the timestamped sibling of the source.
func resolveOutputPath(args []string, src string) string {
	if len(args) == 1 && args[0] != "" {
		return args[0]
	}
	if env := os.Getenv("BACKUP_PATH"); env != "" {
		return env
	}
	return database.DefaultBackupPath(src)
}

func dbPath() string {
	if p := os.Getenv("SQLITE_DB_PATH"); p != "" {
		return p
	}
	return defaultDBPath
}
