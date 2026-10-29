package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbfault"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR 0037 decision 5: a deleted contact keeps its photo and attachment files
// until the purge commits, and only then are they removed. These tests pin the
// on-disk side of that contract, which the row-level purge tests don't reach.

func writePurgeFixture(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	return p
}

func TestPurgeSoftDeletedRows_RemovesPhotoThumbnailAndAttachmentFiles(t *testing.T) {
	db, userID := newPurgeDB(t)
	photoDir := t.TempDir()
	attachDir := t.TempDir()

	contact := models.Contact{UserID: userID, Firstname: "Files"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.First(&contact, contact.ID).Error)
	require.NoError(t, db.Model(&contact).Updates(map[string]any{
		"photo":           "photo.jpg",
		"photo_thumbnail": "thumb.jpg",
	}).Error)
	require.NoError(t, db.Create(&models.Attachment{
		UserID:          userID,
		ContactVCardUID: contact.VCardUID,
		StoredName:      "attach-uuid.bin",
		OriginalName:    "doc.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       1,
	}).Error)

	photo := writePurgeFixture(t, photoDir, "photo.jpg")
	thumb := writePurgeFixture(t, photoDir, "thumb.jpg")
	attach := writePurgeFixture(t, attachDir, "attach-uuid.bin")
	softDeleteAt(t, db, &models.Contact{}, contact.ID, time.Now().AddDate(0, 0, -60))

	cfg := config.Config{DeleteRetentionDays: 30, ProfilePhotoDir: photoDir, AttachmentsDir: attachDir}
	require.NoError(t, PurgeSoftDeletedRows(db, cfg))

	for _, p := range []string{photo, thumb, attach} {
		_, err := os.Stat(p)
		assert.True(t, os.IsNotExist(err), "%s removed after the purge committed", p)
	}
}

func TestPurgeSoftDeletedRows_ToleratesMissingAndUnsafeStoredNames(t *testing.T) {
	db, userID := newPurgeDB(t)
	attachDir := t.TempDir()
	outside := t.TempDir()
	survivor := writePurgeFixture(t, outside, "keep.txt")

	contact := models.Contact{UserID: userID, Firstname: "Ghost"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.First(&contact, contact.ID).Error)
	require.NoError(t, db.Model(&contact).Update("photo", "never-written.jpg").Error)
	// A traversal-shaped stored name must be refused, never resolved outside
	// the attachments directory.
	require.NoError(t, db.Create(&models.Attachment{
		UserID:          userID,
		ContactVCardUID: contact.VCardUID,
		StoredName:      "../keep.txt",
		OriginalName:    "x",
		ContentType:     "text/plain",
	}).Error)
	softDeleteAt(t, db, &models.Contact{}, contact.ID, time.Now().AddDate(0, 0, -60))

	cfg := config.Config{DeleteRetentionDays: 30, ProfilePhotoDir: t.TempDir(), AttachmentsDir: attachDir}
	require.NoError(t, PurgeSoftDeletedRows(db, cfg), "missing files and refused names are logged, not fatal")

	_, err := os.Stat(survivor)
	assert.NoError(t, err, "a traversal stored name must not remove a file outside the attachments dir")
}

func TestPurgeSoftDeletedRows_UnreadableAttachmentListStillPurges(t *testing.T) {
	db, userID := newPurgeDB(t)

	contact := models.Contact{UserID: userID, Firstname: "NoAttachTable"}
	require.NoError(t, db.Create(&contact).Error)
	softDeleteAt(t, db, &models.Contact{}, contact.ID, time.Now().AddDate(0, 0, -60))
	require.NoError(t, db.Exec("DROP TABLE attachments").Error)

	// The attachment row purge itself fails (no table), which is joined into
	// the result; the failed file lookup is only logged and the contact row
	// purge still runs.
	require.Error(t, PurgeSoftDeletedRows(db, purgeConfig()))

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", contact.ID).Count(&count).Error)
	assert.Zero(t, count, "a failed attachment lookup yields no files and the contact row purge still runs")
}

func TestRemovePurgedFiles_NoDirectoriesConfiguredIsNoOp(t *testing.T) {
	dir := t.TempDir()
	keep := writePurgeFixture(t, dir, "keep.jpg")

	RemovePurgedFiles(config.Config{}, PurgedFiles{Photos: []string{"keep.jpg"}, Attachments: []string{"keep.jpg"}})

	_, err := os.Stat(keep)
	assert.NoError(t, err, "with no photo or attachment directory configured nothing is touched")
}

func TestRemovePurgedFiles_NonEmptyDirectoryFailureDoesNotStopOthers(t *testing.T) {
	photoDir := t.TempDir()
	attachDir := t.TempDir()

	// os.Remove fails on a non-empty directory with a non-NotExist error; the
	// remaining files must still be removed.
	blocker := filepath.Join(photoDir, "blocker")
	require.NoError(t, os.Mkdir(blocker, 0o700))
	writePurgeFixture(t, blocker, "child.txt")
	attach := writePurgeFixture(t, attachDir, "a.bin")
	photo := writePurgeFixture(t, photoDir, "p.jpg")

	RemovePurgedFiles(config.Config{ProfilePhotoDir: photoDir, AttachmentsDir: attachDir}, PurgedFiles{
		Photos:      []string{"blocker", "p.jpg"},
		Attachments: []string{"a.bin"},
	})

	_, err := os.Stat(photo)
	assert.True(t, os.IsNotExist(err), "photo after a failed removal is still removed")
	_, err = os.Stat(attach)
	assert.True(t, os.IsNotExist(err), "attachment is removed")
	_, err = os.Stat(blocker)
	assert.NoError(t, err, "a non-empty directory is not removed, and the failure is only logged")
}

func TestPurgeSoftDeletedRows_FailedPhotoListingIsLoggedNotFatal(t *testing.T) {
	db := dbtest.New(t, dbtest.WithFaults())
	user := models.User{Username: "photo-list", Email: "photo-list@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Listing"}
	require.NoError(t, db.Create(&contact).Error)
	softDeleteAt(t, db, &models.Contact{}, contact.ID, time.Now().AddDate(0, 0, -60))

	// The first statement of the purge is the photo listing; fail only that one.
	dbfault.For(db).FailNth(1)
	require.NoError(t, PurgeSoftDeletedRows(db, purgeConfig()),
		"a failed photo listing is logged and the purge still runs")
	dbfault.For(db).Disarm()

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", contact.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRemovePurgedFiles_UnremovableAttachmentIsLoggedNotFatal(t *testing.T) {
	attachDir := t.TempDir()
	// A non-empty directory under the attachment name cannot be unlinked; the
	// failure is logged and the purge carries on.
	blocker := filepath.Join(attachDir, "blocked")
	require.NoError(t, os.Mkdir(blocker, 0o700))
	writePurgeFixture(t, blocker, "child.txt")

	RemovePurgedFiles(config.Config{AttachmentsDir: attachDir}, PurgedFiles{Attachments: []string{"blocked"}})

	_, err := os.Stat(blocker)
	assert.NoError(t, err, "the directory survives and the failure is only logged")
}
