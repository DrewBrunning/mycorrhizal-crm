package services

import (
	"fmt"
	"path/filepath"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/diskspace"

	"gorm.io/gorm"
)

// Capacity preflight for bulk-write operations (issue #498).
//
// A bulk import runs in one transaction. On SQLite that transaction grows the
// database file and its WAL; when the filesystem cannot extend either, the
// write fails mid-transaction. That fails closed (the transaction rolls back)
// but the operator only learns why from a raw SQLite "disk I/O error" / ENOSPC
// deep in the logs, and only after the disk is already full. A cheap statfs
// check before the transaction opens turns that into an actionable 507 while
// nothing has been touched — degrade before you die.

const (
	// importPreflightBytesPerContact is the rough on-disk cost of one imported
	// contact once its row, denormalized columns, canonical-record JSON, FTS
	// entries and audit event are written, plus WAL amplification. A generous
	// over-estimate: the point is to catch "obviously not enough room", not to
	// bill the operation to the byte.
	importPreflightBytesPerContact = 8 << 10 // 8 KiB

	// importPreflightFloorBytes is asked for regardless of contact count —
	// headroom for the WAL, any field definitions the import creates, and the
	// photo files a VCF import writes after the transaction.
	importPreflightFloorBytes = 32 << 20 // 32 MiB
)

// sqliteMainFilePath returns the on-disk path of the connection's main
// database and true, or ("", false) for an in-memory database or when the
// path cannot be determined (in which case the caller skips the preflight).
func sqliteMainFilePath(db *gorm.DB) (string, bool) {
	var path string
	if err := db.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&path).Error; err != nil { // # pragma: no cover — the pragma cannot fail on an open connection; the guard keeps a broken query from blocking an import
		return "", false
	}
	if path == "" {
		return "", false
	}
	return path, true
}

// preflightImportDiskSpace refuses an import of approxContacts contacts with a
// 507 when the database's filesystem plainly does not have room for it. It is
// best-effort: an in-memory database, an unreadable statfs, or an
// undeterminable path all skip the check (the transaction keeps its own
// fail-closed path). approxContacts should be the staged row count.
func preflightImportDiskSpace(db *gorm.DB, approxContacts int) *apperrors.AppError {
	path, ok := sqliteMainFilePath(db)
	if !ok {
		return nil
	}
	if approxContacts < 0 { // # pragma: no cover — callers pass len(...), never negative; a guard against a future caller
		approxContacts = 0
	}
	need := uint64(approxContacts)*importPreflightBytesPerContact + importPreflightFloorBytes
	if err := diskspace.Require(filepath.Dir(path), need); err != nil {
		return apperrors.ErrInsufficientStorage(fmt.Sprintf(
			"not enough disk space to import %d contact(s): %v", approxContacts, err,
		)).WithError(err)
	}
	return nil
}
