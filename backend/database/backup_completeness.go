package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A backup is three independent pieces (see docs/deployment.md's Backups
// section): the SQLite database, the profile-photo directory
// (PROFILE_PHOTO_DIR), and the attachments directory (ATTACHMENTS_DIR).
// BackupSnapshot / cmd/backup own only the database; the two directories are
// the operator's to copy. Nothing coordinated the three, and nothing verified
// that a copied set is internally consistent — a database that restores
// perfectly can still hold an attachment row whose file was never in the
// backup, and the failure is silent until someone opens the attachment.
//
// VerifyBackupSet is that check (BACKUP-02, issue #454). Given an assembled
// backup set — a database file plus the photo and attachment directories that
// were copied beside it — it reconciles every live attachment and profile-photo
// row against the files present and reports:
//
//   - missing files: a live row that points at a file absent from the set.
//     This is a real defect; report.Complete() is false when any exist.
//   - orphan files: a file in a directory with no live row that owns it. This
//     is informational — it distinguishes a stale directory from a lost one,
//     and feeds the orphan detection in DB-01 (issue #460) — never a failure
//     on its own.
//
// Soft-deleted rows are deliberately excluded: DeleteContact and
// deleteContactAttachmentFiles (controllers/contact_controller.go) remove the
// on-disk file at delete time, so a soft-deleted row with no file is the
// expected steady state, not a hole in the backup.

// MissingRef names one live database row whose backing file is absent from the
// backup set, so an operator (or a failing restore drill) can point at exactly
// what is gone rather than "a file is missing".
type MissingRef struct {
	// Kind is "attachment" or "contact_photo".
	Kind string
	// StoredName is the on-disk filename the row references, relative to its
	// directory (attachments.stored_name, or contacts.photo /
	// contacts.photo_thumbnail).
	StoredName string
	// Owner identifies the row: the attachment's id, or the contact's
	// vcard_uid (falling back to "contact#<id>" when the uid is null).
	Owner string
}

func (m MissingRef) String() string {
	return fmt.Sprintf("%s %s (owner %s)", m.Kind, m.StoredName, m.Owner)
}

// BackupSetReport is the result of VerifyBackupSet.
type BackupSetReport struct {
	// AttachmentsScanned / PhotosScanned are the live row counts each check
	// looked at, so "0 missing" can be told apart from "nothing checked".
	AttachmentsScanned int
	PhotosScanned      int

	MissingAttachments []MissingRef
	MissingPhotos      []MissingRef

	OrphanAttachmentFiles []string
	OrphanPhotoFiles      []string
}

// Complete reports whether every live attachment and profile-photo row in the
// database resolves to a file present in the backup set. Orphan files do not
// affect it.
func (r *BackupSetReport) Complete() bool {
	return len(r.MissingAttachments) == 0 && len(r.MissingPhotos) == 0
}

// TotalMissing is the number of live rows with no backing file.
func (r *BackupSetReport) TotalMissing() int {
	return len(r.MissingAttachments) + len(r.MissingPhotos)
}

// TotalOrphans is the number of files with no owning live row.
func (r *BackupSetReport) TotalOrphans() int {
	return len(r.OrphanAttachmentFiles) + len(r.OrphanPhotoFiles)
}

// String renders a human-readable summary for the CLI and the restore-drill
// failure detail.
func (r *BackupSetReport) String() string {
	var b strings.Builder
	if r.Complete() {
		fmt.Fprintf(&b, "backup set is complete: %d attachment file(s) and %d photo file(s) all present",
			r.AttachmentsScanned, r.PhotosScanned)
	} else {
		fmt.Fprintf(&b, "backup set is INCOMPLETE: %d of %d attachment file(s) and %d of %d photo file(s) missing",
			len(r.MissingAttachments), r.AttachmentsScanned, len(r.MissingPhotos), r.PhotosScanned)
	}
	for _, m := range r.MissingAttachments {
		fmt.Fprintf(&b, "\n  missing attachment: %s (owner %s)", m.StoredName, m.Owner)
	}
	for _, m := range r.MissingPhotos {
		fmt.Fprintf(&b, "\n  missing photo: %s (owner %s)", m.StoredName, m.Owner)
	}
	if n := r.TotalOrphans(); n > 0 {
		fmt.Fprintf(&b, "\n%d orphan file(s) present with no owning row (informational):", n)
		for _, f := range r.OrphanAttachmentFiles {
			fmt.Fprintf(&b, "\n  orphan attachment file: %s", f)
		}
		for _, f := range r.OrphanPhotoFiles {
			fmt.Fprintf(&b, "\n  orphan photo file: %s", f)
		}
	}
	return b.String()
}

// VerifyBackupSet reconciles the database at dbPath against the photo and
// attachment directories that make up the rest of the backup set. See the
// package-level comment above for what "missing" and "orphan" mean and why
// soft-deleted rows are skipped.
//
// The database is opened read-only through the app's own connection pragmas
// (openDSN) via a raw *sql.DB — deliberately not GORM, matching IntegrityCheck
// — and no migrations are run: a backup set must be verifiable without mutating
// the snapshot.
func VerifyBackupSet(dbPath, photoDir, attachmentsDir string) (*BackupSetReport, error) {
	if photoDir == "" || attachmentsDir == "" {
		return nil, fmt.Errorf("verify backup set: both a photo directory and an attachments directory are required")
	}
	// sql.Open is lazy and SQLite would materialise an empty database for a
	// typo'd path, silently reporting every row as "missing"; refuse up front,
	// exactly like IntegrityCheck.
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("open %q: %w", dbPath, err)
	}
	if err := statDir(photoDir, "photo"); err != nil {
		return nil, err
	}
	if err := statDir(attachmentsDir, "attachments"); err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil { // # pragma: no cover — a file DSN cannot fail to open here; errors surface at first query
		return nil, fmt.Errorf("open %q: %w", dbPath, err)
	}
	defer sqlDB.Close()

	report := &BackupSetReport{}

	attRefs, err := liveAttachmentRefs(sqlDB)
	if err != nil {
		return nil, err
	}
	report.AttachmentsScanned = len(attRefs)
	report.MissingAttachments = missingRefs(attRefs, attachmentsDir)
	orphanAtt, err := orphanFiles(attachmentsDir, referencedNames(attRefs))
	if err != nil { // # pragma: no cover — orphanFiles can only fail via the TOCTOU race noted on its own os.ReadDir call
		return nil, err
	}
	report.OrphanAttachmentFiles = orphanAtt

	photoRefs, err := livePhotoRefs(sqlDB)
	if err != nil {
		return nil, err
	}
	// PhotosScanned counts distinct contact rows with a photo, not individual
	// file references (a row can name both a full photo and a thumbnail file).
	seenOwners := map[string]bool{}
	for _, ref := range photoRefs {
		seenOwners[ref.Owner] = true
	}
	report.PhotosScanned = len(seenOwners)
	report.MissingPhotos = missingRefs(photoRefs, photoDir)
	orphanPhoto, err := orphanFiles(photoDir, referencedNames(photoRefs))
	if err != nil { // # pragma: no cover — see the matching note on the attachments orphan check above
		return nil, err
	}
	report.OrphanPhotoFiles = orphanPhoto

	return report, nil
}

func statDir(dir, label string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("open %s directory %q: %w", label, dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s directory %q is not a directory", label, dir)
	}
	return nil
}

// liveAttachmentRefs returns one MissingRef-shaped reference per non-deleted
// attachment row (the file may or may not exist yet — missingRefs decides).
func liveAttachmentRefs(db *sql.DB) ([]MissingRef, error) {
	rows, err := db.Query(`SELECT id, stored_name FROM attachments WHERE deleted_at IS NULL AND stored_name <> ''`)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	defer rows.Close()

	var refs []MissingRef
	for rows.Next() {
		var id int64
		var storedName string
		if err := rows.Scan(&id, &storedName); err != nil { // # pragma: no cover — the query's own column types (INTEGER, TEXT) match the scan targets; a driver that returned mismatched types would already have failed migration
			return nil, fmt.Errorf("scan attachment row: %w", err)
		}
		refs = append(refs, MissingRef{
			Kind:       "attachment",
			StoredName: storedName,
			Owner:      fmt.Sprintf("attachment#%d", id),
		})
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — surfaces only a mid-iteration driver/I/O failure, not reproducible without a corrupt disk
		return nil, fmt.Errorf("iterate attachment rows: %w", err)
	}
	return refs, nil
}

// livePhotoRefs returns a reference per on-disk photo file named by a
// non-deleted contact row: the full photo (contacts.photo) and, when it is a
// filename rather than an inline "data:" URL, the thumbnail
// (contacts.photo_thumbnail). Thumbnails are normally stored inline as base64
// data URLs and never touch disk; the filename form is legacy but cheap to
// cover.
func livePhotoRefs(db *sql.DB) ([]MissingRef, error) {
	rows, err := db.Query(`
		SELECT COALESCE(NULLIF(vcard_uid, ''), 'contact#' || id) AS owner,
		       COALESCE(photo, ''), COALESCE(photo_thumbnail, '')
		FROM contacts
		WHERE deleted_at IS NULL AND (photo IS NOT NULL AND photo <> '')`)
	if err != nil {
		return nil, fmt.Errorf("query contact photos: %w", err)
	}
	defer rows.Close()

	var refs []MissingRef
	for rows.Next() {
		var owner, photo, thumb string
		if err := rows.Scan(&owner, &photo, &thumb); err != nil { // # pragma: no cover — see the matching note in liveAttachmentRefs
			return nil, fmt.Errorf("scan contact photo row: %w", err)
		}
		if photo != "" {
			refs = append(refs, MissingRef{Kind: "contact_photo", StoredName: photo, Owner: owner})
		}
		if thumb != "" && !strings.HasPrefix(thumb, "data:") {
			refs = append(refs, MissingRef{Kind: "contact_photo", StoredName: thumb, Owner: owner})
		}
	}
	if err := rows.Err(); err != nil { // # pragma: no cover — see the matching note in liveAttachmentRefs
		return nil, fmt.Errorf("iterate contact photo rows: %w", err)
	}
	return refs, nil
}

// missingRefs returns the subset of refs whose file is absent from dir.
func missingRefs(refs []MissingRef, dir string) []MissingRef {
	var missing []MissingRef
	for _, ref := range refs {
		if _, err := os.Stat(filepath.Join(dir, filepath.Base(ref.StoredName))); err != nil {
			missing = append(missing, ref)
		}
	}
	return missing
}

// referencedNames is the set of bare filenames the given refs point at.
func referencedNames(refs []MissingRef) map[string]bool {
	names := make(map[string]bool, len(refs))
	for _, ref := range refs {
		names[filepath.Base(ref.StoredName)] = true
	}
	return names
}

// orphanFiles lists the regular files directly in dir whose name is not in
// referenced. The result is sorted for a stable report.
func orphanFiles(dir string, referenced map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil { // # pragma: no cover — VerifyBackupSet's statDir already confirmed dir exists and is a directory immediately before this call; only a TOCTOU race (removed mid-call) reaches here
		return nil, fmt.Errorf("read directory %q: %w", dir, err)
	}
	var orphans []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if referenced[e.Name()] {
			continue
		}
		orphans = append(orphans, e.Name())
	}
	sort.Strings(orphans)
	return orphans, nil
}
