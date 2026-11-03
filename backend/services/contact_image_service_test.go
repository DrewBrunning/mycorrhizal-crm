package services

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/internal/diskspace"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func uploadNoteImage(t *testing.T, db *gorm.DB, dir string, userID, noteID uint) *models.NoteImage {
	t.Helper()
	return uploadNoteImageData(t, db, dir, userID, noteID, imageTestJPEG(t))
}

func uploadNoteImageData(t *testing.T, db *gorm.DB, dir string, userID, noteID uint, data []byte) *models.NoteImage {
	t.Helper()
	svc := NoteImageService(db, imaging.Store{Dir: dir}, 10, 50_000_000, 20<<20, UserQuota{})
	img, err := svc.Upload(context.Background(), userID, OwnerRef{ID: noteID}, bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

// imageTestJPEGVariant returns a visually distinct picture per seed, so its
// dHash is far from imageTestJPEG's and from other seeds'.
func imageTestJPEGVariant(t *testing.T, seed int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			c := color.RGBA{R: uint8(x * 3), G: uint8(y * 4), B: 80, A: 255}
			cell := 4 + seed
			if (x/cell+y/cell)%2 == 0 {
				c = color.RGBA{R: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func TestSetAndClearContactMainImage(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	photoDir := t.TempDir()
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	img.CropX, img.CropY, img.CropW, img.CropH = ptr(0.0), ptr(0.0), ptr(0.5), ptr(0.5)

	got, err := SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, photoDir)
	require.NoError(t, err)
	require.NotNil(t, got.MainImageID)
	assert.Equal(t, img.ID, *got.MainImageID)
	assert.NotEmpty(t, got.Photo)
	assert.NotEmpty(t, got.PhotoThumbnail)

	avatar := filepath.Join(photoDir, got.Photo)
	require.FileExists(t, avatar)
	cleared, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, photoDir)
	require.NoError(t, err)
	assert.Nil(t, cleared.MainImageID)
	assert.Empty(t, cleared.Photo)
	assert.Empty(t, cleared.PhotoThumbnail)
	assert.NoFileExists(t, avatar, "clearing removes the derived avatar file")

	// Clearing again is a no-op.
	again, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, photoDir)
	require.NoError(t, err)
	assert.Nil(t, again.MainImageID)
}

func TestSetContactMainImage_MissingFileIsNotFound(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	img := &models.ContactImage{ImageFields: models.ImageFields{StoredName: "missing.jpg", Sensitivity: models.ImageSensitivityNormal}}
	_, err := SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, t.TempDir())
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func writePhotoFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	return path
}

func TestSetContactMainImage_ReplacesDerivedAvatarButKeepsLegacyPhoto(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	photoDir := t.TempDir()
	store := imaging.Store{Dir: dir}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	// Legacy photo with no main image exists nowhere else: it must survive.
	legacy := writePhotoFile(t, photoDir, "legacy.jpg")
	contact.Photo = "legacy.jpg"
	_, err = SetContactMainImage(context.Background(), svc.DB, store, userID, &contact, img, photoDir)
	require.NoError(t, err)
	assert.FileExists(t, legacy, "legacy photo must not be unlinked")
	firstAvatar := filepath.Join(photoDir, contact.Photo)
	assert.FileExists(t, firstAvatar)

	// A previously derived avatar is replaced and its file removed.
	_, err = SetContactMainImage(context.Background(), svc.DB, store, userID, &contact, img, photoDir)
	require.NoError(t, err)
	if filepath.Join(photoDir, contact.Photo) != firstAvatar {
		assert.NoFileExists(t, firstAvatar)
	}
	assert.FileExists(t, filepath.Join(photoDir, contact.Photo))
}

func TestSetContactMainImage_RejectsNonNormalSensitivity(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	for _, sens := range []string{models.ImageSensitivityPrivate, models.ImageSensitivitySecret} {
		img.Sensitivity = sens
		photoDir := t.TempDir()
		_, err = SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, photoDir)
		var appErr *apperrors.AppError
		require.ErrorAs(t, err, &appErr, sens)
		assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode)
		assert.Nil(t, contact.MainImageID)
		entries, _ := os.ReadDir(photoDir)
		assert.Empty(t, entries, "no avatar may be derived from a non-normal image")
	}
}

func TestSetContactMainImage_SaveFailureRemovesDerivedPhoto(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, svc.DB.Exec("DROP TABLE contacts").Error)
	photoDir := t.TempDir()
	_, err = SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, photoDir)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
	entries, readErr := os.ReadDir(photoDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "derived avatar must be removed when the contact save fails")
}

func TestClearContactMainImage_SaveFailure(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	id := uint(1)
	contact.MainImageID = &id
	require.NoError(t, svc.DB.Exec("DROP TABLE contacts").Error)
	_, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, t.TempDir())
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestAdoptImageIntoContact_FromNote(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	store := imaging.Store{Dir: dir}

	row, err := adoptForTest(svc.DB, store, userID, &contact, "note", note.ID, src.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, contact.VCardUID, row.ContactVCardUID)
	assert.NotEqual(t, src.StoredName, row.StoredName, "adopt copies the file")
	assert.Equal(t, src.Sensitivity, row.Sensitivity)
}

func TestAdoptImageIntoContact_FromActivity(t *testing.T) {
	asvc, activity, userID, dir := imageServiceEnv(t)
	contact := models.Contact{UserID: userID, Firstname: "A", VCardUID: "22222222-2222-4222-8222-222222222222"}
	require.NoError(t, asvc.DB.Create(&contact).Error)
	src, err := asvc.Upload(context.Background(), userID, OwnerRef{ID: activity.ID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	store := imaging.Store{Dir: dir}

	// Not linked yet: 404.
	_, err = adoptForTest(asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID, 10)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	require.NoError(t, asvc.DB.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", activity.ID, contact.ID).Error)
	row, err := adoptForTest(asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, contact.VCardUID, row.ContactVCardUID)

	// Wrong image id for a linked activity: 404.
	_, err = adoptForTest(asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID+999, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func TestAdoptImageIntoContact_Rejections(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	store := imaging.Store{Dir: dir}
	other := models.Contact{UserID: userID, Firstname: "O", VCardUID: "33333333-3333-4333-8333-333333333333"}
	require.NoError(t, svc.DB.Create(&other).Error)
	note := models.Note{UserID: userID, ContactID: &other.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)

	var appErr *apperrors.AppError
	// Note filed under another contact.
	_, err := adoptForTest(svc.DB, store, userID, &contact, "note", note.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Unknown note.
	_, err = adoptForTest(svc.DB, store, userID, &contact, "note", note.ID+999, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Right note, wrong image.
	_, err = adoptForTest(svc.DB, store, userID, &other, "note", note.ID, src.ID+999, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Unknown kind.
	_, err = adoptForTest(svc.DB, store, userID, &contact, "bogus", 1, 1, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode)

	// Source file gone: full, then thumbnail.
	require.NoError(t, store.Remove(userID, src.StoredName))
	_, err = adoptForTest(svc.DB, store, userID, &other, "note", note.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	src2 := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	require.NoError(t, store.Remove(userID, src2.ThumbName))
	_, err = adoptForTest(svc.DB, store, userID, &other, "note", note.ID, src2.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func TestAdoptImageIntoContact_LimitAndDBFailures(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	store := imaging.Store{Dir: dir}
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)

	_, err := adoptForTest(svc.DB, store, userID, &contact, "note", note.ID, src.ID, 1)
	require.NoError(t, err)
	var appErr *apperrors.AppError
	other := uploadNoteImageData(t, svc.DB, dir, userID, note.ID, imageTestJPEGVariant(t, 3))
	_, err = adoptForTest(svc.DB, store, userID, &contact, "note", note.ID, other.ID, 1)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageLimitReached), appErr.ReasonCode)

	// Insert failure removes the copied files.
	require.NoError(t, svc.DB.Exec("DROP TABLE contact_images").Error)
	_, err = adoptForTest(svc.DB, store, userID, &contact, "note", note.ID, src.ID, 0)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestAdoptImageIntoContact_ActivityLinkQueryFailure(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	require.NoError(t, svc.DB.Exec("DROP TABLE activity_contacts").Error)
	_, err := adoptForTest(svc.DB, imaging.Store{Dir: dir}, userID, &contact, "activity", 1, 1, 0)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestImageExtension(t *testing.T) {
	assert.Equal(t, "png", imageExtension("IMAGE/PNG"))
	assert.Equal(t, "gif", imageExtension("image/gif"))
	assert.Equal(t, "jpg", imageExtension("image/jpeg"))
	assert.Equal(t, "jpg", imageExtension("image/webp"))
}

func TestListContactPhotos_AggregatesAndOrders(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	ctx := context.Background()
	_, err := svc.Upload(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	noteImg := uploadNoteImage(t, svc.DB, dir, userID, note.ID)

	titled := models.Activity{UserID: userID, Title: "Hike", Date: Now()}
	untitled := models.Activity{UserID: userID, Title: "", Date: Now()}
	require.NoError(t, svc.DB.Create(&titled).Error)
	require.NoError(t, svc.DB.Create(&untitled).Error)
	asvc := ActivityImageService(svc.DB, imaging.Store{Dir: dir}, 10, 50_000_000, 20<<20, UserQuota{})
	for _, a := range []models.Activity{titled, untitled} {
		require.NoError(t, svc.DB.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", a.ID, contact.ID).Error)
		_, err := asvc.Upload(ctx, userID, OwnerRef{ID: a.ID}, bytes.NewReader(imageTestJPEG(t)))
		require.NoError(t, err)
	}

	// Pin the order: the note image is newest via TakenAt.
	newest := time.Now().Add(time.Hour)
	require.NoError(t, svc.DB.Model(&models.NoteImage{}).Where("id = ?", noteImg.ID).Update("taken_at", newest).Error)

	photos, err := ListContactPhotos(ctx, svc.DB, userID, &contact, 0)
	require.NoError(t, err)
	require.Len(t, photos, 4)
	assert.Equal(t, "note", photos[0].Kind)
	labels := map[string]bool{}
	kinds := map[string]int{}
	for _, p := range photos {
		labels[p.SourceLabel] = true
		kinds[p.Kind]++
	}
	assert.Equal(t, map[string]int{"contact": 1, "note": 1, "activity": 2}, kinds)
	assert.True(t, labels["Hike"])
	assert.True(t, labels["Activity"], "untitled activity falls back to a generic label")

	capped, err := ListContactPhotos(ctx, svc.DB, userID, &contact, 2)
	require.NoError(t, err)
	assert.Len(t, capped, 2)
}

func TestListContactPhotos_QueryFailures(t *testing.T) {
	for _, table := range []string{"contact_images", "note_images", "activity_images"} {
		t.Run(table, func(t *testing.T) {
			svc, contact, userID, _ := contactImageServiceEnv(t)
			require.NoError(t, svc.DB.Exec("DROP TABLE "+table).Error)
			_, err := ListContactPhotos(context.Background(), svc.DB, userID, &contact, 10)
			var appErr *apperrors.AppError
			require.ErrorAs(t, err, &appErr)
			assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
		})
	}
}

func TestActivityLabels_QueryFailureAndEmpty(t *testing.T) {
	svc, _, userID, _ := contactImageServiceEnv(t)
	labels, err := activityLabels(context.Background(), svc.DB, userID, nil)
	require.NoError(t, err)
	assert.Empty(t, labels)
	require.NoError(t, svc.DB.Exec("DROP TABLE activities").Error)
	_, err = activityLabels(context.Background(), svc.DB, userID, []models.ActivityImage{{ActivityID: 1}})
	assert.Error(t, err)
}

func TestClearContactMainImage_LeavesLegacyPhotoAlone(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	photoDir := t.TempDir()
	legacy := writePhotoFile(t, photoDir, "legacy.jpg")
	contact.Photo = "legacy.jpg"
	contact.PhotoThumbnail = "thumb"
	got, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, photoDir)
	require.NoError(t, err)
	assert.Equal(t, "legacy.jpg", got.Photo)
	assert.Equal(t, "thumb", got.PhotoThumbnail)
	assert.FileExists(t, legacy)
}

func TestAdoptImageIntoContact_NeverLowersSensitivity(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	require.NoError(t, svc.DB.Model(&models.NoteImage{}).Where("id = ?", src.ID).Update("sensitivity", models.ImageSensitivitySecret).Error)
	row, err := adoptForTest(svc.DB, imaging.Store{Dir: dir}, userID, &contact, "note", note.ID, src.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, models.ImageSensitivitySecret, row.Sensitivity)
}

func TestAdoptImageIntoContact_InsertFailureLeavesNoCopies(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	userDir := filepath.Join(dir, "u"+strconv.FormatUint(uint64(userID), 10))
	before, err := os.ReadDir(userDir)
	require.NoError(t, err)
	require.NoError(t, svc.DB.Exec("DROP TABLE contact_images").Error)
	_, err = adoptForTest(svc.DB, imaging.Store{Dir: dir}, userID, &contact, "note", note.ID, src.ID, 0)
	require.Error(t, err)
	after, err := os.ReadDir(userDir)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "copied files must be removed when the insert fails")
}

func TestAdoptImageIntoContact_TrashedOrForeignActivityIs404(t *testing.T) {
	asvc, activity, userID, dir := imageServiceEnv(t)
	contact := models.Contact{UserID: userID, Firstname: "A", VCardUID: "44444444-4444-4444-8444-444444444444"}
	require.NoError(t, asvc.DB.Create(&contact).Error)
	src, err := asvc.Upload(context.Background(), userID, OwnerRef{ID: activity.ID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, asvc.DB.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", activity.ID, contact.ID).Error)
	store := imaging.Store{Dir: dir}
	var appErr *apperrors.AppError

	// Another user cannot adopt through this activity.
	_, err = adoptForTest(asvc.DB, store, userID+1000, &contact, "activity", activity.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	require.NoError(t, asvc.DB.Delete(&activity).Error)
	_, err = adoptForTest(asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code, "an image of a trashed activity cannot be adopted")
}

func TestListContactPhotos_ScopedToOwnerAndContact(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	ctx := context.Background()
	store := imaging.Store{Dir: dir}
	db := svc.DB

	otherContact := models.Contact{UserID: userID, Firstname: "Other", VCardUID: "55555555-5555-4555-8555-555555555555"}
	require.NoError(t, db.Create(&otherContact).Error)
	stranger := models.User{Username: "stranger", Password: "password123!A", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&stranger).Error)
	strangerContact := models.Contact{UserID: stranger.ID, Firstname: "S", VCardUID: "66666666-6666-4666-8666-666666666666"}
	require.NoError(t, db.Create(&strangerContact).Error)

	// Expected: exactly one note image and one activity image for `contact`.
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "mine"}
	require.NoError(t, db.Create(&note).Error)
	wantNote := uploadNoteImage(t, db, dir, userID, note.ID)
	act := models.Activity{UserID: userID, Title: "mine", Date: Now()}
	require.NoError(t, db.Create(&act).Error)
	asvc := ActivityImageService(db, store, 10, 50_000_000, 20<<20, UserQuota{})
	wantAct, err := asvc.Upload(ctx, userID, OwnerRef{ID: act.ID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", act.ID, contact.ID).Error)

	// Noise that must not appear.
	otherNote := models.Note{UserID: userID, ContactID: &otherContact.ID, Content: "other contact"}
	require.NoError(t, db.Create(&otherNote).Error)
	uploadNoteImage(t, db, dir, userID, otherNote.ID)
	otherAct := models.Activity{UserID: userID, Title: "other", Date: Now()}
	require.NoError(t, db.Create(&otherAct).Error)
	_, err = asvc.Upload(ctx, userID, OwnerRef{ID: otherAct.ID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", otherAct.ID, otherContact.ID).Error)
	_, err = svc.Upload(ctx, userID, OwnerRef{ID: otherContact.ID, UID: otherContact.VCardUID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	trashedNote := models.Note{UserID: userID, ContactID: &contact.ID, Content: "trashed"}
	require.NoError(t, db.Create(&trashedNote).Error)
	uploadNoteImage(t, db, dir, userID, trashedNote.ID)
	require.NoError(t, db.Delete(&trashedNote).Error)
	trashedAct := models.Activity{UserID: userID, Title: "trashed", Date: Now()}
	require.NoError(t, db.Create(&trashedAct).Error)
	_, err = asvc.Upload(ctx, userID, OwnerRef{ID: trashedAct.ID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", trashedAct.ID, contact.ID).Error)
	require.NoError(t, db.Delete(&trashedAct).Error)

	// A stranger's note filed under the contact id (cross-tenant row) must not leak.
	strangerNote := models.Note{UserID: stranger.ID, ContactID: &contact.ID, Content: "foreign"}
	require.NoError(t, db.Create(&strangerNote).Error)
	uploadNoteImage(t, db, dir, stranger.ID, strangerNote.ID)

	// Own gallery: one normal and one secret image (the owner's view returns both).
	ownGallery, err := svc.Upload(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ContactImage{}).Where("id = ?", ownGallery.ID).Update("sensitivity", models.ImageSensitivitySecret).Error)

	photos, err := ListContactPhotos(ctx, db, userID, &contact, 0)
	require.NoError(t, err)
	got := map[string]uint{}
	for _, p := range photos {
		got[p.Kind+":"+strconv.FormatUint(uint64(p.ImageID), 10)] = p.ImageID
	}
	want := map[string]uint{
		"contact:" + strconv.FormatUint(uint64(ownGallery.ID), 10): ownGallery.ID,
		"note:" + strconv.FormatUint(uint64(wantNote.ID), 10):      wantNote.ID,
		"activity:" + strconv.FormatUint(uint64(wantAct.ID), 10):   wantAct.ID,
	}
	assert.Equal(t, want, got)
	for _, p := range photos {
		if p.Kind == "contact" {
			assert.Equal(t, models.ImageSensitivitySecret, p.Sensitivity, "the owner's Photos tab includes secret images")
		}
	}
}

func TestSetContactMainImage_RejectsImageDeletedMeanwhile(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	photoDir := t.TempDir()
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	// The caller loaded the image and read its file, then a concurrent request
	// tombstoned the row (files are removed only after that commit, so they
	// still exist at the moment SetContactMainImage re-checks).
	require.NoError(t, svc.DB.Delete(&models.ContactImage{}, img.ID).Error)

	_, err = SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, photoDir)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
	assert.Nil(t, contact.MainImageID, "in-memory contact is restored")
	assert.Empty(t, contact.Photo)
	var stored models.Contact
	require.NoError(t, svc.DB.First(&stored, contact.ID).Error)
	assert.Nil(t, stored.MainImageID, "no dangling pointer to a tombstone")
	entries, _ := os.ReadDir(photoDir)
	assert.Empty(t, entries)
}

func adoptServiceForTest(db *gorm.DB, store imaging.Store, limit int, quota UserQuota) *ImageService[models.ContactImage, *models.ContactImage] {
	return ContactImageService(db, store, limit, 50_000_000, 20<<20, quota)
}

// adoptForTest runs AdoptImageIntoContact against a contact service with the
// given per-contact cap (0 = unlimited) and no quota.
func adoptForTest(db *gorm.DB, store imaging.Store, userID uint, contact *models.Contact, kind string, sourceID, imageID uint, limit int) (*models.ContactImage, error) {
	row, _, err := AdoptImageIntoContact(context.Background(), adoptServiceForTest(db, store, limit, UserQuota{}), userID, contact, kind, sourceID, imageID)
	return row, err
}

func TestAdoptImageIntoContact_NearDuplicateReturnsExistingEvenAtCap(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	first := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	// A second note image of the same picture (distinct row, same dHash).
	second := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	capped := adoptServiceForTest(svc.DB, imaging.Store{Dir: dir}, 1, UserQuota{})
	ctx := context.Background()

	adopted, dup, err := AdoptImageIntoContact(ctx, capped, userID, &contact, "note", note.ID, first.ID)
	require.NoError(t, err)
	assert.False(t, dup)

	again, dup, err := AdoptImageIntoContact(ctx, capped, userID, &contact, "note", note.ID, second.ID)
	require.NoError(t, err, "a duplicate is exempt from the cap")
	assert.True(t, dup)
	assert.Equal(t, adopted.ID, again.ID)
	count, err := capped.Count(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestAdoptImageIntoContact_EnforcesQuotaAndDisk(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	store := imaging.Store{Dir: dir}
	userDir := filepath.Join(dir, "u"+strconv.FormatUint(uint64(userID), 10))
	before, err := os.ReadDir(userDir)
	require.NoError(t, err)
	var appErr *apperrors.AppError

	// A user at their storage quota cannot grow it by adopting.
	over := adoptServiceForTest(svc.DB, store, 10, UserQuota{AttachmentBytes: 1})
	_, _, err = AdoptImageIntoContact(context.Background(), over, userID, &contact, "note", note.ID, src.ID)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)

	restore := diskspace.StubForTest(0)
	_, _, err = AdoptImageIntoContact(context.Background(), adoptServiceForTest(svc.DB, store, 10, UserQuota{}), userID, &contact, "note", note.ID, src.ID)
	restore()
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)

	after, err := os.ReadDir(userDir)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "a refused adopt copies nothing")
	var rows int64
	require.NoError(t, svc.DB.Model(&models.ContactImage{}).Count(&rows).Error)
	assert.Zero(t, rows)
}

func TestImageService_UploadDedupedDuplicateIsExemptFromCap(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	svc.Limit = 1
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	data := imageTestJPEG(t)
	ctx := context.Background()
	first, _, err := svc.UploadDeduped(ctx, userID, owner, bytes.NewReader(data))
	require.NoError(t, err)

	again, dup, err := svc.UploadDeduped(ctx, userID, owner, bytes.NewReader(data))
	require.NoError(t, err, "a retried upload at the cap is an idempotent 200, not a 409")
	assert.True(t, dup)
	assert.Equal(t, first.ID, again.ID)

	_, _, err = svc.UploadDeduped(ctx, userID, owner, bytes.NewReader(imageTestJPEGVariant(t, 3)))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageLimitReached), appErr.ReasonCode)
}

func TestListContactPhotos_TiesAreOrderedDeterministically(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	ctx := context.Background()
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	a, err := svc.Upload(ctx, userID, owner, bytes.NewReader(imageTestJPEGVariant(t, 1)))
	require.NoError(t, err)
	b, err := svc.Upload(ctx, userID, owner, bytes.NewReader(imageTestJPEGVariant(t, 5)))
	require.NoError(t, err)
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	n := uploadNoteImageData(t, svc.DB, dir, userID, note.ID, imageTestJPEGVariant(t, 7))

	same := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, svc.DB.Model(&models.ContactImage{}).Where("1 = 1").Update("taken_at", same).Error)
	require.NoError(t, svc.DB.Model(&models.NoteImage{}).Where("1 = 1").Update("taken_at", same).Error)

	for i := 0; i < 5; i++ {
		photos, err := ListContactPhotos(ctx, svc.DB, userID, &contact, 0)
		require.NoError(t, err)
		require.Len(t, photos, 3)
		got := []string{photos[0].Kind, photos[1].Kind, photos[2].Kind}
		assert.Equal(t, []string{"contact", "contact", "note"}, got, "equal times order by kind, then image id")
		assert.Equal(t, a.ID, photos[0].ImageID)
		assert.Equal(t, b.ID, photos[1].ImageID)
		assert.Equal(t, n.ID, photos[2].ImageID)
	}
}
