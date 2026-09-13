// Command backupverify reconciles an assembled backup set — a SQLite database
// plus the profile-photo and attachment directories copied beside it — and
// authenticates the database piece, reporting any live row whose backing file
// is not in the set (BACKUP-02, issue #454; snapshot signatures issue #943).
//
// A backup is three independent pieces (see docs/deployment.md's Backups
// section): the database (owned by `make backup` / database.BackupSnapshot and
// signed by database.SignBackup), the photo directory, and the attachment
// directory (both the operator's to copy). `make backup` verifies and signs the
// database it writes; nothing verified that the copied directories still line
// up with it. This CLI closes that gap: run it against a fresh backup set to
// turn "I have a backup" into "I have an authenticated, restorable backup".
//
// Like cmd/backup and cmd/migrate it owns no logic of its own — the work is in
// database.VerifyBackupSet and database.VerifyBackupSignature, and this is only
// argument/env plumbing.
//
// Paths, in precedence order:
//
//	database:    positional argument, else SQLITE_DB_PATH, else mycorrhizal.db
//	photos:      BACKUP_PHOTOS_DIR, else PROFILE_PHOTO_DIR
//	attachments: BACKUP_ATTACHMENTS_DIR, else ATTACHMENTS_DIR
//
// The signature is verified with a key derived from the at-rest master key, so
// verification must run with the same key environment the server (and `make
// backup`) use. BACKUP_ALLOW_UNSIGNED=1 tolerates an unsigned (legacy) set
// instead of failing closed; a present-but-invalid manifest still fails.
//
// Exit status 0 means the set is complete and authenticated; 1 means it is
// not (a missing attachment/photo file, an unsigned set without the opt-out,
// or a signature that does not verify); 2 means the command could not run at
// all (bad usage, unreadable database or directories, no signing key).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

// backupAllowUnsignedEnv is the opt-out for reconciling a set taken before
// snapshot signing existed. It tolerates a *missing* manifest only; a manifest
// that is present and does not verify always fails.
const backupAllowUnsignedEnv = "BACKUP_ALLOW_UNSIGNED"

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

	// Reconcile the three-piece set first: it distinguishes "cannot run"
	// (unreadable database/directories → exit 2) from "a hole in the set"
	// (→ exit 1). Authentication follows, so a tampered snapshot is reported
	// even when its rows happen to reconcile.
	report, err := database.VerifyBackupSet(dbPath, photoDir, attachmentsDir)
	if err != nil {
		fmt.Fprintln(errOut, "backupverify:", err)
		return 2
	}

	sigDetail, sigErr, sigCode := checkSnapshotSignature(dbPath)
	if sigErr != nil {
		fmt.Fprintln(errOut, "backupverify:", sigErr)
		return sigCode
	}

	fmt.Fprintf(out, "database:    %s\nphotos:      %s\nattachments: %s\nsignature:   %s\n\n%s\n",
		dbPath, photoDir, attachmentsDir, sigDetail, report.String())
	if !report.Complete() {
		return 1
	}
	return 0
}

// checkSnapshotSignature authenticates the database piece against its detached
// manifest. It returns the human-facing detail line on success, or an error and
// the exit code to report: 1 when the set is not authenticated (missing or
// invalid signature), 2 when authentication could not be attempted (no key).
func checkSnapshotSignature(dbPath string) (detail string, err error, exitCode int) {
	allowUnsigned := os.Getenv(backupAllowUnsignedEnv) == "1"

	signingKey, keyErr := atrest.BackupSigningKey()
	if keyErr != nil {
		return "", keyErr, 2
	}

	manifestPath := database.ManifestPath(dbPath)
	_, statErr := os.Stat(manifestPath)
	if os.IsNotExist(statErr) {
		if allowUnsigned {
			return "unsigned (allowed by BACKUP_ALLOW_UNSIGNED=1)", nil, 0
		}
		return "", fmt.Errorf("snapshot is unsigned: no manifest at %q — every backup must be signed (issue #943); set %s=1 to reconcile a legacy unsigned set", manifestPath, backupAllowUnsignedEnv), 1
	}
	if statErr != nil {
		return "", fmt.Errorf("stat manifest %q: %w", manifestPath, statErr), 2
	}

	if len(signingKey) == 0 {
		if allowUnsigned {
			return "not verified (no signing key; BACKUP_ALLOW_UNSIGNED=1)", nil, 0
		}
		return "", errors.New("no at-rest master key configured — set DATA_ENCRYPTION_KEY (preferred) or JWT_SECRET_KEY to the same value the server uses, or set BACKUP_ALLOW_UNSIGNED=1 to reconcile an unsigned legacy set"), 2
	}

	if err := database.VerifyBackupSignature(dbPath, signingKey); err != nil {
		return "", err, 1
	}
	return "verified (hmac-sha256)", nil, 0
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
