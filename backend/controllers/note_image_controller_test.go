package controllers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type noteImageTestEnv struct {
	db     *gorm.DB
	router *gin.Engine
	user   models.User
	dir    string
}

func setupNoteImageRouter(t *testing.T) noteImageTestEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "image-owner", Password: "password123!A", Email: "image-owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := filepath.Join(t.TempDir(), "images")
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), ImagesDir: dir, ImageMaxUploadMB: 20, ImageMaxPixels: 50_000_000, ImageMaxPerNote: 10}
	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/notes/:id/images", UploadNoteImage)
	router.GET("/notes/:id/images", ListNoteImages)
	router.GET("/notes/:id/images/:image_id/file", func(c *gin.Context) { DownloadNoteImage(c, false) })
	router.GET("/notes/:id/images/:image_id/thumbnail", func(c *gin.Context) { DownloadNoteImage(c, true) })
	router.DELETE("/notes/:id/images/:image_id", DeleteNoteImage)
	return noteImageTestEnv{db: db, router: router, user: user, dir: dir}
}

func uploadNoteImageForTest(t *testing.T, env noteImageTestEnv, noteID uint, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "photo.jpg")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req, _ := http.NewRequest("POST", "/notes/"+strconv.FormatUint(uint64(noteID), 10)+"/images", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func TestNoteImages_UploadListDownloadDelete(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	data := testJPEG(t)

	rec := uploadNoteImageForTest(t, env, note.ID, data)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Image NoteImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, note.ID, created.Image.NoteID)
	assert.Equal(t, "image/jpeg", created.Image.MIMEType)
	assert.NotEmpty(t, created.Image.FileURL)

	var row models.NoteImage
	require.NoError(t, env.db.First(&row, created.Image.ID).Error)
	_, err := os.Stat(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName))
	require.NoError(t, err, "stored image must exist on disk")

	list, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", nil)
	listRec := httptest.NewRecorder()
	env.router.ServeHTTP(listRec, list)
	require.Equal(t, http.StatusOK, listRec.Code)
	var body struct {
		Images []NoteImageDTO `json:"images"`
		Total  int            `json:"total"`
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

	delReq, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(row.ID), 10), nil)
	delRec := httptest.NewRecorder()
	env.router.ServeHTTP(delRec, delReq)
	require.Equal(t, http.StatusOK, delRec.Code, delRec.Body.String())
	_, err = os.Stat(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName))
	assert.True(t, os.IsNotExist(err), "deleted image file must be removed")
	var count int64
	require.NoError(t, env.db.Model(&models.NoteImage{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestNoteImages_ForeignNoteIs404(t *testing.T) {
	env := setupNoteImageRouter(t)
	other := models.User{Username: "image-other", Password: "password123!A", Email: "image-other@example.com"}
	require.NoError(t, env.db.Create(&other).Error)
	note := models.Note{UserID: other.ID, Content: "Theirs", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)

	rec := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	req, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", nil)
	listRec := httptest.NewRecorder()
	env.router.ServeHTTP(listRec, req)
	assert.Equal(t, http.StatusNotFound, listRec.Code)
}

func TestNoteImages_RejectNonImage(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)

	rec := uploadNoteImageForTest(t, env, note.ID, []byte("<svg></svg>"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var count int64
	require.NoError(t, env.db.Model(&models.NoteImage{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestNoteImages_ThumbnailETagAndErrors(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	rec := uploadNoteImageForTest(t, env, note.ID, testPNG(t))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Image NoteImageDTO `json:"image"`
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
	fileReq, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/999999/file", nil)
	fileRec := httptest.NewRecorder()
	env.router.ServeHTTP(fileRec, fileReq)
	assert.Equal(t, http.StatusNotFound, fileRec.Code)

	badReq, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/not-a-number/file", nil)
	badRec := httptest.NewRecorder()
	env.router.ServeHTTP(badRec, badReq)
	assert.Equal(t, http.StatusBadRequest, badRec.Code)

	badDel, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/not-a-number", nil)
	badDelRec := httptest.NewRecorder()
	env.router.ServeHTTP(badDelRec, badDel)
	assert.Equal(t, http.StatusBadRequest, badDelRec.Code)

	// Removing the file behind the row is a 404, not a 500.
	var row models.NoteImage
	require.NoError(t, env.db.First(&row, created.Image.ID).Error)
	require.NoError(t, os.Remove(filepath.Join(env.dir, "u"+strconv.FormatUint(uint64(env.user.ID), 10), row.StoredName)))
	missingReq, _ := http.NewRequest("GET", strings.TrimPrefix(created.Image.FileURL, "/api/v1"), nil)
	missingRec := httptest.NewRecorder()
	env.router.ServeHTTP(missingRec, missingReq)
	assert.Equal(t, http.StatusNotFound, missingRec.Code)
}

func TestNoteImages_UploadValidationAndLimit(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)

	// Missing file part.
	req, _ := http.NewRequest("POST", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", nil)
	req.Header.Set("Content-Type", "multipart/form-data")
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Invalid note id.
	badReq, _ := http.NewRequest("GET", "/notes/not-a-number/images", nil)
	badRec := httptest.NewRecorder()
	env.router.ServeHTTP(badRec, badReq)
	assert.Equal(t, http.StatusBadRequest, badRec.Code)

	// Per-note cap.
	cfgEnv := env
	for i := 0; i < 10; i++ {
		capRec := uploadNoteImageForTest(t, cfgEnv, note.ID, testJPEG(t))
		require.Equal(t, http.StatusCreated, capRec.Code, capRec.Body.String())
	}
	overRec := uploadNoteImageForTest(t, cfgEnv, note.ID, testJPEG(t))
	assert.Equal(t, http.StatusConflict, overRec.Code, overRec.Body.String())
}

func TestNoteImages_StorageFailures(t *testing.T) {
	t.Run("list query", func(t *testing.T) {
		env := setupNoteImageRouter(t)
		note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&note).Error)
		failDBTableOn(t, env.db, "note_images", "query")
		req, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("upload create", func(t *testing.T) {
		env := setupNoteImageRouter(t)
		note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&note).Error)
		failDBTableOn(t, env.db, "note_images", "create")
		rec := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("delete", func(t *testing.T) {
		env := setupNoteImageRouter(t)
		note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
		require.NoError(t, env.db.Create(&note).Error)
		up := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
		require.Equal(t, http.StatusCreated, up.Code)
		var body struct {
			Image NoteImageDTO `json:"image"`
		}
		require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
		failDBTableOn(t, env.db, "note_images", "delete")
		req, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10), nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestNoteImages_OversizedUploadIsRejectedBeforeDecode(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "image-oversize", Password: "password123!A", Email: "image-oversize@example.com"}
	require.NoError(t, db.Create(&user).Error)
	note := models.Note{UserID: user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, db.Create(&note).Error)
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), ImagesDir: t.TempDir(), ImageMaxUploadMB: 1, ImageMaxPixels: 50_000_000, ImageMaxPerNote: 10}
	router := gin.Default()
	router.MaxMultipartMemory = 4 << 20
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/notes/:id/images", UploadNoteImage)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "big.jpg")
	require.NoError(t, err)
	_, err = part.Write(bytes.Repeat([]byte{0xFF}, (1<<20)+1))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req, _ := http.NewRequest("POST", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

func TestNoteImages_LoadAndFilePathFailures(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)

	failDBTableOn(t, env.db, "notes", "query")
	req, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestNoteImages_InvalidStoredNameIsNotFound(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	up := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, up.Code)
	var body struct {
		Image NoteImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
	require.NoError(t, env.db.Model(&models.NoteImage{}).Where("id = ?", body.Image.ID).
		Update("stored_name", "../escape.jpg").Error)

	req, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10)+"/file", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNoteImages_DownloadDatabaseFailure(t *testing.T) {
	env := setupNoteImageRouter(t)
	note := models.Note{UserID: env.user.ID, Content: "Trip", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	up := uploadNoteImageForTest(t, env, note.ID, testJPEG(t))
	require.Equal(t, http.StatusCreated, up.Code)
	var body struct {
		Image NoteImageDTO `json:"image"`
	}
	require.NoError(t, json.Unmarshal(up.Body.Bytes(), &body))
	failDBTableOn(t, env.db, "note_images", "query")
	req, _ := http.NewRequest("GET", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/"+strconv.FormatUint(uint64(body.Image.ID), 10)+"/file", nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestNoteImages_ForeignDownloadAndDeleteAre404(t *testing.T) {
	env := setupNoteImageRouter(t)
	other := models.User{Username: "image-other-dl", Password: "password123!A", Email: "image-other-dl@example.com"}
	require.NoError(t, env.db.Create(&other).Error)
	note := models.Note{UserID: other.ID, Content: "Theirs", Date: testDate()}
	require.NoError(t, env.db.Create(&note).Error)
	url := "/notes/" + strconv.FormatUint(uint64(note.ID), 10) + "/images/1/file"
	req, _ := http.NewRequest("GET", url, nil)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	delReq, _ := http.NewRequest("DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10)+"/images/1", nil)
	delRec := httptest.NewRecorder()
	env.router.ServeHTTP(delRec, delReq)
	assert.Equal(t, http.StatusNotFound, delRec.Code)
}

func TestNoteNoteImageUploadBodyLimit(t *testing.T) {
	cfg := config.Config{ImageMaxUploadMB: 20}
	assert.Equal(t, int64(21<<20), NoteImageUploadBodyLimit(&cfg))
}

func TestNoteImageUploadBodyLimit(t *testing.T) {
	cfg := config.Config{ImageMaxUploadMB: 20}
	assert.Equal(t, int64(21<<20), NoteImageUploadBodyLimit(&cfg))
}
