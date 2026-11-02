package controllers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type imageTestEnv struct {
	db     *gorm.DB
	router *gin.Engine
	user   models.User
	dir    string
}

func setupImageRouter(t *testing.T) imageTestEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "image-owner", Password: "password123!A", Email: "image-owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := filepath.Join(t.TempDir(), "images")
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), ImagesDir: dir, ImageMaxUploadMB: 20, ImageMaxPixels: 50_000_000, ImageMaxPerActivity: 10}
	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/activities/:id/images", UploadActivityImage)
	router.GET("/activities/:id/images", ListActivityImages)
	router.GET("/activities/:id/images/:image_id/file", func(c *gin.Context) { DownloadActivityImage(c, false) })
	router.GET("/activities/:id/images/:image_id/thumbnail", func(c *gin.Context) { DownloadActivityImage(c, true) })
	router.DELETE("/activities/:id/images/:image_id", DeleteActivityImage)
	return imageTestEnv{db: db, router: router, user: user, dir: dir}
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 4), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func uploadActivityImage(t *testing.T, env imageTestEnv, activityID uint, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "photo.jpg")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req, _ := http.NewRequest("POST", "/activities/"+strconv.FormatUint(uint64(activityID), 10)+"/images", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func TestActivityImages_UploadListDownloadDelete(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)
	data := testJPEG(t)

	rec := uploadActivityImage(t, env, activity.ID, data)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Image ActivityImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, activity.ID, created.Image.ActivityID)
	assert.Equal(t, "image/jpeg", created.Image.MIMEType)
	assert.NotEmpty(t, created.Image.FileURL)

	var row models.ActivityImage
	require.NoError(t, env.db.First(&row, created.Image.ID).Error)
	_, err := os.Stat(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName))
	require.NoError(t, err, "stored image must exist on disk")

	list, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", nil)
	listRec := httptest.NewRecorder()
	env.router.ServeHTTP(listRec, list)
	require.Equal(t, http.StatusOK, listRec.Code)
	var body struct {
		Images []ActivityImageDTO `json:"images"`
		Total  int                `json:"total"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &body))
	require.Len(t, body.Images, 1)
	assert.Equal(t, 1, body.Total)

	fileReq, _ := http.NewRequest("GET", strings.TrimPrefix(created.Image.FileURL, "/api/v1"), nil)
	fileRec := httptest.NewRecorder()
	env.router.ServeHTTP(fileRec, fileReq)
	require.Equal(t, http.StatusOK, fileRec.Code)
	assert.Equal(t, "image/jpeg", fileRec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", fileRec.Header().Get("X-Content-Type-Options"))

	delReq, _ := http.NewRequest("DELETE", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/"+strconv.FormatUint(uint64(row.ID), 10), nil)
	delRec := httptest.NewRecorder()
	env.router.ServeHTTP(delRec, delReq)
	require.Equal(t, http.StatusOK, delRec.Code, delRec.Body.String())
	_, err = os.Stat(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName))
	assert.True(t, os.IsNotExist(err), "deleted image file must be removed")
	var count int64
	require.NoError(t, env.db.Model(&models.ActivityImage{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestActivityImages_ForeignActivityIs404(t *testing.T) {
	env := setupImageRouter(t)
	other := models.User{Username: "image-other", Password: "password123!A", Email: "image-other@example.com"}
	require.NoError(t, env.db.Create(&other).Error)
	activity := models.Activity{UserID: other.ID, Title: "Theirs", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)

	rec := uploadActivityImage(t, env, activity.ID, testJPEG(t))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	req, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", nil)
	listRec := httptest.NewRecorder()
	env.router.ServeHTTP(listRec, req)
	assert.Equal(t, http.StatusNotFound, listRec.Code)
}

func TestActivityImages_RejectNonImage(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)

	rec := uploadActivityImage(t, env, activity.ID, []byte("<svg></svg>"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var count int64
	require.NoError(t, env.db.Model(&models.ActivityImage{}).Count(&count).Error)
	assert.Zero(t, count)
}

func testDate() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.Set(0, 0, color.RGBA{R: 1, A: 0})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestActivityImages_ThumbnailETagAndErrors(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)
	rec := uploadActivityImage(t, env, activity.ID, testPNG(t))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Image ActivityImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "image/png", created.Image.MIMEType)

	thumbReq, _ := http.NewRequest("GET", strings.TrimPrefix(created.Image.ThumbnailURL, "/api/v1"), nil)
	thumbRec := httptest.NewRecorder()
	env.router.ServeHTTP(thumbRec, thumbReq)
	require.Equal(t, http.StatusOK, thumbRec.Code)
	assert.Equal(t, "image/png", thumbRec.Header().Get("Content-Type"))
	etag := thumbRec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	thumbReq2, _ := http.NewRequest("GET", strings.TrimPrefix(created.Image.ThumbnailURL, "/api/v1"), nil)
	thumbReq2.Header.Set("If-None-Match", etag)
	thumbRec2 := httptest.NewRecorder()
	env.router.ServeHTTP(thumbRec2, thumbReq2)
	assert.Equal(t, http.StatusNotModified, thumbRec2.Code)

	// A foreign image id is 404, not 403.
	fileReq, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/999999/file", nil)
	fileRec := httptest.NewRecorder()
	env.router.ServeHTTP(fileRec, fileReq)
	assert.Equal(t, http.StatusNotFound, fileRec.Code)

	badReq, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/not-a-number/file", nil)
	badRec := httptest.NewRecorder()
	env.router.ServeHTTP(badRec, badReq)
	assert.Equal(t, http.StatusBadRequest, badRec.Code)

	badDel, _ := http.NewRequest("DELETE", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/not-a-number", nil)
	badDelRec := httptest.NewRecorder()
	env.router.ServeHTTP(badDelRec, badDel)
	assert.Equal(t, http.StatusBadRequest, badDelRec.Code)

	// Removing the file behind the row is a 404, not a 500.
	var row models.ActivityImage
	require.NoError(t, env.db.First(&row, created.Image.ID).Error)
	require.NoError(t, os.Remove(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName)))
	missingReq, _ := http.NewRequest("GET", strings.TrimPrefix(created.Image.FileURL, "/api/v1"), nil)
	missingRec := httptest.NewRecorder()
	env.router.ServeHTTP(missingRec, missingReq)
	assert.Equal(t, http.StatusNotFound, missingRec.Code)
}

func TestActivityImages_UploadValidationAndLimit(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)

	// Missing file part.
	req, _ := http.NewRequest("POST", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Invalid activity id.
	badReq, _ := http.NewRequest("GET", "/activities/not-a-number/images", nil)
	badRec := httptest.NewRecorder()
	env.router.ServeHTTP(badRec, badReq)
	assert.Equal(t, http.StatusBadRequest, badRec.Code)

	// Per-activity cap.
	cfgEnv := env
	for i := 0; i < 10; i++ {
		capRec := uploadActivityImage(t, cfgEnv, activity.ID, testJPEG(t))
		require.Equal(t, http.StatusCreated, capRec.Code, capRec.Body.String())
	}
	overRec := uploadActivityImage(t, cfgEnv, activity.ID, testJPEG(t))
	assert.Equal(t, http.StatusConflict, overRec.Code, overRec.Body.String())
}

func TestActivityImages_StorageFailures(t *testing.T) {
	t.Run("list query", func(t *testing.T) {
		env := setupImageRouter(t)
		activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&activity).Error)
		failDBTableOn(t, env.db, "activity_images", "query")
		req, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("upload create", func(t *testing.T) {
		env := setupImageRouter(t)
		activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&activity).Error)
		failDBTableOn(t, env.db, "activity_images", "create")
		rec := uploadActivityImage(t, env, activity.ID, testJPEG(t))
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("delete", func(t *testing.T) {
		env := setupImageRouter(t)
		activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&activity).Error)
		up := uploadActivityImage(t, env, activity.ID, testJPEG(t))
		require.Equal(t, http.StatusCreated, up.Code)
		var body struct {
			Image ActivityImageDTO `json:"image"`
		}
		require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
		failDBTableOn(t, env.db, "activity_images", "delete")
		req, _ := http.NewRequest("DELETE", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10), nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestActivityImages_OversizedUploadIsRejectedBeforeDecode(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "image-oversize", Password: "password123!A", Email: "image-oversize@example.com"}
	require.NoError(t, db.Create(&user).Error)
	activity := models.Activity{UserID: user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, db.Create(&activity).Error)
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), ImagesDir: t.TempDir(), ImageMaxUploadMB: 1, ImageMaxPixels: 50_000_000, ImageMaxPerActivity: 10}
	router := gin.Default()
	router.MaxMultipartMemory = 4 << 20
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/activities/:id/images", UploadActivityImage)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "big.jpg")
	require.NoError(t, err)
	_, err = part.Write(bytes.Repeat([]byte{0xFF}, (1<<20)+1))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req, _ := http.NewRequest("POST", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

func TestActivityImages_LoadAndFilePathFailures(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)

	failDBTableOn(t, env.db, "activities", "query")
	req, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestActivityImages_InvalidStoredNameIsNotFound(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)
	up := uploadActivityImage(t, env, activity.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, up.Code)
	var body struct {
		Image ActivityImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
	require.NoError(t, env.db.Model(&models.ActivityImage{}).Where("id = ?", body.Image.ID).
		Update("stored_name", "../escape.jpg").Error)

	req, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10)+"/file", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestActivityImages_DownloadDatabaseFailure(t *testing.T) {
	env := setupImageRouter(t)
	activity := models.Activity{UserID: env.user.ID, Title: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)
	up := uploadActivityImage(t, env, activity.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, up.Code)
	var body struct {
		Image ActivityImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
	failDBTableOn(t, env.db, "activity_images", "query")
	req, _ := http.NewRequest("GET", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10)+"/file", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestActivityImages_ForeignDownloadAndDeleteAre404(t *testing.T) {
	env := setupImageRouter(t)
	other := models.User{Username: "image-other-dl", Password: "password123!A", Email: "image-other-dl@example.com"}
	require.NoError(t, env.db.Create(&other).Error)
	activity := models.Activity{UserID: other.ID, Title: "Theirs", Date: testDate()}
	require.NoError(t, env.db.Create(&activity).Error)
	url := "/activities/" + strconv.FormatUint(uint64(activity.ID), 10) + "/images/1/file"
	req, _ := http.NewRequest("GET", url, nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	delReq, _ := http.NewRequest("DELETE", "/activities/"+strconv.FormatUint(uint64(activity.ID), 10)+"/images/1", nil)
	delRec := httptest.NewRecorder()
	env.router.ServeHTTP(delRec, delReq)
	assert.Equal(t, http.StatusNotFound, delRec.Code)
}

func TestImageUploadBodyLimit(t *testing.T) {
	cfg := config.Config{ImageMaxUploadMB: 20}
	assert.Equal(t, int64(21<<20), ImageUploadBodyLimit(&cfg))
}
