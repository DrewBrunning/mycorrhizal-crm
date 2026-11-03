package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSweepImages_FailurePaths(t *testing.T) {
	useFakeClock(t, svcT0)

	t.Run("empty-note query failure is a scan error", func(t *testing.T) {
		db := dbtest.New(t)
		dbtest.HideTable(t, db, "notes")
		_, err := SweepImages(context.Background(), db, config.Config{})
		require.Error(t, err)
	})

	t.Run("image table read failure is a scan error", func(t *testing.T) {
		db := dbtest.New(t)
		dir := t.TempDir()
		writeSweepFile(t, dir, 1, "x.jpg", 1, 2*time.Hour)
		dbtest.HideTable(t, db, "note_images")
		_, err := SweepImages(context.Background(), db, config.Config{ImagesDir: dir})
		require.Error(t, err)
	})

	t.Run("an unreadable user directory is a scan error and an unremovable file is skipped", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permission bits do not bind root")
		}
		db := dbtest.New(t)
		dir := t.TempDir()
		stuck := writeSweepFile(t, dir, 1, "stuck.jpg", 1, 2*time.Hour)
		userDir := filepath.Dir(stuck)

		require.NoError(t, os.Chmod(userDir, 0o500)) // listable, not writable: Remove fails
		res, err := SweepImages(context.Background(), db, config.Config{ImagesDir: dir})
		require.NoError(t, err)
		assert.Zero(t, res.FilesRemoved)
		assert.FileExists(t, stuck)

		require.NoError(t, os.Chmod(userDir, 0o000)) // not even listable
		_, err = SweepImages(context.Background(), db, config.Config{ImagesDir: dir})
		require.Error(t, err)
		require.NoError(t, os.Chmod(userDir, 0o700))
	})

	t.Run("a failed scan releases the job lock and surfaces", func(t *testing.T) {
		db := dbtest.New(t)
		file := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		_, err := SweepImagesScheduled(db, config.Config{ImagesDir: file})
		require.Error(t, err)
	})
}

func TestBuildAccountBundle_ImageReadFailureFailsTheExport(t *testing.T) {
	useFakeClock(t, svcT0)
	for _, table := range []string{"contact_images", "note_images", "activity_images"} {
		t.Run(table, func(t *testing.T) {
			f := newBundleImageFixture(t)
			dbtest.HideTable(t, f.db, table)
			_, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store()})
			require.Error(t, err)
		})
	}
}

func TestImportBundleImages_MainImageEdges(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, _, _ := f.uploadAll(t)
	require.NoError(t, f.db.Model(contactImg).Update("sensitivity", "private").Error)
	bundle, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)
	var contactEntry models.AccountBundleImage
	for _, im := range bundle.Images {
		if im.OwnerKind == "contact" {
			contactEntry = im
		}
	}

	dest := models.User{Username: "main-edges", Password: "password123!A", Email: "main-edges@example.com"}
	require.NoError(t, f.db.Create(&dest).Error)
	contact := models.Contact{UserID: dest.ID, Firstname: "D", VCardUID: "55555555-5555-4555-8555-555555555555"}
	require.NoError(t, f.db.Create(&contact).Error)
	opts := BundleImageImport{Config: fullImportConfig(t.TempDir())}
	refToID := map[string]uint{"contact/" + f.contact.VCardUID: contact.ID}
	only := []models.AccountBundleImage{contactEntry}

	// Unknown contact uid, and an image uuid that never landed: nothing applied.
	ImportBundleImages(context.Background(), f.db, opts, dest.ID, only, map[string]string{"unknown-uid": contactEntry.UUID}, refToID)
	ImportBundleImages(context.Background(), f.db, opts, dest.ID, nil, map[string]string{f.contact.VCardUID: "never-landed"}, refToID)
	var got models.Contact
	require.NoError(t, f.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)

	// A private image can never become the avatar source: the derive step
	// refuses, the import still succeeds and leaves no main image.
	// (The image already landed on the first call, so this pass skips it and
	// still resolves it for the main-image step.)
	res := ImportBundleImages(context.Background(), f.db, opts, dest.ID, only, map[string]string{f.contact.VCardUID: contactEntry.UUID}, refToID)
	assert.Equal(t, 1, res.Skipped)
	require.NoError(t, f.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)
	assert.Empty(t, got.Photo)
}
