// Command backupverify reconciles an assembled backup set — a SQLite database
// plus the profile-photo and attachment directories copied beside it — and
// reports any live row whose backing file is not in the set (BACKUP-02,
// issue #454).
//
// A backup is three independent pieces (see docs/deployment.md's Backups
// section): the database (owned by `make backup` / database.BackupSnapshot),
// the photo directory, and the attachment directory (both the operator's to
// copy). `make backup` verifies the database it writes; nothing verified that
// the copied directories still line up with it. This CLI closes that gap: run
// it against a fresh backup set to turn "I have a backup" into "I have a
// restorable backup".
//
// Like cmd/backup and cmd/migrate it owns no logic of its own — the work is in
// database.VerifyBackupSet and this is only argument/env plumbing.
//
// Paths, in precedence order:
//
//	database:    positional argument, else SQLITE_DB_PATH, else mycorrhizal.db
//	photos:      BACKUP_PHOTOS_DIR, else PROFILE_PHOTO_DIR
//	attachments: BACKUP_ATTACHMENTS_DIR, else ATTACHMENTS_DIR
//
// Exit status 0 means the set is complete; 1 means a live row's file is
// missing (report.Complete() is false — orphan files alone never produce
// this); 2 means the command could not run at all (bad usage, unreadable
// database or directories).
package main

import (
	"fmt"
	"io"
	"os"

	"mycorrhizal/database"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable (mirrors
// cmd/genschema and cmd/gencontract's shape).
func run(args []string, out, errOut io.Writer) int {
	dbPath, photoDir, attachmentsDir, err := resolvePaths(args)
	if err != nil {
		fmt.Fprintln(errOut, "backupverify:", err)
		return 2
	}

	report, err := database.VerifyBackupSet(dbPath, photoDir, attachmentsDir)
	if err != nil {
		fmt.Fprintln(errOut, "backupverify:", err)
		return 2
	}

	fmt.Fprintf(out, "database:    %s\nphotos:      %s\nattachments: %s\n\n%s\n", dbPath, photoDir, attachmentsDir, report.String())
	if !report.Complete() {
		return 1
	}
	return 0
}

// resolvePaths applies the precedence documented in the package comment.
func resolvePaths(args []string) (dbPath, photoDir, attachmentsDir string, err error) {
	if len(args) > 1 {
		return "", "", "", fmt.Errorf("usage: backupverify [DB_PATH] (or set SQLITE_DB_PATH); got unexpected extra argument %q", args[1])
	}

	dbPath = defaultDBPath
	if len(args) == 1 && args[0] != "" {
		dbPath = args[0]
	} else if env := os.Getenv("SQLITE_DB_PATH"); env != "" {
		dbPath = env
	}

	photoDir = firstNonEmpty(os.Getenv("BACKUP_PHOTOS_DIR"), os.Getenv("PROFILE_PHOTO_DIR"))
	attachmentsDir = firstNonEmpty(os.Getenv("BACKUP_ATTACHMENTS_DIR"), os.Getenv("ATTACHMENTS_DIR"))
	if photoDir == "" || attachmentsDir == "" {
		return "", "", "", fmt.Errorf("set the photo directory (BACKUP_PHOTOS_DIR or PROFILE_PHOTO_DIR) and the attachments directory (BACKUP_ATTACHMENTS_DIR or ATTACHMENTS_DIR)")
	}
	return dbPath, photoDir, attachmentsDir, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
