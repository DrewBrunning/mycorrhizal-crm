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
	"mycorrhizal/internal/dbtest"

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

// Without a decrypter the payload is read as plaintext; a trashed contact
// with no attachments in its batch still owns its photo.
func TestVerifyBackupSetTrashedContactWithoutAttachments(t *testing.T) {
	s := newBackupSet(t)
	s.trashContact(t, `{"soft":{},"hard":{}}`)
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "trashed_photo.jpg"), []byte("jpeg"), 0o600))

	report := s.verify(t)

	assert.True(t, report.Complete(), report.String())
	assert.Equal(t, 1, report.PhotosScanned)
	assert.Equal(t, 0, report.AttachmentsScanned)
}

// A snapshot taken before the Trash existed has no snapshot table.
func TestVerifyBackupSetPreTrashSnapshotHasNothingToReconcile(t *testing.T) {
	s := newBackupSet(t)
	dbtest.HideTable(t, s.db, "contact_deletion_snapshots")

	report := s.verify(t)

	assert.True(t, report.Complete(), report.String())
}

// Issue #2367's image half: the contact cascade soft-deletes a trashed
// contact's gallery images and its notes' images with it (ADR 0040), and a
// Trash restore revives them, so the files their deletion batch names
// (payload.soft["contact_images"] / ["note_images"]) are required like a live
// image's. An image deleted on its own is not in the batch.

// addDeletedImage inserts a soft-deleted image row for user 1 into table
// (contact_images or note_images) and returns its id. ownerCol/ownerVal tie
// it to the trashed contact (contact_vcard_uid) or a note (note_id).
func (s *backupSet) addDeletedImage(t *testing.T, table, ownerCol string, ownerVal any, name string) int64 {
	t.Helper()
	// #nosec G202 -- table and column are fixed test literals.
	require.NoError(t, s.db.Exec(
		`INSERT INTO `+table+` (user_id, uuid, `+ownerCol+`, stored_name, thumb_name, mime_type, thumb_mime_type,
		 width, height, size_bytes, thumb_size_bytes, sensitivity, deleted_at)
		 VALUES (1, ?, ?, ?, ?, 'image/jpeg', 'image/jpeg', 1, 1, 1, 1, 'normal', CURRENT_TIMESTAMP)`,
		name, ownerVal, name+".jpg", name+"_t.jpg").Error)
	var id int64
	// #nosec G202 -- table is a fixed test literal.
	require.NoError(t, s.db.Raw(`SELECT id FROM `+table+` WHERE uuid = ?`, name).Scan(&id).Error)
	return id
}

func imageBatchPayload(contactImageID, noteImageID int64) string {
	return `{"soft":{"contact_images":["` + strconv.FormatInt(contactImageID, 10) +
		`"],"note_images":["` + strconv.FormatInt(noteImageID, 10) + `"]},"hard":{},"self_contact":false}`
}

func (s *backupSet) addDeletedNote(t *testing.T) {
	t.Helper()
	require.NoError(t, s.db.Exec(
		`INSERT INTO notes (id, user_id, content, date, deleted_at) VALUES (60, 1, 'n', '2024-01-01', CURRENT_TIMESTAMP)`).Error)
}

func (s *backupSet) verifyImagesWithKey(t *testing.T, imagesDir string) *database.BackupSetReport {
	t.Helper()
	report, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir, openerFor(nil))
	require.NoError(t, err)
	return report
}

func TestVerifyBackupSetRequiresTrashedContactImages(t *testing.T) {
	s := newBackupSet(t)
	s.addDeletedNote(t)
	gallery := s.addDeletedImage(t, "contact_images", "contact_vcard_uid", "uid-trashed", "gallery")
	notePhoto := s.addDeletedImage(t, "note_images", "note_id", 60, "notephoto")
	// Deleted on its own before the contact: not in the batch.
	s.addDeletedImage(t, "contact_images", "contact_vcard_uid", "uid-trashed", "standalone")
	s.trashContact(t, imageBatchPayload(gallery, notePhoto))
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "trashed_photo.jpg"), []byte("jpeg"), 0o600))
	imagesDir := t.TempDir()

	report := s.verifyImagesWithKey(t, imagesDir)

	assert.False(t, report.Complete(), report.String())
	var missing []string
	for _, m := range report.MissingImages {
		missing = append(missing, m.StoredName)
	}
	assert.ElementsMatch(t, []string{
		"u1/gallery.jpg", "u1/gallery_t.jpg", "u1/notephoto.jpg", "u1/notephoto_t.jpg",
	}, missing)
	assert.Equal(t, 2, report.ImagesScanned)
}

func TestVerifyBackupSetTrashedContactImagesPresentAreComplete(t *testing.T) {
	s := newBackupSet(t)
	s.addDeletedNote(t)
	gallery := s.addDeletedImage(t, "contact_images", "contact_vcard_uid", "uid-trashed", "gallery")
	notePhoto := s.addDeletedImage(t, "note_images", "note_id", 60, "notephoto")
	s.trashContact(t, imageBatchPayload(gallery, notePhoto))
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "trashed_photo.jpg"), []byte("jpeg"), 0o600))
	imagesDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "u1"), 0o750))
	for _, f := range []string{"gallery.jpg", "gallery_t.jpg", "notephoto.jpg", "notephoto_t.jpg"} {
		require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "u1", f), []byte("img"), 0o600))
	}

	report := s.verifyImagesWithKey(t, imagesDir)

	assert.True(t, report.Complete(), report.String())
	assert.Empty(t, report.OrphanImageFiles)
	assert.Equal(t, 2, report.ImagesScanned)
}

// A batch id is scoped to the snapshot's user: another user's image with
// that id is not the Trash's to require.
func TestVerifyBackupSetTrashedImageIDsAreUserScoped(t *testing.T) {
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(
		`INSERT INTO users (id, username, email, password) VALUES (2, 'v', 'v@example.com', 'x')`).Error)
	other := s.addDeletedImage(t, "contact_images", "contact_vcard_uid", "uid-other", "otheruser")
	require.NoError(t, s.db.Exec(`UPDATE contact_images SET user_id = 2 WHERE id = ?`, other).Error)
	s.trashContact(t, `{"soft":{"contact_images":["`+strconv.FormatInt(other, 10)+`"]},"hard":{}}`)
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "trashed_photo.jpg"), []byte("jpeg"), 0o600))

	report := s.verifyImagesWithKey(t, t.TempDir())

	assert.True(t, report.Complete(), report.String())
	assert.Equal(t, 0, report.ImagesScanned)
}
