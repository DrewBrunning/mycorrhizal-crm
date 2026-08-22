package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listAttachments GETs a contact's attachments and decodes the raw response.
func listAttachments(t *testing.T, router *gin.Engine, contactID string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts/"+contactID+"/attachments", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestListContactAttachments_EmptyListReturnsArrayNotNil(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	rec := listAttachments(t, router, itoa2(contact.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// The raw JSON must carry an empty ARRAY, not null: the frontend's
	// AttachmentListResponse requires `attachments: Attachment[]` and reads
	// `.length` straight off it (CLAUDE.md trap #8 — a nil slice encodes as
	// null and crashes the caller).
	assert.JSONEq(t, `{"attachments":[],"total":0}`, rec.Body.String())
}

func TestListContactAttachments_ReturnsNewestFirst(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	first := uploadFile(t, router, itoa2(contact.ID), "a.txt", "text/plain", []byte("first"))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := uploadFile(t, router, itoa2(contact.ID), "b.txt", "text/plain", []byte("second"))
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())

	rec := listAttachments(t, router, itoa2(contact.ID))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Attachments []models.Attachment `json:"attachments"`
		Total       int                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Attachments, 2)
	assert.Equal(t, 2, body.Total)
	assert.Equal(t, "b.txt", body.Attachments[0].OriginalName, "attachments must be listed newest first")
	assert.Equal(t, "a.txt", body.Attachments[1].OriginalName)
}

func TestListContactAttachments_ScopedToContactAndUser(t *testing.T) {
	db, router, user, _ := setupAttachmentRouter(t)

	// A contact belonging to another user must 404 for this caller (the
	// contact lookup itself is user-scoped).
	other := models.User{Username: "attachments-other-list", Password: "password123!A", Email: "attachments-other-list@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&otherContact).Error)
	otherUpload := uploadFile(t, router, itoa2(otherContact.ID), "theirs.txt", "text/plain", []byte("theirs"))
	assert.Equal(t, http.StatusNotFound, otherUpload.Code, "uploading to another user's contact must 404")

	// A non-existent contact id also 404s.
	rec := listAttachments(t, router, "999999")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	// An existing own contact with an attachment lists it.
	mine := models.Contact{UserID: user.ID, Firstname: "Mine"}
	require.NoError(t, db.Create(&mine).Error)
	up := uploadFile(t, router, itoa2(mine.ID), "mine.txt", "text/plain", []byte("mine"))
	require.Equal(t, http.StatusCreated, up.Code, up.Body.String())

	// The same attachment must NOT be listable by the other user: the contact
	// lookup itself is scoped by user_id, so it 404s rather than exposing the
	// first user's contact at all.
	gin.SetMode(gin.ReleaseMode)
	otherRouter := gin.Default()
	otherRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", other.ID)
		c.Next()
	})
	otherRouter.GET("/contacts/:id/attachments", ListContactAttachments)
	recOther := listAttachments(t, otherRouter, itoa2(mine.ID))
	assert.Equal(t, http.StatusNotFound, recOther.Code, "another user must not be able to list a contact's attachments")
}

func TestListContactAttachments_InvalidContactID(t *testing.T) {
	_, router, _, _ := setupAttachmentRouter(t)

	rec := listAttachments(t, router, "not-a-number")
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}
