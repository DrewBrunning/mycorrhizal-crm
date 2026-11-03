package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/imaging"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func galleryEnv(t *testing.T) contactImageTestEnv {
	t.Helper()
	env := setupContactImageRouter(t)
	env.router.PATCH("/contacts/:id/images/:image_id", PatchContactImage)
	env.router.PUT("/contacts/:id/main-image", PutContactMainImage)
	env.router.DELETE("/contacts/:id/main-image", DeleteContactMainImage)
	env.router.POST("/contacts/:id/images/adopt", AdoptContactImage)
	env.router.GET("/contacts/:id/photos", GetContactPhotos)
	return env
}

func galleryDo(env contactImageTestEnv, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s)
	} else if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func galleryPath(contactID uint, suffix string) string {
	return "/contacts/" + strconv.FormatUint(uint64(contactID), 10) + suffix
}

func galleryUpload(t *testing.T, env contactImageTestEnv, contactID uint, seed int) uint {
	t.Helper()
	rec := uploadContactImageForTest(t, env, contactID, testJPEGVariant(t, seed))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var out struct {
		Image ContactImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.Image.ID
}

func TestPatchContactImage(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Patch"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	p := galleryPath(contact.ID, "/images/"+strconv.FormatUint(uint64(id), 10))

	rec := galleryDo(env, "PATCH", p, map[string]any{"sensitivity": "private", "crop": map[string]float64{"x": 0.1, "y": 0.1, "w": 0.5, "h": 0.5}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var row models.ContactImage
	require.NoError(t, env.db.First(&row, id).Error)
	assert.Equal(t, models.ImageSensitivityPrivate, row.Sensitivity)
	require.NotNil(t, row.CropW)

	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "PATCH", p, "{not json").Code)
	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "PATCH", p, map[string]any{"sensitivity": "bogus"}).Code)
	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "PATCH", galleryPath(contact.ID, "/images/abc"), map[string]any{}).Code)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "PATCH", galleryPath(contact.ID, "/images/9999"), map[string]any{}).Code)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "PATCH", galleryPath(9999, "/images/1"), map[string]any{}).Code)
}

func TestContactMainImage_SetAndClear(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Main"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	p := galleryPath(contact.ID, "/main-image")

	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "PUT", p, map[string]any{}).Code)
	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "PUT", p, "{bad").Code)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "PUT", p, map[string]any{"image_id": 9999}).Code)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "PUT", galleryPath(9999, "/main-image"), map[string]any{"image_id": id}).Code)

	rec := galleryDo(env, "PUT", p, map[string]any{"image_id": id})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.Contact
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	require.NotNil(t, got.MainImageID)
	assert.Equal(t, id, *got.MainImageID)
	assert.NotEmpty(t, got.Photo)

	rec = galleryDo(env, "DELETE", p, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)
	assert.Empty(t, got.Photo)

	assert.Equal(t, http.StatusNotFound, galleryDo(env, "DELETE", galleryPath(9999, "/main-image"), nil).Code)
}

func TestAdoptContactImage_AndPhotosTab(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Adopt"}
	require.NoError(t, env.db.Create(&contact).Error)
	note := models.Note{UserID: env.user.ID, ContactID: &contact.ID, Content: "n"}
	require.NoError(t, env.db.Create(&note).Error)

	// Seed a note image by uploading through the note service path.
	noteImgID := seedNoteImage(t, env, note.ID)
	p := galleryPath(contact.ID, "/images/adopt")

	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "POST", p, map[string]any{}).Code)
	assert.Equal(t, http.StatusBadRequest, galleryDo(env, "POST", p, map[string]any{"source_kind": "bogus", "source_id": note.ID, "image_id": noteImgID}).Code)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "POST", galleryPath(9999, "/images/adopt"), map[string]any{"source_kind": "note", "source_id": note.ID, "image_id": noteImgID}).Code)

	rec := galleryDo(env, "POST", p, map[string]any{"source_kind": "note", "source_id": note.ID, "image_id": noteImgID})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	galleryUpload(t, env, contact.ID, 3)
	// Order is by taken/created time; pin the note image newest.
	require.NoError(t, env.db.Model(&models.NoteImage{}).Where("id = ?", noteImgID).Update("taken_at", time.Now().Add(time.Hour)).Error)

	rec = galleryDo(env, "GET", galleryPath(contact.ID, "/photos"), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Photos []ContactPhotoDTO `json:"photos"`
		Total  int               `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, 3, out.Total)
	require.Len(t, out.Photos, 3)
	assert.Equal(t, "note", out.Photos[0].Kind)
	assert.Equal(t, "/api/v1/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(noteImgID), 10)+"/file", out.Photos[0].FileURL)
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "GET", galleryPath(9999, "/photos"), nil).Code)
}

func TestContactPhotoDTO_URLsByKind(t *testing.T) {
	act := contactPhotoDTO(servicesPhoto("activity", 7, 9), 3)
	assert.Equal(t, "/api/v1/activities/7/images/9/thumbnail", act.ThumbnailURL)
	own := contactPhotoDTO(servicesPhoto("contact", 3, 9), 3)
	assert.Equal(t, "/api/v1/contacts/3/images/9/file", own.FileURL)
}

func servicesPhoto(kind string, sourceID, imageID uint) services.ContactPhoto {
	return services.ContactPhoto{Kind: kind, SourceID: sourceID, ImageID: imageID}
}

func seedNoteImage(t *testing.T, env contactImageTestEnv, noteID uint) uint {
	t.Helper()
	svc := services.NoteImageService(env.db, imaging.Store{Dir: env.dir}, 10, 50_000_000, 20<<20, services.UserQuota{})
	img, err := svc.Upload(t.Context(), env.user.ID, services.OwnerRef{ID: noteID}, bytes.NewReader(testJPEGVariant(t, 5)))
	require.NoError(t, err)
	return img.ID
}

func TestContactGalleryHandlers_DatabaseFailures(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Broken"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	img := "/images/" + strconv.FormatUint(uint64(id), 10)
	require.NoError(t, env.db.Exec("DROP TABLE contact_images").Error)
	require.NoError(t, env.db.Exec("DROP TABLE note_images").Error)

	assert.Equal(t, http.StatusInternalServerError, galleryDo(env, "PATCH", galleryPath(contact.ID, img), map[string]any{"sensitivity": "private"}).Code)
	assert.Equal(t, http.StatusInternalServerError, galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id}).Code)
	assert.Equal(t, http.StatusInternalServerError, galleryDo(env, "GET", galleryPath(contact.ID, "/photos"), nil).Code)
	assert.Equal(t, http.StatusInternalServerError, galleryDo(env, "POST", galleryPath(contact.ID, "/images/adopt"), map[string]any{"source_kind": "activity", "source_id": 1, "image_id": 1}).Code)
}

func TestContactImages_DuplicateUploadReturnsExisting(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Dup"}
	require.NoError(t, env.db.Create(&contact).Error)
	data := testJPEGVariant(t, 1)
	first := uploadContactImageForTest(t, env, contact.ID, data)
	require.Equal(t, http.StatusCreated, first.Code)
	second := uploadContactImageForTest(t, env, contact.ID, data)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"deduplicated":true`)
}

func TestContactImages_DeletingMainImageClearsAvatar(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "MainDel"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	require.Equal(t, http.StatusOK, galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id}).Code)

	rec := galleryDo(env, "DELETE", galleryPath(contact.ID, "/images/"+strconv.FormatUint(uint64(id), 10)), nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.Contact
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)
	assert.Empty(t, got.Photo)
}

func TestContactMainImage_ServiceFailuresSurface(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Fail"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	var row models.ContactImage
	require.NoError(t, env.db.First(&row, id).Error)

	// Stored file gone: setting the main image is a 404.
	require.NoError(t, os.Remove(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName)))
	assert.Equal(t, http.StatusNotFound, galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id}).Code)

	// Contact save failure while clearing a main image is a 500.
	require.NoError(t, env.db.Model(&models.Contact{}).Where("id = ?", contact.ID).Update("main_image_id", id).Error)
	require.NoError(t, env.db.Callback().Update().Before("gorm:update").Register("test:fail_contact_save", func(tx *gorm.DB) {
		if tx.Statement.Table == "contacts" {
			_ = tx.AddError(errors.New("boom"))
		}
	}))
	assert.Equal(t, http.StatusInternalServerError, galleryDo(env, "DELETE", galleryPath(contact.ID, "/main-image"), nil).Code)
}

func contactRevision(t *testing.T, env contactImageTestEnv, id uint) int64 {
	t.Helper()
	var c models.Contact
	require.NoError(t, env.db.First(&c, id).Error)
	return c.Revision
}

func TestContactMainImage_IfMatch(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "IfMatch"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	p := galleryPath(contact.ID, "/main-image")
	do := func(method, ifMatch string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, p, &buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", ifMatch)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		return rec
	}
	start := contactRevision(t, env, contact.ID)

	// A stale precondition is rejected and nothing changes.
	assert.Equal(t, http.StatusPreconditionFailed, do("PUT", `"999999"`, map[string]any{"image_id": id}).Code)
	var got models.Contact
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)
	assert.Equal(t, start, got.Revision)

	// The current revision is accepted, bumps the revision and reports it.
	rec := do("PUT", `"`+strconv.FormatInt(start, 10)+`"`, map[string]any{"image_id": id})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Revision int64 `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	now := contactRevision(t, env, contact.ID)
	assert.Greater(t, now, start)
	assert.Equal(t, now, out.Revision, "response carries the new revision")

	assert.Equal(t, http.StatusPreconditionFailed, do("DELETE", `"`+strconv.FormatInt(start, 10)+`"`, nil).Code)
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	assert.NotNil(t, got.MainImageID, "a stale delete must not clear the main image")
	assert.Equal(t, http.StatusOK, do("DELETE", `"`+strconv.FormatInt(now, 10)+`"`, nil).Code)
}

func TestContactMainImage_ConcurrentContactWriteIs412Not500(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Race"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)

	// Simulate another writer bumping the revision between load and save.
	require.NoError(t, env.db.Callback().Update().Before("gorm:update").Register("test:concurrent_writer", func(tx *gorm.DB) {
		if tx.Statement.Table == "contacts" {
			_ = tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE contacts SET revision = revision + 1 WHERE id = ?", contact.ID).Error
		}
	}))
	rec := galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id})
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code, rec.Body.String())
	var got models.Contact
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	assert.Nil(t, got.MainImageID)
	entries, err := os.ReadDir(env.profilePhotoDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the derived avatar is removed when the save loses the race")
}

func TestContactImages_PatchCropRegeneratesMainAvatar(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Regen"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	require.Equal(t, http.StatusOK, galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id}).Code)
	var before models.Contact
	require.NoError(t, env.db.First(&before, contact.ID).Error)

	p := galleryPath(contact.ID, "/images/"+strconv.FormatUint(uint64(id), 10))
	rec := galleryDo(env, "PATCH", p, map[string]any{"crop": map[string]float64{"x": 0, "y": 0, "w": 0.5, "h": 0.5}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var after models.Contact
	require.NoError(t, env.db.First(&after, contact.ID).Error)
	assert.NotEqual(t, before.PhotoThumbnail, after.PhotoThumbnail, "a crop change regenerates the avatar")
	assert.Greater(t, after.Revision, before.Revision)
	assert.NoFileExists(t, filepath.Join(env.profilePhotoDir, before.Photo), "the superseded avatar file is removed")
	assert.FileExists(t, filepath.Join(env.profilePhotoDir, after.Photo))

	// Making the main image non-normal clears the avatar entirely.
	require.Equal(t, http.StatusOK, galleryDo(env, "PATCH", p, map[string]any{"sensitivity": "private"}).Code)
	require.NoError(t, env.db.First(&after, contact.ID).Error)
	assert.Nil(t, after.MainImageID)
	assert.Empty(t, after.Photo)

	// A sensitivity or crop change on a non-main image never touches the contact.
	rev := contactRevision(t, env, contact.ID)
	require.Equal(t, http.StatusOK, galleryDo(env, "PATCH", p, map[string]any{"sensitivity": "normal"}).Code)
	assert.Equal(t, rev, contactRevision(t, env, contact.ID))
}

func TestContactImages_DeleteMainImageIsAtomic(t *testing.T) {
	env := galleryEnv(t)
	contact := models.Contact{UserID: env.user.ID, Firstname: "Atomic"}
	require.NoError(t, env.db.Create(&contact).Error)
	id := galleryUpload(t, env, contact.ID, 0)
	require.Equal(t, http.StatusOK, galleryDo(env, "PUT", galleryPath(contact.ID, "/main-image"), map[string]any{"image_id": id}).Code)
	var row models.ContactImage
	require.NoError(t, env.db.First(&row, id).Error)

	require.NoError(t, env.db.Callback().Update().Before("gorm:update").Register("test:fail_contact_save", func(tx *gorm.DB) {
		if tx.Statement.Table == "contacts" {
			_ = tx.AddError(errors.New("boom"))
		}
	}))
	rec := galleryDo(env, "DELETE", galleryPath(contact.ID, "/images/"+strconv.FormatUint(uint64(id), 10)), nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	// Neither half happened: the image is live, its files exist, the pointer stays.
	var live int64
	require.NoError(t, env.db.Model(&models.ContactImage{}).Where("id = ?", id).Count(&live).Error)
	assert.Equal(t, int64(1), live, "image must not be deleted when clearing the main image fails")
	assert.FileExists(t, filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName))
	var got models.Contact
	require.NoError(t, env.db.First(&got, contact.ID).Error)
	require.NotNil(t, got.MainImageID)
	assert.NotEmpty(t, got.Photo)
}
