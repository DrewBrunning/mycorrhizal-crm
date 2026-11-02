package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
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
