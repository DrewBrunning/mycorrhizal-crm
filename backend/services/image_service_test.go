package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/imaging"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/diskspace"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func imageTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 5), G: uint8(y * 7), B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func imageServiceEnv(t *testing.T) (*ImageService[models.ActivityImage, *models.ActivityImage], models.Activity, uint, string) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "image-service", Password: "password123!A", Email: "image-service@example.com"}
	require.NoError(t, db.Create(&user).Error)
	activity := models.Activity{UserID: user.ID, Title: "trip", Date: Now()}
	require.NoError(t, db.Create(&activity).Error)
	dir := t.TempDir()
	svc := ActivityImageService(db, imaging.Store{Dir: dir}, 10, 50_000_000, 20<<20, UserQuota{})
	return svc, activity, user.ID, dir
}

func TestImageService_UploadListGetCountDelete(t *testing.T) {
	svc, activity, userID, dir := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID, UID: activity.UUID}

	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	assert.Equal(t, activity.ID, img.ActivityID)
	assert.Equal(t, userID, img.UserID)
	count, err := svc.Count(context.Background(), userID, owner)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	list, err := svc.List(context.Background(), userID, owner)
	require.NoError(t, err)
	require.Len(t, list, 1)

	got, err := svc.Get(context.Background(), userID, owner, img.ID)
	require.NoError(t, err)
	assert.Equal(t, img.UUID, got.UUID)

	require.NoError(t, svc.Delete(context.Background(), userID, owner, img.ID))
	_, err = svc.Get(context.Background(), userID, owner, img.ID)
	assert.Error(t, err)

	for _, name := range []string{img.StoredName, img.ThumbName} {
		_, statErr := os.Stat(filepath.Join(dir, "u1", name))
		assert.True(t, os.IsNotExist(statErr))
	}
}

func TestImageService_LimitBytesQuotaAndDisk(t *testing.T) {
	svc, activity, userID, _ := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID, UID: activity.UUID}
	data := imageTestJPEG(t)

	svc.Limit = 1
	_, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
	require.NoError(t, err)
	_, err = svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageLimitReached), appErr.ReasonCode)

	svc.Limit = 0
	svc.MaxBytes = 1
	_, err = svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageTooLarge), appErr.ReasonCode)

	svc.MaxBytes = 20 << 20
	svc.Quota = UserQuota{AttachmentBytes: 1}
	_, err = svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)

	svc.Quota = UserQuota{}
	restore := diskspace.StubForTest(0)
	coolDown := func() { restore() }
	_, err = svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
	coolDown()
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)
}

func TestImageService_ErrorMappingAndStoreFailure(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code apperrors.ReasonCode
	}{
		{imaging.ErrBusy, apperrors.ReasonImageProcessingBusy},
		{imaging.ErrPixelsExceeded, apperrors.ReasonImagePixelsExceeded},
		{imaging.ErrHEICUnsupported, apperrors.ReasonImageHEICUnsupported},
		{imaging.ErrUnsupportedFormat, apperrors.ReasonImageUnsupportedFormat},
		{imaging.ErrCorrupt, apperrors.ReasonImageCorrupt},
	} {
		got := imageError(tc.err, 10, 1<<20)
		assert.Equal(t, string(tc.code), got.ReasonCode)
	}

	svc, activity, userID, _ := imageServiceEnv(t)
	notDir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notDir, []byte("x"), 0o600))
	svc.Store = imaging.Store{Dir: notDir}
	_, err := svc.Upload(context.Background(), userID, OwnerRef{ID: activity.ID}, bytes.NewReader(imageTestJPEG(t)))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInternal, appErr.Code)
}

func TestImageService_DatabaseFailuresAndNotFound(t *testing.T) {
	svc, activity, userID, _ := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID}

	_, err := svc.Get(context.Background(), userID, owner, 999)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
	assert.Error(t, svc.Delete(context.Background(), userID, owner, 999))

	_, err = svc.Upload(context.Background(), userID, owner, bytes.NewReader([]byte{0xFF, 0xD8, 0xFF, 0xD9}))
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonImageCorrupt), appErr.ReasonCode)

	dbtest.HideTable(t, svc.DB, "activity_images")
	_, err = svc.List(context.Background(), userID, owner)
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
	_, err = svc.Count(context.Background(), userID, owner)
	require.ErrorAs(t, err, &appErr)
	_, err = svc.Get(context.Background(), userID, owner, 1)
	require.ErrorAs(t, err, &appErr)
}

func failImageTableOn(t *testing.T, svc *ImageService[models.ActivityImage, *models.ActivityImage], kind string) {
	t.Helper()
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table == "activity_images" {
			tx.AddError(errors.New("simulated image table failure"))
		}
	}
	name := "fail-image-" + kind
	switch kind {
	case "create":
		require.NoError(t, svc.DB.Callback().Create().Before("gorm:create").Register(name, fail))
	case "delete":
		require.NoError(t, svc.DB.Callback().Delete().Before("gorm:delete").Register(name, fail))
	default:
		t.Fatalf("unknown kind %q", kind)
	}
}

func TestImageService_CreateAndDeleteDatabaseFailures(t *testing.T) {
	svc, activity, userID, dir := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID}

	failImageTableOn(t, svc, "create")
	_, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
	// The files written before the failed insert are removed again.
	entries, readErr := os.ReadDir(filepath.Join(dir, "u"+strconv.FormatUint(uint64(userID), 10)))
	if readErr == nil {
		assert.Empty(t, entries)
	}
}

func TestImageService_DeleteUsesNumericAuditIDWhenUIDIsEmpty(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "image-audit", Password: "password123!A", Email: "image-audit@example.com"}
	require.NoError(t, db.Create(&user).Error)
	activity := models.Activity{UserID: user.ID, Title: "trip", Date: Now()}
	require.NoError(t, db.Create(&activity).Error)
	svc := ActivityImageService(db, imaging.Store{Dir: t.TempDir()}, 10, 50_000_000, 20<<20, UserQuota{})
	owner := OwnerRef{ID: activity.ID}
	img, err := svc.Upload(context.Background(), user.ID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	failImageTableOn(t, svc, "delete")
	err = svc.Delete(context.Background(), user.ID, owner, img.ID)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestImageService_DeleteLogsFileRemovalFailures(t *testing.T) {
	svc, activity, userID, dir := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	// Replace each stored file with a directory so Remove fails with a
	// non-IsNotExist error; the row delete must still succeed.
	for _, name := range []string{img.StoredName, img.ThumbName} {
		path := filepath.Join(dir, "u"+strconv.FormatUint(uint64(userID), 10), name)
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.Mkdir(path, 0o700))
	}
	assert.NoError(t, svc.Delete(context.Background(), userID, owner, img.ID))
}

func TestImageService_OwnerValueForContactUsesUID(t *testing.T) {
	var svc ImageService[models.ActivityImage, *models.ActivityImage]
	svc.Kind = OwnerKindContact
	assert.Equal(t, "contact-uid", svc.ownerValue(OwnerRef{UID: "contact-uid", ID: 9}))
	svc.Kind = OwnerKindActivity
	assert.Equal(t, uint(9), svc.ownerValue(OwnerRef{UID: "contact-uid", ID: 9}))
}

func TestImageService_QuotaImageQueryFailure(t *testing.T) {
	svc, activity, userID, _ := imageServiceEnv(t)
	svc.Quota = UserQuota{AttachmentBytes: 100}
	require.NoError(t, svc.DB.Exec("DROP TABLE activity_images").Error)
	_, err := svc.Upload(context.Background(), userID, OwnerRef{ID: activity.ID}, bytes.NewReader(imageTestJPEG(t)))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestImageService_AuditEntityByKind(t *testing.T) {
	var svc ImageService[models.NoteImage, *models.NoteImage]
	svc.Kind = OwnerKindNote
	assert.Equal(t, models.AuditEntityNote, svc.auditEntity())
	svc.Kind = OwnerKindActivity
	assert.Equal(t, models.AuditEntityActivity, svc.auditEntity())
	svc.Kind = OwnerKindContact
	assert.Equal(t, models.AuditEntityContact, svc.auditEntity())
	svc.Kind = OwnerKind("")
	assert.Equal(t, models.AuditEntityActivity, svc.auditEntity())
}

func contactImageServiceEnv(t *testing.T) (*ImageService[models.ContactImage, *models.ContactImage], models.Contact, uint, string) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "contact-image-service", Password: "password123!A", Email: "contact-image-service@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Dana", VCardUID: "11111111-1111-4111-8111-111111111111"}
	require.NoError(t, db.Create(&contact).Error)
	dir := t.TempDir()
	svc := ContactImageService(db, imaging.Store{Dir: dir}, 10, 50_000_000, 20<<20, UserQuota{})
	return svc, contact, user.ID, dir
}

func TestImageService_UploadDedupedReturnsExistingContactImage(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	data := imageTestJPEG(t)

	first, dup, err := svc.UploadDeduped(context.Background(), userID, owner, bytes.NewReader(data))
	require.NoError(t, err)
	assert.False(t, dup)

	second, dup, err := svc.UploadDeduped(context.Background(), userID, owner, bytes.NewReader(data))
	require.NoError(t, err)
	assert.True(t, dup)
	assert.Equal(t, first.ID, second.ID)

	count, err := svc.Count(context.Background(), userID, owner)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestImageService_UpdateSensitivityAndCrop(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	private := models.ImageSensitivityPrivate
	crop := &models.ImageCrop{X: 0.1, Y: 0.2, W: 0.5, H: 0.6}
	updated, err := svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{
		Sensitivity: &private,
		Crop:        models.OptionalCrop{Set: true, Value: crop},
	})
	require.NoError(t, err)
	assert.Equal(t, models.ImageSensitivityPrivate, updated.Sensitivity)
	require.NotNil(t, updated.CropX)
	assert.InDelta(t, 0.1, *updated.CropX, 0.0001)
	assert.InDelta(t, 0.6, *updated.CropH, 0.0001)

	cleared, err := svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{Crop: models.OptionalCrop{Set: true}})
	require.NoError(t, err)
	assert.Nil(t, cleared.CropX)
	assert.Nil(t, cleared.CropW)
}

func TestImageService_UpdateRejectsInvalidValues(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	bad := "bogus"
	_, err = svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{Sensitivity: &bad})
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode)

	_, err = svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{
		Crop: models.OptionalCrop{Set: true, Value: &models.ImageCrop{X: 0.8, Y: 0.8, W: 0.5, H: 0.5}},
	})
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode)
}

func TestImageService_UploadDedupedBranches(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	ctx := context.Background()

	// processUpload failure propagates.
	_, _, err := svc.UploadDeduped(ctx, userID, owner, bytes.NewReader([]byte("not an image")))
	require.Error(t, err)

	// Non-contact owners never dedupe.
	asvc, activity, auid, _ := imageServiceEnv(t)
	aowner := OwnerRef{ID: activity.ID}
	data := imageTestJPEG(t)
	_, dup, err := asvc.UploadDeduped(ctx, auid, aowner, bytes.NewReader(data))
	require.NoError(t, err)
	assert.False(t, dup)
	_, dup, err = asvc.UploadDeduped(ctx, auid, aowner, bytes.NewReader(data))
	require.NoError(t, err)
	assert.False(t, dup)

	// Duplicate-lookup query failure.
	require.NoError(t, svc.DB.Exec("DROP TABLE contact_images").Error)
	_, _, err = svc.UploadDeduped(ctx, userID, owner, bytes.NewReader(data))
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestImageService_UpdateBranches(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	ctx := context.Background()

	// Unknown image id.
	_, err := svc.Update(ctx, userID, owner, 9999, models.ContactImagePatchInput{})
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	img, err := svc.Upload(ctx, userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)

	// Empty patch returns the row untouched.
	same, err := svc.Update(ctx, userID, owner, img.ID, models.ContactImagePatchInput{})
	require.NoError(t, err)
	assert.Equal(t, img.ID, same.ID)

	// Update failure surfaces as a database error.
	require.NoError(t, svc.DB.Callback().Update().Before("gorm:update").Register("test:fail_update", func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("boom"))
	}))
	secret := models.ImageSensitivitySecret
	_, err = svc.Update(ctx, userID, owner, img.ID, models.ContactImagePatchInput{Sensitivity: &secret})
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeDatabase, appErr.Code)
}

func TestImageService_UploadDedupedIsScopedToOwnerContact(t *testing.T) {
	svc, contactA, userID, _ := contactImageServiceEnv(t)
	ctx := context.Background()
	data := imageTestJPEG(t)
	contactB := models.Contact{UserID: userID, Firstname: "B", VCardUID: "77777777-7777-4777-8777-777777777777"}
	require.NoError(t, svc.DB.Create(&contactB).Error)
	stranger := models.User{Username: "dedupe-stranger", Password: "password123!A", Email: "dedupe-stranger@example.com"}
	require.NoError(t, svc.DB.Create(&stranger).Error)
	strangerContact := models.Contact{UserID: stranger.ID, Firstname: "S", VCardUID: "88888888-8888-4888-8888-888888888888"}
	require.NoError(t, svc.DB.Create(&strangerContact).Error)

	first, dup, err := svc.UploadDeduped(ctx, userID, OwnerRef{ID: contactA.ID, UID: contactA.VCardUID}, bytes.NewReader(data))
	require.NoError(t, err)
	require.False(t, dup)

	// The same picture on another contact, or for another user, is a new row.
	other, dup, err := svc.UploadDeduped(ctx, userID, OwnerRef{ID: contactB.ID, UID: contactB.VCardUID}, bytes.NewReader(data))
	require.NoError(t, err)
	assert.False(t, dup, "dedupe must not cross contacts")
	assert.NotEqual(t, first.ID, other.ID)

	foreign, dup, err := svc.UploadDeduped(ctx, stranger.ID, OwnerRef{ID: strangerContact.ID, UID: strangerContact.VCardUID}, bytes.NewReader(data))
	require.NoError(t, err)
	assert.False(t, dup, "dedupe must not cross users")
	assert.Equal(t, stranger.ID, foreign.UserID)
}

func TestImageService_UpdateCropValidation(t *testing.T) {
	svc, contact, userID, _ := contactImageServiceEnv(t)
	owner := OwnerRef{ID: contact.ID, UID: contact.VCardUID}
	img, err := svc.Upload(context.Background(), userID, owner, bytes.NewReader(imageTestJPEG(t)))
	require.NoError(t, err)
	nan := math.NaN()
	for name, c := range map[string]models.ImageCrop{
		"nan x":         {X: nan, Y: 0, W: 0.5, H: 0.5},
		"nan w":         {X: 0, Y: 0, W: nan, H: 0.5},
		"inf w":         {X: 0, Y: 0, W: math.Inf(1), H: 0.5},
		"negative x":    {X: -0.1, Y: 0, W: 0.5, H: 0.5},
		"zero width":    {X: 0, Y: 0, W: 0, H: 0.5},
		"overflow in x": {X: 0.6, Y: 0, W: 0.5, H: 0.5},
	} {
		crop := c
		_, err := svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{Crop: models.OptionalCrop{Set: true, Value: &crop}})
		var appErr *apperrors.AppError
		require.ErrorAs(t, err, &appErr, name)
		assert.Equal(t, string(apperrors.ReasonValidationInvalid), appErr.ReasonCode, name)
	}
	// Float rounding at the edge is tolerated; the full image is a valid crop.
	for _, c := range []models.ImageCrop{{X: 0.7, Y: 0, W: 0.30000000000000004, H: 1}, {X: 0, Y: 0, W: 1, H: 1}} {
		crop := c
		_, err := svc.Update(context.Background(), userID, owner, img.ID, models.ContactImagePatchInput{Crop: models.OptionalCrop{Set: true, Value: &crop}})
		require.NoError(t, err)
	}
}

// uploadConcurrently fires n uploads of data at one owner and returns the
// per-call errors. Each goroutine waits on a start gate so the cheap pre-check
// races the way N multi-select picker uploads do.
func uploadConcurrently(svc *ImageService[models.ActivityImage, *models.ActivityImage], userID uint, owner OwnerRef, data []byte, n int) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	// Park every upload after its pre-checks so all N pass `count < Limit` (and
	// the quota check) against the same empty state before any of them inserts.
	// That makes the race deterministic instead of timing-dependent.
	var arrived sync.WaitGroup
	arrived.Add(n)
	svc.beforeInsert = func() {
		arrived.Done()
		arrived.Wait()
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Upload(context.Background(), userID, owner, bytes.NewReader(data))
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	require.NoError(t, filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return err
	}))
	return n
}

// Issue #2313: the per-owner limit was a check-then-act race, so N concurrent
// uploads all passed `count < Limit` and all inserted.
func TestImageService_ConcurrentUploadsRespectOwnerLimit(t *testing.T) {
	svc, activity, userID, dir := imageServiceEnv(t)
	svc.Limit = 1
	owner := OwnerRef{ID: activity.ID, UID: activity.UUID}

	errs := uploadConcurrently(svc, userID, owner, imageTestJPEG(t), 8)

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		var appErr *apperrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, string(apperrors.ReasonImageLimitReached), appErr.ReasonCode)
		assert.Equal(t, http.StatusConflict, appErr.HTTPStatus)
	}
	assert.Equal(t, 1, successes, "exactly one concurrent upload may win a limit of 1")
	count, err := svc.Count(context.Background(), userID, owner)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, 2, countFiles(t, dir), "the losers' files are removed; only the winner's full+thumb pair remains")
}

// The opt-in byte quota had the same race: every concurrent upload saw the
// same `used` total.
func TestImageService_ConcurrentUploadsRespectUserQuota(t *testing.T) {
	svc, activity, userID, dir := imageServiceEnv(t)
	owner := OwnerRef{ID: activity.ID, UID: activity.UUID}
	data := imageTestJPEG(t)

	probe, err := imaging.Process(bytes.NewReader(data), imaging.DefaultLimits)
	require.NoError(t, err)
	one := int64(len(probe.Full) + len(probe.Thumb))
	svc.Quota = UserQuota{AttachmentBytes: one + one/2} // room for one upload, not two

	errs := uploadConcurrently(svc, userID, owner, data, 8)

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		var appErr *apperrors.AppError
		require.ErrorAs(t, err, &appErr)
		assert.Equal(t, http.StatusInsufficientStorage, appErr.HTTPStatus)
	}
	assert.Equal(t, 1, successes)
	count, err := svc.Count(context.Background(), userID, owner)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Equal(t, 2, countFiles(t, dir))
}
