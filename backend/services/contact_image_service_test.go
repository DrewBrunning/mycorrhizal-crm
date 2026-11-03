package services

import (
	"bytes"
	"context"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func uploadNoteImage(t *testing.T, db *gorm.DB, dir string, userID, noteID uint) *models.NoteImage {
	t.Helper()
	svc := NoteImageService(db, imaging.Store{Dir: dir}, 10, 50_000_000, 20<<20, UserQuota{})
	img, err := svc.Upload(context.Background(), userID, OwnerRef{ID: noteID}, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	return img
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

	cleared, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, photoDir)
	require.NoError(t, err)
	assert.Nil(t, cleared.MainImageID)
	assert.Empty(t, cleared.Photo)

	// Clearing again is a no-op.
	again, err := ClearContactMainImage(context.Background(), svc.DB, userID, &contact, photoDir)
	require.NoError(t, err)
	assert.Nil(t, again.MainImageID)
}

func TestSetContactMainImage_MissingFileIsNotFound(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	img := &models.ContactImage{ImageFields: models.ImageFields{StoredName: "missing.jpg"}}
	_, err := SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, t.TempDir())
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func TestSetContactMainImage_ReplacesOldPhoto(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	photoDir := t.TempDir()
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	contact.Photo = "old-photo.jpg"
	_, err = SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, photoDir)
	require.NoError(t, err)
	assert.NotEqual(t, "old-photo.jpg", contact.Photo)
}

func TestSetContactMainImage_SaveFailureRemovesDerivedPhoto(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	require.NoError(t, svc.DB.Exec("DROP TABLE contacts").Error)
	_, err = SetContactMainImage(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, img, t.TempDir())
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
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

	row, err := AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID, src.ID, 10)
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
	_, err = AdoptImageIntoContact(context.Background(), asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID, 10)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	require.NoError(t, asvc.DB.Exec("INSERT INTO activity_contacts (activity_id, contact_id) VALUES (?, ?)", activity.ID, contact.ID).Error)
	row, err := AdoptImageIntoContact(context.Background(), asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID, 10)
	require.NoError(t, err)
	assert.Equal(t, contact.VCardUID, row.ContactVCardUID)

	// Wrong image id for a linked activity: 404.
	_, err = AdoptImageIntoContact(context.Background(), asvc.DB, store, userID, &contact, "activity", activity.ID, src.ID+999, 10)
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
	_, err := AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Unknown note.
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID+999, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Right note, wrong image.
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &other, "note", note.ID, src.ID+999, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	// Unknown kind.
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "bogus", 1, 1, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode)

	// Source file gone: full, then thumbnail.
	require.NoError(t, store.Remove(userID, src.StoredName))
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &other, "note", note.ID, src.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	src2 := uploadNoteImage(t, svc.DB, dir, userID, note.ID)
	require.NoError(t, store.Remove(userID, src2.ThumbName))
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &other, "note", note.ID, src2.ID, 10)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func TestAdoptImageIntoContact_LimitAndDBFailures(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	store := imaging.Store{Dir: dir}
	note := models.Note{UserID: userID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, svc.DB.Create(&note).Error)
	src := uploadNoteImage(t, svc.DB, dir, userID, note.ID)

	_, err := AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID, src.ID, 1)
	require.NoError(t, err)
	var appErr *apperrors.AppError
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID, src.ID, 1)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageLimitReached), appErr.ReasonCode)

	// Insert failure removes the copied files.
	require.NoError(t, svc.DB.Exec("DROP TABLE contact_images").Error)
	_, err = AdoptImageIntoContact(context.Background(), svc.DB, store, userID, &contact, "note", note.ID, src.ID, 0)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestAdoptImageIntoContact_ActivityLinkQueryFailure(t *testing.T) {
	svc, contact, userID, dir := contactImageServiceEnv(t)
	require.NoError(t, svc.DB.Exec("DROP TABLE activity_contacts").Error)
	_, err := AdoptImageIntoContact(context.Background(), svc.DB, imaging.Store{Dir: dir}, userID, &contact, "activity", 1, 1, 0)
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
	own, err := svc.Upload(ctx, userID, OwnerRef{ID: contact.ID, UID: contact.VCardUID}, bytes.NewReader(imageTestJPEG(t)))
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
	_ = own

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
	labels, err := activityLabels(svc.DB, userID, nil)
	require.NoError(t, err)
	assert.Empty(t, labels)
	require.NoError(t, svc.DB.Exec("DROP TABLE activities").Error)
	_, err = activityLabels(svc.DB, userID, []models.ActivityImage{{ActivityID: 1}})
	assert.Error(t, err)
}
