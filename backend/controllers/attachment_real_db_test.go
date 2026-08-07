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
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAttachmentRouter builds a real-schema router over a temp attachments
// directory.
func setupAttachmentRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User, string) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "attachments-test.db"))
	require.NoError(t, err)
	user := models.User{Username: "attachments", Password: "password123!A", Email: "attachments@example.com"}
	require.NoError(t, db.Create(&user).Error)

	attachmentsDir := filepath.Join(t.TempDir(), "attachments")
	cfg := config.Config{ProfilePhotoDir: t.TempDir(), AttachmentsDir: attachmentsDir}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &cfg) })
	router.GET("/contacts/:id/attachments", ListContactAttachments)
	router.GET("/attachments/:id/download", func(c *gin.Context) { DownloadAttachment(c, &cfg) })
	router.DELETE("/attachments/:id", func(c *gin.Context) { DeleteAttachment(c, &cfg) })
	router.DELETE("/contacts/:id", DeleteContact)
	return db, router, user, attachmentsDir
}

// uploadFile posts a multipart attachment and returns the recorder + attachment.
func uploadFile(t *testing.T, router *gin.Engine, contactID string, filename string, contentType string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, _ := http.NewRequest("POST", "/contacts/"+contactID+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeAttachment(t *testing.T, rec *httptest.ResponseRecorder) models.Attachment {
	t.Helper()
	var body struct {
		Attachment models.Attachment `json:"attachment"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Attachment
}

// reloadAttachment fetches the full row from the DB — StoredName is
// deliberately json:"-" on the wire, so tests read it from the database.
func reloadAttachment(t *testing.T, db *gorm.DB, id uint) models.Attachment {
	t.Helper()
	var att models.Attachment
	require.NoError(t, db.First(&att, id).Error)
	return att
}

func TestAttachmentRoundTrip(t *testing.T) {
	db, router, user, dir := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	rec := uploadFile(t, router, itoa2(contact.ID), "cv.pdf", "application/pdf", []byte("%PDF-1.4 fake pdf"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	att := decodeAttachment(t, rec)
	att = reloadAttachment(t, db, att.ID)
	assert.Equal(t, "cv.pdf", att.OriginalName)
	assert.Equal(t, "application/pdf", att.ContentType)
	assert.Equal(t, int64(len("%PDF-1.4 fake pdf")), att.SizeBytes)

	// The file exists on disk under the stored UUID name.
	_, err := os.Stat(filepath.Join(dir, att.StoredName))
	require.NoError(t, err)

	// Download returns the bytes.
	dl := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/attachments/"+itoa2(att.ID)+"/download", nil)
	router.ServeHTTP(dl, req)
	require.Equal(t, http.StatusOK, dl.Code)
	assert.Equal(t, "%PDF-1.4 fake pdf", dl.Body.String())
	assert.Contains(t, dl.Header().Get("Content-Disposition"), "inline", "pdf is on the inline allow-list")
	assert.Equal(t, "nosniff", dl.Header().Get("X-Content-Type-Options"))

	// Delete removes the record and the file.
	del := httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/attachments/"+itoa2(att.ID), nil)
	router.ServeHTTP(del, req)
	require.Equal(t, http.StatusOK, del.Code)
	_, err = os.Stat(filepath.Join(dir, att.StoredName))
	assert.True(t, os.IsNotExist(err), "deleting an attachment must remove its file")
}

func TestAttachmentCrossUserDenied(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	other := models.User{Username: "attachments-other", Password: "password123!A", Email: "attachments-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&otherContact).Error)

	// The calling user cannot upload to another user's contact.
	rec := uploadFile(t, router, itoa2(otherContact.ID), "x.txt", "text/plain", []byte("x"))
	assert.Equal(t, http.StatusNotFound, rec.Code, "uploading to another user's contact must 404")

	// Upload to our own contact, then confirm the other user's ID is not
	// reachable via the API (the router is bound to the first user).
	mine := models.Contact{UserID: user.ID, Firstname: "Mine"}
	require.NoError(t, db.Create(&mine).Error)
	rec = uploadFile(t, router, itoa2(mine.ID), "x.txt", "text/plain", []byte("x"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	att := decodeAttachment(t, rec)
	// A second router bound to the other user must not see it (loadOwnedAttachment
	// scopes by user_id).
	gin.SetMode(gin.ReleaseMode)
	otherCfg := config.Config{AttachmentsDir: filepath.Join(t.TempDir(), "other-attachments")}
	router2 := gin.Default()
	router2.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", other.ID)
		c.Set("cfg", otherCfg)
		c.Next()
	})
	router2.GET("/attachments/:id/download", func(c *gin.Context) { DownloadAttachment(c, &otherCfg) })
	dl := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/attachments/"+itoa2(att.ID)+"/download", nil)
	router2.ServeHTTP(dl, req)
	assert.Equal(t, http.StatusNotFound, dl.Code, "another user's attachment must not be downloadable")
}

func TestAttachmentTraversalFilenameNeutralized(t *testing.T) {
	db, router, user, dir := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	rec := uploadFile(t, router, itoa2(contact.ID), "../../../etc/passwd", "text/plain", []byte("root:x:0:0"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	att := reloadAttachment(t, db, decodeAttachment(t, rec).ID)

	// The stored name is a UUID — the traversal never reached the filesystem.
	assert.Equal(t, filepath.Base(att.StoredName), att.StoredName)
	_, err := os.Stat(filepath.Join(dir, att.StoredName))
	require.NoError(t, err)

	// Download still works; the display name is sanitized into a single segment.
	dl := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/attachments/"+itoa2(att.ID)+"/download", nil)
	router.ServeHTTP(dl, req)
	require.Equal(t, http.StatusOK, dl.Code)
	assert.Contains(t, dl.Header().Get("Content-Disposition"), "attachment", "text/plain is not inline")
	assert.NotContains(t, dl.Header().Get("Content-Disposition"), "..", "the display name must not carry the traversal")
}

func TestAttachmentRejectsSVGAndHTML(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	rec := uploadFile(t, router, itoa2(contact.ID), "evil.svg", "image/svg+xml", svg)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "SVG upload must be rejected")

	html := []byte(`<html><script>alert(1)</script></html>`)
	rec = uploadFile(t, router, itoa2(contact.ID), "evil.html", "text/html", html)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "HTML upload must be rejected")

	var count int64
	require.NoError(t, db.Model(&models.Attachment{}).Count(&count).Error)
	assert.Zero(t, count, "rejected uploads must not leave records")
}

func TestDeleteContactRemovesAttachmentFiles(t *testing.T) {
	db, router, user, dir := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	rec := uploadFile(t, router, itoa2(contact.ID), "doc.pdf", "application/pdf", []byte("%PDF"))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	att := reloadAttachment(t, db, decodeAttachment(t, rec).ID)
	filePath := filepath.Join(dir, att.StoredName)
	_, err := os.Stat(filePath)
	require.NoError(t, err)

	// Deleting the contact removes the attachment file from disk.
	del := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/contacts/"+itoa2(contact.ID), nil)
	router.ServeHTTP(del, req)
	require.Equal(t, http.StatusOK, del.Code)
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err), "deleting a contact must remove its attachment files")
}

func itoa2(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
