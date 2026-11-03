package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// backupSet builds a real migrated database plus a photo directory and an
// attachments directory, all mutually consistent: every live attachment and
// profile-photo row has its file on disk, and no stray files exist. Individual
// tests then knock one piece out and assert VerifyBackupSet notices.
type backupSet struct {
	dbPath         string
	photoDir       string
	attachmentsDir string
	db             *gorm.DB
}

func newBackupSet(t *testing.T) *backupSet {
	t.Helper()
	dir := t.TempDir()
	s := &backupSet{
		dbPath:         filepath.Join(dir, "backup.db"),
		photoDir:       filepath.Join(dir, "photos"),
		attachmentsDir: filepath.Join(dir, "attachments"),
	}
	require.NoError(t, os.MkdirAll(s.photoDir, 0o750))
	require.NoError(t, os.MkdirAll(s.attachmentsDir, 0o750))

	db, err := database.InitDB(s.dbPath)
	require.NoError(t, err, "migrations must apply")
	s.db = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Exec(
		`INSERT INTO users (id, username, email, password) VALUES (1, 'u', 'u@example.com', 'x')`).Error)

	return s
}

// addContactWithPhoto inserts a live contact row whose photo column names
// photoFile, and writes that file into the photo directory.
func (s *backupSet) addContactWithPhoto(t *testing.T, id int, vcardUID, photoFile string) {
	t.Helper()
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo) VALUES (?, ?, 1, ?, ?)`,
		id, "Contact", vcardUID, photoFile).Error)
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, photoFile), []byte("jpegbytes"), 0o600))
}

// addAttachment inserts a live attachment row for storedName and writes that
// file into the attachments directory. Returns the row id.
func (s *backupSet) addAttachment(t *testing.T, storedName string) int64 {
	t.Helper()
	res := s.db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes)
		 VALUES (1, 'c-uid', ?, 'doc.pdf', 'application/pdf', 9)`, storedName)
	require.NoError(t, res.Error)
	var id int64
	require.NoError(t, s.db.Raw(`SELECT id FROM attachments WHERE stored_name = ?`, storedName).Scan(&id).Error)
	require.NoError(t, os.WriteFile(filepath.Join(s.attachmentsDir, storedName), []byte("filebytes"), 0o600))
	return id
}

func (s *backupSet) verify(t *testing.T) *database.BackupSetReport {
	t.Helper()
	report, err := database.VerifyBackupSet(s.dbPath, s.photoDir, s.attachmentsDir)
	require.NoError(t, err)
	return report
}

func TestVerifyBackupSetCompleteSet(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	s.addContactWithPhoto(t, 1, "uid-1", "aaa_photo.jpg")
	s.addContactWithPhoto(t, 2, "uid-2", "bbb_photo.jpg")
	s.addAttachment(t, "11111111-1111-1111-1111-111111111111")
	s.addAttachment(t, "22222222-2222-2222-2222-222222222222")

	report := s.verify(t)

	assert.True(t, report.Complete(), report.String())
	assert.Equal(t, 2, report.AttachmentsScanned)
	assert.Equal(t, 2, report.PhotosScanned)
	assert.Empty(t, report.MissingAttachments)
	assert.Empty(t, report.MissingPhotos)
	assert.Empty(t, report.OrphanAttachmentFiles)
	assert.Empty(t, report.OrphanPhotoFiles)
	assert.Contains(t, report.String(), "backup set is complete")
}

func TestVerifyBackupSetMissingAttachmentFileIsNamed(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	keptID := s.addAttachment(t, "kept-file")
	goneID := s.addAttachment(t, "gone-file")
	require.NoError(t, os.Remove(filepath.Join(s.attachmentsDir, "gone-file")))

	report := s.verify(t)

	assert.False(t, report.Complete())
	require.Len(t, report.MissingAttachments, 1)
	assert.Equal(t, "gone-file", report.MissingAttachments[0].StoredName)
	assert.Equal(t, "attachment", report.MissingAttachments[0].Kind)
	assert.Contains(t, report.MissingAttachments[0].Owner, "attachment#")
	assert.Contains(t, report.String(), "gone-file")
	assert.Equal(t, 2, report.AttachmentsScanned)
	_ = keptID
	_ = goneID
}

func TestVerifyBackupSetMissingPhotoFileIsNamedByContactUID(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	s.addContactWithPhoto(t, 1, "uid-present", "here_photo.jpg")
	s.addContactWithPhoto(t, 2, "uid-lost", "lost_photo.jpg")
	require.NoError(t, os.Remove(filepath.Join(s.photoDir, "lost_photo.jpg")))

	report := s.verify(t)

	assert.False(t, report.Complete())
	require.Len(t, report.MissingPhotos, 1)
	assert.Equal(t, "lost_photo.jpg", report.MissingPhotos[0].StoredName)
	assert.Equal(t, "uid-lost", report.MissingPhotos[0].Owner)
	assert.Contains(t, report.String(), "backup set is INCOMPLETE")
}

func TestVerifyBackupSetContactWithoutVCardUIDFallsBackToID(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, photo) VALUES (7, 'NoUID', 1, 'x_photo.jpg')`).Error)
	// No file written — the row is missing its photo.

	report := s.verify(t)

	require.Len(t, report.MissingPhotos, 1)
	assert.Equal(t, "contact#7", report.MissingPhotos[0].Owner)
}

func TestVerifyBackupSetOrphanFilesAreReportedButNotAFailure(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	s.addContactWithPhoto(t, 1, "uid-1", "owned_photo.jpg")
	s.addAttachment(t, "owned-attachment")
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "stray_photo.jpg"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(s.attachmentsDir, "stray-attachment"), []byte("x"), 0o600))

	report := s.verify(t)

	assert.True(t, report.Complete(), "orphans alone must not make the set incomplete")
	assert.Equal(t, []string{"stray_photo.jpg"}, report.OrphanPhotoFiles)
	assert.Equal(t, []string{"stray-attachment"}, report.OrphanAttachmentFiles)
	assert.Equal(t, 2, report.TotalOrphans())
	assert.Contains(t, report.String(), "orphan")
}

func TestVerifyBackupSetIgnoresSoftDeletedRows(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	// A soft-deleted attachment: its file is removed at delete time, so the
	// absent file is the expected steady state, not a hole.
	require.NoError(t, s.db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes, deleted_at)
		 VALUES (1, 'c', 'deleted-attachment', 'd.pdf', 'application/pdf', 1, CURRENT_TIMESTAMP)`).Error)
	// A soft-deleted contact still carries its photo column but its file is gone.
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo, deleted_at)
		 VALUES (9, 'Gone', 1, 'uid-gone', 'gone_photo.jpg', CURRENT_TIMESTAMP)`).Error)

	report := s.verify(t)

	assert.True(t, report.Complete(), report.String())
	assert.Equal(t, 0, report.AttachmentsScanned)
	assert.Equal(t, 0, report.PhotosScanned)
	assert.Empty(t, report.OrphanAttachmentFiles)
	assert.Empty(t, report.OrphanPhotoFiles)
}

func TestVerifyBackupSetInlineThumbnailIsNotADiskReference(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	// Full photo present on disk; thumbnail is an inline data URL (the norm).
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo, photo_thumbnail)
		 VALUES (1, 'C', 1, 'uid-1', 'p_photo.jpg', 'data:image/jpeg;base64,AAAA')`).Error)
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "p_photo.jpg"), []byte("x"), 0o600))

	report := s.verify(t)

	assert.True(t, report.Complete(), report.String())
	assert.Empty(t, report.MissingPhotos)
}

func TestVerifyBackupSetFilenameThumbnailIsCheckedAndReferenced(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo, photo_thumbnail)
		 VALUES (1, 'C', 1, 'uid-1', 'full_photo.jpg', 'thumb_photo.jpg')`).Error)
	require.NoError(t, os.WriteFile(filepath.Join(s.photoDir, "full_photo.jpg"), []byte("x"), 0o600))
	// thumb_photo.jpg intentionally absent.

	report := s.verify(t)

	assert.False(t, report.Complete())
	require.Len(t, report.MissingPhotos, 1)
	assert.Equal(t, "thumb_photo.jpg", report.MissingPhotos[0].StoredName)
	// The full photo is present, so it must not also appear as an orphan.
	assert.NotContains(t, report.OrphanPhotoFiles, "full_photo.jpg")
	// One distinct contact row was scanned even though it names two files.
	assert.Equal(t, 1, report.PhotosScanned)
}

func TestVerifyBackupSetEmptyIsClean(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)

	report := s.verify(t)

	assert.True(t, report.Complete())
	assert.Equal(t, 0, report.AttachmentsScanned)
	assert.Equal(t, 0, report.PhotosScanned)
	assert.Equal(t, 0, report.TotalOrphans())
	assert.Contains(t, report.String(), "backup set is complete")
}

// TestVerifyBackupSetQueryErrorsSurfaceWhichTable pins that a database
// missing a table VerifyBackupSet expects (a genuinely corrupt or wrong-schema
// backup, not merely an empty one) fails loudly and names which query broke,
// rather than being silently treated as "zero rows".
func TestVerifyBackupSetQueryErrorsSurfaceWhichTable(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(`DROP TABLE attachments`).Error)

	_, err := database.VerifyBackupSet(s.dbPath, s.photoDir, s.attachmentsDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query attachments")
}

func TestVerifyBackupSetQueryErrorOnContactsTable(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(`DROP TABLE contacts`).Error)

	_, err := database.VerifyBackupSet(s.dbPath, s.photoDir, s.attachmentsDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query contact photos")
}

func TestVerifyBackupSetIgnoresSubdirectories(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)
	// A subdirectory inside either backup directory (e.g. a stray editor swap
	// dir, or an operator's own nested organisation) must be neither a missing
	// reference nor reported as an orphan file.
	require.NoError(t, os.MkdirAll(filepath.Join(s.attachmentsDir, "nested"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(s.photoDir, "nested"), 0o750))

	report := s.verify(t)

	assert.True(t, report.Complete())
	assert.Empty(t, report.OrphanAttachmentFiles)
	assert.Empty(t, report.OrphanPhotoFiles)
}

func TestVerifyBackupSetUsageErrors(t *testing.T) {
	t.Parallel()
	s := newBackupSet(t)

	_, err := database.VerifyBackupSet(s.dbPath, "", s.attachmentsDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")

	_, err = database.VerifyBackupSet(s.dbPath, s.photoDir, "")
	require.Error(t, err)

	_, err = database.VerifyBackupSet(filepath.Join(t.TempDir(), "nope.db"), s.photoDir, s.attachmentsDir)
	require.Error(t, err)

	_, err = database.VerifyBackupSet(s.dbPath, filepath.Join(t.TempDir(), "no-such-dir"), s.attachmentsDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "photo directory")

	// A file where a directory is expected.
	notDir := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(notDir, []byte("x"), 0o600))
	_, err = database.VerifyBackupSet(s.dbPath, s.photoDir, notDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

// ADR 0040 decision 5: VerifyBackupSet learns IMAGES_DIR. Image files live in
// per-user subdirectories (u<id>/<name>), so a live image row of any of the
// three owner kinds whose full or thumbnail file is absent makes the set
// incomplete.

func (s *backupSet) addNoteImage(t *testing.T, imagesDir, uuid string, writeThumb bool) {
	t.Helper()
	require.NoError(t, s.db.Exec(`INSERT INTO notes (id, user_id, content, date) VALUES (50, 1, 'n', '2024-01-01')`).Error)
	img := models.NoteImage{NoteID: 50, ImageFields: models.ImageFields{UserID: 1, UUID: uuid, StoredName: uuid + ".jpg", ThumbName: uuid + "_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}
	require.NoError(t, s.db.Create(&img).Error)
	userDir := filepath.Join(imagesDir, "u1")
	require.NoError(t, os.MkdirAll(userDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, uuid+".jpg"), []byte("full"), 0o600))
	if writeThumb {
		require.NoError(t, os.WriteFile(filepath.Join(userDir, uuid+"_t.jpg"), []byte("thumb"), 0o600))
	}
}

func TestVerifyBackupSetWithImages_CompleteAndMissingAndOrphan(t *testing.T) {
	s := newBackupSet(t)
	imagesDir := filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.MkdirAll(imagesDir, 0o750))
	s.addNoteImage(t, imagesDir, "a", true)

	report, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir)
	require.NoError(t, err)
	assert.True(t, report.Complete(), report.String())
	assert.Equal(t, 1, report.ImagesScanned)

	// An orphan file is informational only.
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "u1", "stray.jpg"), []byte("x"), 0o600))
	report, err = database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir)
	require.NoError(t, err)
	assert.True(t, report.Complete())
	assert.Equal(t, []string{"u1/stray.jpg"}, report.OrphanImageFiles)

	// A missing thumbnail is a real hole in the set.
	require.NoError(t, os.Remove(filepath.Join(imagesDir, "u1", "a_t.jpg")))
	report, err = database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir)
	require.NoError(t, err)
	assert.False(t, report.Complete())
	require.Len(t, report.MissingImages, 1)
	assert.Equal(t, "u1/a_t.jpg", report.MissingImages[0].StoredName)
	assert.Contains(t, report.String(), "missing image: u1/a_t.jpg")
}

func TestVerifyBackupSetWithImages_SnapshotWithoutImageTablesIsClean(t *testing.T) {
	s := newBackupSet(t)
	require.NoError(t, s.db.Exec(`DROP TABLE contact_images`).Error)
	require.NoError(t, s.db.Exec(`DROP TABLE note_images`).Error)
	require.NoError(t, s.db.Exec(`DROP TABLE activity_images`).Error)
	imagesDir := t.TempDir()
	report, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir)
	require.NoError(t, err, "a pre-gallery snapshot has no image tables")
	assert.True(t, report.Complete())
	assert.Zero(t, report.ImagesScanned)
}

func TestVerifyBackupSet_IgnoresImagesWhenNoDirGiven(t *testing.T) {
	s := newBackupSet(t)
	s.addNoteImage(t, t.TempDir(), "a", false) // file set elsewhere; the plain verify never looks
	report := s.verify(t)
	assert.True(t, report.Complete())
	assert.Zero(t, report.ImagesScanned)
}

func TestVerifyBackupSetWithImages_RejectsMissingDirAndIgnoresStrayEntries(t *testing.T) {
	s := newBackupSet(t)
	_, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, filepath.Join(t.TempDir(), "absent"))
	require.Error(t, err, "an explicit images directory that does not exist is an error")

	imagesDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "top-level-file"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "u1", "nested"), 0o750))

	// A soft-deleted image row owns its files but a missing file is not a hole.
	require.NoError(t, s.db.Exec(`INSERT INTO notes (id, user_id, content, date) VALUES (50, 1, 'n', '2024-01-01')`).Error)
	img := models.NoteImage{NoteID: 50, ImageFields: models.ImageFields{UserID: 1, UUID: "gone", StoredName: "gone.jpg", ThumbName: "gone_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}
	require.NoError(t, s.db.Create(&img).Error)
	require.NoError(t, s.db.Delete(&img).Error)

	report, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, imagesDir)
	require.NoError(t, err)
	assert.True(t, report.Complete(), report.String())
	assert.Zero(t, report.ImagesScanned)
	assert.Empty(t, report.OrphanImageFiles)
}

func TestVerifyBackupSetWithImages_BrokenImageTableIsAnError(t *testing.T) {
	// A snapshot whose image table lacks the expected columns, or holds NULL
	// names, cannot be reconciled: that is an error, not a silent pass.
	for name, ddl := range map[string][]string{
		"missing columns": {`DROP TABLE note_images`, `CREATE TABLE note_images (id integer)`},
		"null names": {
			`DROP TABLE note_images`,
			`CREATE TABLE note_images (id integer, user_id integer, stored_name text, thumb_name text, deleted_at datetime)`,
			`INSERT INTO note_images (id, user_id, stored_name, thumb_name) VALUES (1, 1, NULL, NULL)`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newBackupSet(t)
			require.NoError(t, s.db.Exec(`PRAGMA foreign_keys = OFF`).Error)
			for _, stmt := range ddl {
				require.NoError(t, s.db.Exec(stmt).Error)
			}
			_, err := database.VerifyBackupSetWithImages(s.dbPath, s.photoDir, s.attachmentsDir, t.TempDir())
			require.Error(t, err)
		})
	}
}

func TestBackupSetReport_TotalMissingCountsImages(t *testing.T) {
	r := &database.BackupSetReport{MissingImages: []database.MissingRef{{Kind: "image"}}}
	assert.Equal(t, 1, r.TotalMissing())
	assert.False(t, r.Complete())
}
