package controllers

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupBareAttachmentRouter is setupAttachmentRouter without the default body
// limit middleware, so the handler's own 25MB cap (maxAttachmentSize) is the
// only gate — matching the handler's defensive contract.
func setupBareAttachmentRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "att-bare", Password: "password123!A", Email: "att-bare@example.com"}
	require.NoError(t, db.Create(&user).Error)

	attachmentsDir := filepath.Join(t.TempDir(), "attachments")
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), AttachmentsDir: attachmentsDir}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.MaxMultipartMemory = 64 << 20
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &cfg) })
	return db, router, user
}

func multipartBody(t *testing.T, filename string, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

// userScopedRouter builds a minimal gin router whose middleware sets the given
// keys, so a test can pass userID=0 to exercise the 401 branches.
func userScopedRouter(db *gorm.DB, userID uint, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Set("cfg", cfg)
		c.Next()
	})
	return router
}

func TestUploadAttachment_InvalidContactID(t *testing.T) {
	_, router, _ := setupBareAttachmentRouter(t)
	rec := uploadFile(t, router, "not-a-number", "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestUploadAttachment_Unauthenticated(t *testing.T) {
	db, _, user := setupBareAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	router := userScopedRouter(db, 0, config.Config{})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &config.Config{}) })

	body, ct := multipartBody(t, "a.txt", []byte("x"))
	req, _ := http.NewRequest(http.MethodPost, "/contacts/"+strconv.Itoa(int(contact.ID))+"/attachments", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestUploadAttachment_ContactNotFound(t *testing.T) {
	_, router, _ := setupBareAttachmentRouter(t)
	rec := uploadFile(t, router, "999999", "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestUploadAttachment_NoFile(t *testing.T) {
	db, _, user := setupBareAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	// A multipart form that carries no "file" part reaches the
	// c.FormFile("file") miss branch.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("notfile", "value"))
	require.NoError(t, w.Close())

	router := userScopedRouter(db, user.ID, config.Config{})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &config.Config{}) })

	req, _ := http.NewRequest(http.MethodPost, "/contacts/"+strconv.Itoa(int(contact.ID))+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestUploadAttachment_FileTooLarge(t *testing.T) {
	db, router, user := setupBareAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	// Over 25MB of zeros trips the handler's own cap (bare router, so the
	// default 10MB middleware is not in the way).
	big := make([]byte, maxAttachmentSize+1)
	body, ct := multipartBody(t, "big.bin", big)
	req, _ := http.NewRequest(http.MethodPost, "/contacts/"+strconv.Itoa(int(contact.ID))+"/attachments", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func TestUploadAttachment_StorageFailure(t *testing.T) {
	db, _, user := setupBareAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	// A file sitting at the attachments-dir path makes MkdirAll fail, so
	// attachments.Save surfaces an error.
	blocked := filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	router := userScopedRouter(db, user.ID, config.Config{AttachmentsDir: blocked})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &config.Config{AttachmentsDir: blocked}) })

	rec := uploadFile(t, router, strconv.Itoa(int(contact.ID)), "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}

func TestUploadAttachment_DatabaseFailure(t *testing.T) {
	db, _, user := setupBareAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	dir := t.TempDir()
	router := userScopedRouter(db, user.ID, config.Config{AttachmentsDir: dir})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &config.Config{AttachmentsDir: dir}) })

	// Close the DB so the attachment insert fails after the file is written.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	rec := uploadFile(t, router, strconv.Itoa(int(contact.ID)), "a.txt", "text/plain", []byte("x"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}

func TestListContactAttachments_NotFound(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "att-listnf", Password: "password123!A", Email: "att-listnf@example.com"}
	require.NoError(t, db.Create(&user).Error)

	router := userScopedRouter(db, user.ID, config.Config{})
	router.GET("/contacts/:id/attachments", ListContactAttachments)

	req, _ := http.NewRequest(http.MethodGet, "/contacts/999999/attachments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestListContactAttachments_DatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "att-listerr", Password: "password123!A", Email: "att-listerr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	router := userScopedRouter(db, user.ID, config.Config{})
	router.GET("/contacts/:id/attachments", ListContactAttachments)

	req, _ := http.NewRequest(http.MethodGet, "/contacts/"+strconv.Itoa(int(contact.ID))+"/attachments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestDownloadAttachment_StoragePathError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "att-path", Password: "password123!A", Email: "att-path@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	// A corrupt stored name (traversal) passes the ownership lookup but must
	// be rejected by StoredPath before it reaches the filesystem.
	require.NoError(t, db.Create(&models.Attachment{
		UserID: user.ID, ContactVCardUID: contact.VCardUID, StoredName: "../escape.txt",
		OriginalName: "x.txt", ContentType: "text/plain", SizeBytes: 1,
	}).Error)

	dir := t.TempDir()
	router := userScopedRouter(db, user.ID, config.Config{AttachmentsDir: dir})
	router.GET("/attachments/:id/download", func(c *gin.Context) { DownloadAttachment(c, &config.Config{AttachmentsDir: dir}) })

	req, _ := http.NewRequest(http.MethodGet, "/attachments/1/download", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestDownloadAttachment_MissingFileOnDisk(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	// A row whose file never landed on disk (or was cleaned up manually).
	require.NoError(t, db.Create(&models.Attachment{
		UserID: user.ID, ContactVCardUID: contact.VCardUID, StoredName: "00000000-0000-0000-0000-000000000000",
		OriginalName: "x.txt", ContentType: "text/plain", SizeBytes: 1,
	}).Error)

	var att models.Attachment
	require.NoError(t, db.First(&att).Error)

	req, _ := http.NewRequest(http.MethodGet, "/attachments/"+strconv.Itoa(int(att.ID))+"/download", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestDeleteAttachment_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	router := userScopedRouter(db, 0, config.Config{})
	router.DELETE("/attachments/:id", func(c *gin.Context) { DeleteAttachment(c, &config.Config{}) })

	req, _ := http.NewRequest(http.MethodDelete, "/attachments/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestDeleteAttachment_DatabaseError(t *testing.T) {
	db, _, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Attachment{
		UserID: user.ID, ContactVCardUID: contact.VCardUID, StoredName: "uuid",
		OriginalName: "x.txt", ContentType: "text/plain", SizeBytes: 1,
	}).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	dir := t.TempDir()
	router := userScopedRouter(db, user.ID, config.Config{AttachmentsDir: dir})
	router.DELETE("/attachments/:id", func(c *gin.Context) { DeleteAttachment(c, &config.Config{AttachmentsDir: dir}) })

	req, _ := http.NewRequest(http.MethodDelete, "/attachments/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// TestDeleteAttachment_FileRemovalFailureIsLoggedNotFatal covers the
// post-commit file-removal branch: the record is soft-deleted successfully,
// but the on-disk file cannot be removed (here: a corrupt traversal stored
// name that Remove rejects). The delete must still succeed — a failed file
// cleanup is logged, never a 500.
func TestDeleteAttachment_FileRemovalFailureIsLoggedNotFatal(t *testing.T) {
	db, router, user, dir := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Attachment{
		UserID: user.ID, ContactVCardUID: contact.VCardUID, StoredName: "../escape.txt",
		OriginalName: "x.txt", ContentType: "text/plain", SizeBytes: 1,
	}).Error)

	var att models.Attachment
	require.NoError(t, db.First(&att).Error)

	req, _ := http.NewRequest(http.MethodDelete, "/attachments/"+strconv.Itoa(int(att.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The record is gone (soft-deleted) regardless of the file failure.
	var count int64
	require.NoError(t, db.Model(&models.Attachment{}).Where("id = ?", att.ID).Count(&count).Error)
	assert.Zero(t, count)
	// The corrupt stored name must not have escaped the attachments dir.
	_, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt"))
	assert.True(t, os.IsNotExist(err), "a traversal stored name must never reach the filesystem")
}
