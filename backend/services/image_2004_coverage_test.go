package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Edge and failure paths of the #2004 completion work: Restore, the sweeper's
// scheduled entry point and scan errors, bundle image export/import corners,
// the session wiring, and the restore drill's image reconciliation.

func fullImportConfig(images string) config.Config {
	return config.Config{ImagesDir: images, ProfilePhotoDir: filepath.Join(images, "..", "photos"), ImageMaxPixels: 50_000_000, ImageMaxUploadMB: 20, ImageMaxPerNote: 10, ImageMaxPerActivity: 10, ImageMaxPerContact: 50}
}

func TestImageService_RestoreAppliesMetadataAndKeepsFreeUUID(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	taken := time.Date(2020, 5, 6, 7, 8, 9, 0, time.UTC)
	day := "2020-05-06"
	created := time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC)

	row, err := svc.Restore(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)), ImageRestoreMeta{
		UUID: "free-uuid", Sensitivity: "secret", TakenAt: &taken, TakenDate: &day,
		Crop: &models.ImageCrop{X: 0.1, Y: 0.2, W: 0.3, H: 0.4}, CreatedAt: created,
	})
	require.NoError(t, err)

	var got models.ContactImage
	require.NoError(t, svc.DB.First(&got, row.ID).Error)
	assert.Equal(t, "free-uuid", got.UUID)
	assert.Equal(t, "secret", got.Sensitivity)
	require.NotNil(t, got.TakenDate)
	assert.Equal(t, day, *got.TakenDate)
	require.NotNil(t, got.StoredCrop())
	assert.InDelta(t, 0.4, got.StoredCrop().H, 1e-9)
	assert.True(t, got.CreatedAt.Equal(created))

	// A uuid this table already holds is not reused (the column is unique).
	second, err := svc.Restore(context.Background(), userID, owner, bytes.NewReader(imageTestJPEGVariant(t, 4)), ImageRestoreMeta{UUID: "free-uuid", Sensitivity: "bogus"})
	require.NoError(t, err)
	assert.NotEqual(t, "free-uuid", second.UUID)
	assert.Equal(t, models.ImageSensitivityNormal, second.Sensitivity, "an unknown sensitivity never overrides the default")

	// A corrupt body surfaces the pipeline's error.
	_, err = svc.Restore(context.Background(), userID, owner, bytes.NewReader([]byte("nope")), ImageRestoreMeta{})
	require.Error(t, err)
}

func TestSweepImagesScheduled_RecordsAndSkips(t *testing.T) {
	useFakeClock(t, svcT0)
	db := dbtest.New(t)
	user := models.User{Username: "sweep-sched", Password: "password123!A", Email: "sweep-sched@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := t.TempDir()
	cfg := config.Config{ImagesDir: dir}
	writeSweepFile(t, dir, user.ID, "orphan.jpg", 10, 3*time.Hour)

	n, err := SweepImagesScheduled(db, cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	var events int64
	require.NoError(t, db.Model(&models.SystemEvent{}).Where("operation = ?", models.JobNameImageSweep).Count(&events).Error)
	assert.EqualValues(t, 1, events)

	// Immediately again: inside the de-dup window, the run is skipped.
	_, err = SweepImagesScheduled(db, cfg)
	assert.ErrorIs(t, err, ErrJobSkipped)
}

func TestSweepImages_IgnoresOddEntriesAndReportsUnreadableRoot(t *testing.T) {
	useFakeClock(t, svcT0)
	db := dbtest.New(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "not-a-user"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ubad"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "u1"), []byte("file, not a dir"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "u7", "subdir"), 0o700))
	res, err := SweepImages(context.Background(), db, config.Config{ImagesDir: dir})
	require.NoError(t, err)
	assert.Zero(t, res.FilesRemoved)

	// A root that is a file, not a directory, is a scan failure.
	file := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	_, err = SweepImages(context.Background(), db, config.Config{ImagesDir: file})
	require.Error(t, err)
}

func TestBuildAccountBundle_ImageExportCorners(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, noteImg, actImg := f.uploadAll(t)

	// An unreadable file and an oversized image are counted, never dropped silently.
	require.NoError(t, os.Remove(filepath.Join(f.dir, "u"+itoaU(f.user.ID), noteImg.StoredName)))
	require.NoError(t, f.db.Model(actImg).Update("size_bytes", maxBundleImageBytes+1).Error)

	bundle, stats, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store()})
	require.NoError(t, err)
	require.Len(t, bundle.Images, 1)
	assert.Equal(t, contactImg.UUID, bundle.Images[0].UUID)
	assert.Equal(t, 2, bundle.Omitted.ImagesOmitted)
	assert.Equal(t, 2, stats.ImagesOmitted)

	// No images directory configured: no images and nothing counted.
	none, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{})
	require.NoError(t, err)
	assert.Empty(t, none.Images)
	assert.NotNil(t, none.Images, "an empty images list marshals as [], never null")
	raw, err := json.Marshal(none)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"images":[]`)
}

func TestImportBundleImages_RejectsAndSkipsCorners(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	f.uploadAll(t)
	bundle, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)

	dest := models.User{Username: "corner-dest", Password: "password123!A", Email: "corner-dest@example.com"}
	require.NoError(t, f.db.Create(&dest).Error)
	contact := models.Contact{UserID: dest.ID, Firstname: "D", VCardUID: "44444444-4444-4444-8444-444444444444", Photo: "has-photo.jpg"}
	require.NoError(t, f.db.Create(&contact).Error)
	note := models.Note{UUID: f.note.UUID, UserID: dest.ID, Content: "", Date: svcT0, ContactID: &contact.ID}
	require.NoError(t, f.db.Create(&note).Error)
	act := models.Activity{UUID: f.act.UUID, UserID: dest.ID, Title: "T", Date: svcT0}
	require.NoError(t, f.db.Create(&act).Error)
	cfg := fullImportConfig(t.TempDir())
	opts := BundleImageImport{Config: cfg}
	refToID := map[string]uint{"contact/" + f.contact.VCardUID: contact.ID}

	// Nothing to do without a directory or images.
	assert.Zero(t, ImportBundleImages(context.Background(), f.db, BundleImageImport{}, dest.ID, bundle.Images, nil, refToID).Created)
	assert.Zero(t, ImportBundleImages(context.Background(), f.db, opts, dest.ID, nil, nil, refToID).Created)

	bad := []models.AccountBundleImage{
		{UUID: "b1", OwnerKind: "note", Owner: f.note.UUID, Data: "!!!not base64!!!"},
		{UUID: "b2", OwnerKind: "bogus", Owner: "x", Data: base64.StdEncoding.EncodeToString([]byte("x"))},
		{UUID: "b3", OwnerKind: "activity", Owner: f.act.UUID, Data: base64.StdEncoding.EncodeToString([]byte("not an image"))},
		{UUID: "b4", OwnerKind: "contact", Owner: f.contact.VCardUID, Data: base64.StdEncoding.EncodeToString([]byte("not an image"))},
		{UUID: "b5", OwnerKind: "note", Owner: "no-such-note", Data: base64.StdEncoding.EncodeToString([]byte("x"))},
		{UUID: "b6", OwnerKind: "activity", Owner: "no-such-activity", Data: base64.StdEncoding.EncodeToString([]byte("x"))},
		{UUID: "b7", OwnerKind: "contact", Owner: "unmapped-contact", Data: base64.StdEncoding.EncodeToString([]byte("x"))},
	}
	res := ImportBundleImages(context.Background(), f.db, opts, dest.ID, bad, nil, refToID)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 4, res.Rejected, "bad base64, an unknown kind and two corrupt images are rejected")
	assert.Equal(t, 3, res.Skipped, "unresolved owners are skipped")

	// A cancelled context stops the pass.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Zero(t, ImportBundleImages(ctx, f.db, opts, dest.ID, bundle.Images, nil, refToID).Created)

	// With a photo of its own the main image sets only the pointer.
	var main models.AccountBundleImage
	for _, im := range bundle.Images {
		if im.OwnerKind == "contact" {
			main = im
		}
	}
	res = ImportBundleImages(context.Background(), f.db, opts, dest.ID, []models.AccountBundleImage{main}, map[string]string{f.contact.VCardUID: main.UUID}, refToID)
	assert.Equal(t, 1, res.Created)
	var got models.Contact
	require.NoError(t, f.db.First(&got, contact.ID).Error)
	require.NotNil(t, got.MainImageID)
	assert.Equal(t, "has-photo.jpg", got.Photo, "the card's own photo is never replaced")

	// A per-owner cap rejects the overflow.
	capped := fullImportConfig(t.TempDir())
	capped.ImageMaxPerNote = 1
	two := []models.AccountBundleImage{}
	for i := 0; i < 2; i++ {
		raw := imageTestJPEGVariant(t, 5+i)
		two = append(two, models.AccountBundleImage{UUID: "cap" + string(rune('a'+i)), OwnerKind: "note", Owner: f.note.UUID, Data: base64.StdEncoding.EncodeToString(raw)})
	}
	res = ImportBundleImages(context.Background(), f.db, BundleImageImport{Config: capped}, dest.ID, two, nil, refToID)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Rejected)
}

func TestMycorrhizalImport_ImagesLandThroughTheSession(t *testing.T) {
	useFakeClock(t, svcT0)
	f := newBundleImageFixture(t)
	contactImg, _, _ := f.uploadAll(t)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.contact.ID).Update("main_image_id", contactImg.ID).Error)
	exported, _, err := BuildAccountBundleWithImages(f.db, f.user.ID, t.TempDir(), BundleImageExport{Store: f.store(), MaxBundleBytes: MaxMycorrhizalBundleSize})
	require.NoError(t, err)
	data, err := json.Marshal(exported)
	require.NoError(t, err)

	targetDB := dbtest.New(t)
	target := bundleTestUser(t, targetDB, "session-images")
	mgr := NewMycorrhizalImportManager()
	log := zerolog.Nop()
	up, appErr := mgr.Upload(target.ID, bundleMultipartHeader(t, data))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(targetDB, target.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
	waitForMycorrhizalPhase(t, mgr, target.ID, up.SessionID, models.SourceImportPhaseReady)
	preview, appErr := mgr.Preview(target.ID, up.SessionID)
	require.Nil(t, appErr)
	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	cfg := fullImportConfig(t.TempDir())
	require.Nil(t, mgr.Confirm(targetDB, target.ID, models.SourceImportConfirmRequest{SessionID: up.SessionID, Actions: actions}, cfg, &log))
	status := waitForMycorrhizalPhase(t, mgr, target.ID, up.SessionID, models.SourceImportPhaseDone)

	require.NotNil(t, status.Result)
	// The contact and note images land; the activity has no contact in the plan,
	// so the entity import never creates it and its image is skipped.
	assert.Equal(t, 2, status.Result.ImagesCreated)
	assert.Zero(t, status.Result.ImagesRejected)
	var landed models.Contact
	require.NoError(t, targetDB.Where("user_id = ?", target.ID).First(&landed).Error)
	assert.NotNil(t, landed.MainImageID, "the main image is re-applied after the images land")
}

func TestCheckBackupSetCompleteness_ReconcilesImages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "snap.db")
	db := dbtest.NewAt(t, dbPath)
	user := models.User{Username: "drill-img", Password: "password123!A", Email: "drill-img@example.com"}
	require.NoError(t, db.Create(&user).Error)
	note := models.Note{UserID: user.ID, Content: "n", Date: svcT0}
	require.NoError(t, db.Create(&note).Error)
	require.NoError(t, db.Create(&models.NoteImage{NoteID: note.ID, ImageFields: models.ImageFields{UserID: user.ID, UUID: "d1", StoredName: "d1.jpg", ThumbName: "d1_t.jpg", MIMEType: "image/jpeg", ThumbMIMEType: "image/jpeg", Width: 1, Height: 1, SizeBytes: 1, ThumbSizeBytes: 1, Sensitivity: models.ImageSensitivityNormal}}).Error)

	base := t.TempDir()
	cfg := config.Config{ProfilePhotoDir: filepath.Join(base, "p"), AttachmentsDir: filepath.Join(base, "a"), ImagesDir: filepath.Join(base, "i")}
	require.NoError(t, os.MkdirAll(cfg.ProfilePhotoDir, 0o750))
	require.NoError(t, os.MkdirAll(cfg.AttachmentsDir, 0o750))

	// IMAGES_DIR is created lazily by the first upload: absent means not checked.
	ok, _, err := checkBackupSetCompleteness(dbPath, cfg)
	require.NoError(t, err)
	assert.True(t, ok)

	// Present but missing the row's files: the drill fails and names them.
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.ImagesDir, "u"+itoaU(user.ID)), 0o750))
	ok, detail, err := checkBackupSetCompleteness(dbPath, cfg)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, detail, "image")
}
