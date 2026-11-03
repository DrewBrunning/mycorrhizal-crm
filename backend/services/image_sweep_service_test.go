package services

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ADR 0040 decision 5: the sweeper removes files no row names (read
// Unscoped) once past the grace period, stale *.tmp files, and soft-deletes
// empty image-less notes past the one-hour window (decision 11).

func writeSweepFile(t *testing.T, dir string, user uint, name string, size int, age time.Duration) string {
	t.Helper()
	userDir := filepath.Join(dir, "u"+strconv.FormatUint(uint64(user), 10))
	require.NoError(t, os.MkdirAll(userDir, 0o700))
	p := filepath.Join(userDir, name)
	require.NoError(t, os.WriteFile(p, make([]byte, size), 0o600))
	at := Now().Add(-age)
	require.NoError(t, os.Chtimes(p, at, at))
	return p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestSweepImages_RemovesOnlyUnreferencedPastGrace(t *testing.T) {
	useFakeClock(t, svcT0)
	db := dbtest.New(t)
	user := models.User{Username: "sweep", Password: "password123!A", Email: "sweep@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := t.TempDir()
	cfg := config.Config{ImagesDir: dir}

	note := models.Note{UserID: user.ID, Content: "n", Date: svcT0}
	require.NoError(t, db.Create(&note).Error)
	live := models.NoteImage{NoteID: note.ID, ImageFields: models.ImageFields{UserID: user.ID, UUID: "live", StoredName: "live.jpg", ThumbName: "live_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}
	require.NoError(t, db.Create(&live).Error)
	soft := models.NoteImage{NoteID: note.ID, ImageFields: models.ImageFields{UserID: user.ID, UUID: "soft", StoredName: "soft.jpg", ThumbName: "soft_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}
	require.NoError(t, db.Create(&soft).Error)
	require.NoError(t, db.Delete(&soft).Error) // soft-deleted rows still own their files until purge

	old := 2 * time.Hour
	liveFull := writeSweepFile(t, dir, user.ID, "live.jpg", 10, old)
	liveThumb := writeSweepFile(t, dir, user.ID, "live_t.jpg", 10, old)
	softFull := writeSweepFile(t, dir, user.ID, "soft.jpg", 10, old)
	orphan := writeSweepFile(t, dir, user.ID, "orphan.jpg", 100, old)
	fresh := writeSweepFile(t, dir, user.ID, "fresh-orphan.jpg", 100, 10*time.Minute)
	staleTmp := writeSweepFile(t, dir, user.ID, "x.jpg.tmp", 50, old)
	freshTmp := writeSweepFile(t, dir, user.ID, "y.jpg.tmp", 50, 5*time.Minute)

	res, err := SweepImages(context.Background(), db, cfg)
	require.NoError(t, err)

	assert.True(t, exists(liveFull) && exists(liveThumb), "files named by a live row survive")
	assert.True(t, exists(softFull), "files named by a soft-deleted row survive until purge")
	assert.False(t, exists(orphan), "an unreferenced file past grace is removed")
	assert.True(t, exists(fresh), "an unreferenced file inside the grace period is never raced")
	assert.False(t, exists(staleTmp), "a stale tmp file is removed")
	assert.True(t, exists(freshTmp), "an in-flight tmp file survives")
	assert.EqualValues(t, 2, res.FilesRemoved)
	assert.EqualValues(t, 150, res.BytesRemoved)
}

func TestSweepImages_SoftDeletesEmptyImagelessNotesPastWindow(t *testing.T) {
	useFakeClock(t, svcT0)
	db := dbtest.New(t)
	user := models.User{Username: "sweep2", Password: "password123!A", Email: "sweep2@example.com"}
	require.NoError(t, db.Create(&user).Error)
	cfg := config.Config{ImagesDir: t.TempDir()}

	mk := func(content string, age time.Duration) models.Note {
		n := models.Note{UserID: user.ID, Content: content, Date: svcT0}
		require.NoError(t, db.Create(&n).Error)
		at := Now().Add(-age)
		require.NoError(t, db.Model(&models.Note{}).Where("id = ?", n.ID).UpdateColumn("created_at", at).Error)
		return n
	}
	stale := mk("", 2*time.Hour)
	inWindow := mk("", 10*time.Minute)
	withText := mk("text", 2*time.Hour)
	withImage := mk("", 2*time.Hour)
	require.NoError(t, db.Create(&models.NoteImage{NoteID: withImage.ID, ImageFields: models.ImageFields{UserID: user.ID, UUID: "wi", StoredName: "wi.jpg", ThumbName: "wi_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}).Error)

	res, err := SweepImages(context.Background(), db, cfg)
	require.NoError(t, err)

	alive := func(id uint) bool {
		var c int64
		require.NoError(t, db.Model(&models.Note{}).Where("id = ?", id).Count(&c).Error)
		return c == 1
	}
	assert.False(t, alive(stale.ID), "empty image-less note past the window is soft-deleted")
	assert.True(t, alive(inWindow.ID))
	assert.True(t, alive(withText.ID))
	assert.True(t, alive(withImage.ID))
	assert.EqualValues(t, 1, res.NotesRemoved)
}

func TestSweepImages_MissingDirIsNotAnError(t *testing.T) {
	useFakeClock(t, svcT0)
	db := dbtest.New(t)
	res, err := SweepImages(context.Background(), db, config.Config{ImagesDir: filepath.Join(t.TempDir(), "absent")})
	require.NoError(t, err)
	assert.Zero(t, res.FilesRemoved)
}
