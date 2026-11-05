package database_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"mycorrhizal/atrest"
	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2367: since contact Trash (ADR 0037) a deleted contact keeps its
// profile photo and the attachment files of its deletion batch on disk until
// purge, so a Trash restore can bring them back. The verifier skipped every
// soft-deleted row, so a backup set missing those files passed.
//
// These tests are not parallel: arming the process-wide at-rest engine to
// write an encrypted payload must not overlap the package's parallel tests.

// trashContact soft-deletes a contact with a photo, records a deletion
// snapshot storing payload as given (plaintext, or ciphertext from
// encryptedTrash).
func (s *backupSet) trashContact(t *testing.T, payload string) {
	t.Helper()
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo, deleted_at)
		 VALUES (9, 'Trashed', 1, 'uid-trashed', 'trashed_photo.jpg', CURRENT_TIMESTAMP)`).Error)
	require.NoError(t, s.db.Exec(
		`INSERT INTO contact_deletion_snapshots (id, created_at, updated_at, user_id, contact_id, contact_vcard_uid, trashed_at, payload_version, payload)
		 VALUES ('snap-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 1, 9, 'uid-trashed', CURRENT_TIMESTAMP, 1, ?)`, payload).Error)
}

// addDeletedAttachment inserts a soft-deleted attachment row and returns its id.
func (s *backupSet) addDeletedAttachment(t *testing.T, storedName string) int64 {
	t.Helper()
	require.NoError(t, s.db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes, deleted_at)
		 VALUES (1, 'uid-trashed', ?, 'doc.pdf', 'application/pdf', 9, CURRENT_TIMESTAMP)`, storedName).Error)
	var id int64
	require.NoError(t, s.db.Raw(`SELECT id FROM attachments WHERE stored_name = ?`, storedName).Scan(&id).Error)
	return id
}

func batchPayload(attachmentID int64) string {
	return `{"soft":{"attachments":["` + strconv.FormatInt(attachmentID, 10) + `"]},"hard":{},"self_contact":false}`
}

// openerFor is how the CLI and the restore drill bind the verifier to a key.
func openerFor(kek []byte) database.SnapshotOpener {
	return func(ctx context.Context, db *sql.DB) (database.Decrypter, error) {
		return atrest.NewOpener(ctx, db, kek)
	}
}

func (s *backupSet) verifyWithKey(t *testing.T, key []byte) (*database.BackupSetReport, error) {
	t.Helper()
	return database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, "", openerFor(key))
}

func TestVerifyBackupSetRequiresTrashedContactFiles(t *testing.T) {
	s := newBackupSet(t)
	batched := s.addDeletedAttachment(t, "batched-attachment")
	// Deleted on its own (DeleteAttachment removes the file at once): not in
	// the batch, so its absent file is the expected steady state.
	s.addDeletedAttachment(t, "standalone-deleted")
	s.trashContact(t, batchPayload(batched))

	report, err := s.verifyWithKey(t, nil)
	require.NoError(t, err)

	assert.False(t, report.Complete(), report.String())
	require.Len(t, report.MissingPhotos, 1)
	assert.Equal(t, "trashed_photo.jpg", report.MissingPhotos[0].StoredName)
	assert.Equal(t, "uid-trashed", report.MissingPhotos[0].Owner)
	require.Len(t, report.MissingAttachments, 1)
	assert.Equal(t, "batched-attachment", report.MissingAttachments[0].StoredName)
	assert.Equal(t, 1, report.PhotosScanned)
	assert.Equal(t, 1, report.AttachmentsScanned)
}

func TestVerifyBackupSetTrashedContactFilesPresentAreNotOrphans(t *testing.T) {
	s := newBackupSet(t)
	batched := s.addDeletedAttachment(t, "batched-attachment")
	s.trashContact(t, batchPayload(batched))
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "trashed_photo.jpg"), []byte("jpeg"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(s.attachmentsDir, "batched-attachment"), []byte("pdf"), 0o600))

	report, err := s.verifyWithKey(t, nil)
	require.NoError(t, err)

	assert.True(t, report.Complete(), report.String())
	assert.Empty(t, report.OrphanPhotoFiles)
	assert.Empty(t, report.OrphanAttachmentFiles)
}

// encryptedTrash writes the snapshot payload encrypted under a fresh master
// key, exactly as the server's serializer:encrypted column stores it, and
// returns that key. The engine is disarmed again before returning, so the
// verifier cannot lean on process-wide state.
func encryptedTrash(t *testing.T, s *backupSet, payload string) []byte {
	t.Helper()
	kek := make([]byte, 32)
	_, err := rand.Read(kek)
	require.NoError(t, err)
	require.NoError(t, atrest.Initialize(s.db, kek))
	defer atrest.ResetForTest()
	enc, err := atrest.Encrypt(payload)
	require.NoError(t, err)
	require.NotEqual(t, payload, enc, "the payload must be stored as ciphertext")
	s.trashContact(t, enc)
	return kek
}

func TestVerifyBackupSetDecryptsTheTrashPayloadWithTheMasterKey(t *testing.T) {
	s := newBackupSet(t)
	batched := s.addDeletedAttachment(t, "batched-attachment")
	s.addDeletedAttachment(t, "standalone-deleted")
	kek := encryptedTrash(t, s, batchPayload(batched))

	report, err := s.verifyWithKey(t, kek)
	require.NoError(t, err)

	require.Len(t, report.MissingAttachments, 1)
	assert.Equal(t, "batched-attachment", report.MissingAttachments[0].StoredName)
	require.Len(t, report.MissingPhotos, 1)
}

func TestVerifyBackupSetEncryptedTrashFailsClosedWithoutTheKey(t *testing.T) {
	s := newBackupSet(t)
	batched := s.addDeletedAttachment(t, "batched-attachment")
	encryptedTrash(t, s, batchPayload(batched))

	_, err := s.verifyWithKey(t, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATA_ENCRYPTION_KEY")
}

func TestVerifyBackupSetEncryptedTrashFailsClosedOnTheWrongKey(t *testing.T) {
	s := newBackupSet(t)
	batched := s.addDeletedAttachment(t, "batched-attachment")
	encryptedTrash(t, s, batchPayload(batched))

	wrong := make([]byte, 32)
	_, err := s.verifyWithKey(t, wrong)
	require.Error(t, err)
}

func TestVerifyBackupSetUnreadableTrashPayloadFailsClosed(t *testing.T) {
	s := newBackupSet(t)
	s.trashContact(t, `{not json`)

	_, err := s.verifyWithKey(t, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uid-trashed")
}
