package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// The Trash half of VerifyBackupSet (issue #2367): a trashed contact (ADR
// 0037) keeps its photo and the attachment files of its deletion batch until
// purge, so they are required like a live row's. See the package comment in
// backup_completeness.go.

// Decrypter opens one at-rest-encrypted column value (an *atrest.Opener).
type Decrypter interface {
	Decrypt(stored string) (string, error)
}

// SnapshotOpener builds the Decrypter for a snapshot from its read-only
// handle, so the DEK comes from the snapshot's own data_encryption_keys row.
// It is a parameter rather than a direct atrest call because atrest's own
// tests import this package.
type SnapshotOpener func(ctx context.Context, db *sql.DB) (Decrypter, error)

// plaintext is the Decrypter used without a snapshot opener.
type plaintext struct{}

func (plaintext) Decrypt(stored string) (string, error) { return stored, nil }

// keyHint ends every snapshot-read failure: the usual cause is a missing or
// different master key.
const keyHint = "set DATA_ENCRYPTION_KEY (or JWT_SECRET_KEY) to the server's value to verify the Trash's files"

// trashedContact is one Trash entry: a soft-deleted contact with a deletion
// snapshot.
type trashedContact struct {
	userID       int64
	owner        string
	photo, thumb string
	payload      string
}

// trashedContactRefs returns the files the Trash still owns: each trashed
// contact's photo, and the attachments of its own deletion batch
// (payload.soft["attachments"], the set purge removes). Rows the user deleted
// on their own are not in a batch, and their files are already gone. A
// snapshot taken before the Trash existed has nothing to reconcile.
func trashedContactRefs(db *sql.DB, openSnapshot SnapshotOpener) (photos, attachments []MissingRef, err error) {
	ctx := context.Background()
	trashed, err := listTrashedContacts(ctx, db)
	if err != nil || len(trashed) == 0 {
		return nil, nil, err
	}
	var opener Decrypter = plaintext{}
	if openSnapshot != nil {
		if opener, err = openSnapshot(ctx, db); err != nil {
			return nil, nil, fmt.Errorf("verify trashed contacts: %w", err)
		}
	}
	for _, tc := range trashed {
		photos = appendPhotoRefs(photos, tc.owner, tc.photo, tc.thumb)
		ids, err := batchAttachmentIDs(opener, tc)
		if err != nil {
			return nil, nil, err
		}
		refs, err := batchAttachmentRefs(ctx, db, tc.userID, ids)
		if err != nil {
			return nil, nil, err
		}
		attachments = append(attachments, refs...)
	}
	return photos, attachments, nil
}

func listTrashedContacts(ctx context.Context, db *sql.DB) ([]trashedContact, error) {
	var out []trashedContact
	err := scanRows(ctx, db, func(rows *sql.Rows) error {
		var tc trashedContact
		err := rows.Scan(&tc.userID, &tc.owner, &tc.photo, &tc.thumb, &tc.payload)
		out = append(out, tc)
		return err
	}, `
		SELECT c.user_id, COALESCE(NULLIF(c.vcard_uid, ''), 'contact#' || c.id),
		       COALESCE(c.photo, ''), COALESCE(c.photo_thumbnail, ''), s.payload
		FROM contact_deletion_snapshots s
		JOIN contacts c ON c.id = s.contact_id AND c.user_id = s.user_id
		WHERE c.deleted_at IS NOT NULL
		ORDER BY c.id`)
	// Only the Trash's own table may be absent (a pre-v1.6.0 snapshot);
	// livePhotoRefs has already read contacts.
	if err != nil && strings.Contains(err.Error(), "no such table: contact_deletion_snapshots") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("trashed contacts: %w", err)
	}
	return out, nil
}

// batchAttachmentIDs decrypts and decodes one deletion snapshot's payload.
// An unreadable payload fails the check rather than silently trusting it.
func batchAttachmentIDs(opener Decrypter, tc trashedContact) ([]string, error) {
	plain, err := opener.Decrypt(tc.payload)
	if err != nil {
		return nil, fmt.Errorf("trashed contact %s: decrypt deletion snapshot (%s): %w", tc.owner, keyHint, err)
	}
	var payload struct {
		Soft map[string][]string `json:"soft"`
	}
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return nil, fmt.Errorf("trashed contact %s: decode deletion snapshot (%s): %w", tc.owner, keyHint, err)
	}
	return payload.Soft["attachments"], nil
}

// batchAttachmentRefs returns a reference per attachment row of the given
// ids, scoped to the snapshot's user.
func batchAttachmentRefs(ctx context.Context, db *sql.DB, userID int64, ids []string) ([]MissingRef, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	var refs []MissingRef
	// #nosec G202 -- only "?" placeholders are concatenated; the ids are bound.
	query := `SELECT id, stored_name FROM attachments WHERE user_id = ? AND stored_name <> '' AND id IN (?` + strings.Repeat(",?", len(ids)-1) + `) ORDER BY id`
	err := scanRows(ctx, db, func(rows *sql.Rows) error {
		var id int64
		var storedName string
		err := rows.Scan(&id, &storedName)
		refs = append(refs, MissingRef{Kind: "attachment", StoredName: storedName, Owner: fmt.Sprintf("attachment#%d", id)})
		return err
	}, query, args...)
	return refs, err
}

// scanRows runs query and calls scan once per row, returning the first error.
func scanRows(ctx context.Context, db *sql.DB, scan func(*sql.Rows) error, query string, args ...any) error {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil { // # pragma: no cover — the queries' column types (INTEGER, TEXT) match the scan targets
			return fmt.Errorf("scan row: %w", err)
		}
	}
	return rows.Err()
}
