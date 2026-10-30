package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Contact trash routes (issue #2002, ADR 0037) against the real migrated schema.

func trashRouter(t *testing.T, retentionDays int) (*gorm.DB, *gin.Engine, uint) {
	t.Helper()
	db, router := setupRouterWithRetention(t, retentionDays)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.DELETE("/contacts/:id", DeleteContact)
	router.GET("/trash/contacts", ListTrashedContacts)
	router.DELETE("/trash/contacts", EmptyTrash)
	router.POST("/trash/contacts/:id/restore", RestoreTrashedContact)
	router.DELETE("/trash/contacts/:id", PurgeTrashedContact)
	return db, router, user.ID
}

func trashDo(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func trashContact(t *testing.T, db *gorm.DB, router *gin.Engine, userID uint, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: name}
	require.NoError(t, db.Create(&c).Error)
	w := trashDo(router, http.MethodDelete, fmt.Sprintf("/contacts/%d", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return c
}

func TestTrash_DeleteListRestore(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	c := trashContact(t, db, router, uid, "Ada")

	w := trashDo(router, http.MethodGet, "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code)
	var list models.TrashListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Contacts, 1)
	assert.Equal(t, c.ID, list.Contacts[0].Contact.ID)
	assert.Equal(t, 30, list.RetentionDays)
	require.NotNil(t, list.Contacts[0].PurgeAt)
	assert.True(t, list.Contacts[0].PurgeAt.After(list.Contacts[0].DeletedAt))

	w = trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var restored models.TrashRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &restored))
	assert.Equal(t, c.ID, restored.Contact.ID)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["skipped"]), "collection field is [] not absent (frontend trap #8)")

	var n int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", c.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "contact is live again")
	assert.EqualValues(t, 1, auditCount(t, db, c.VCardUID, models.AuditOpRestore))

	w = trashDo(router, http.MethodGet, "/trash/contacts")
	var empty map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &empty))
	assert.JSONEq(t, `[]`, string(empty["contacts"]))
}

func auditCount(t *testing.T, db *gorm.DB, uid, op string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.AuditEvent{}).Where("entity_type = ? AND entity_id = ? AND operation = ?", models.AuditEntityContact, uid, op).Count(&n).Error)
	return n
}

func TestTrash_RetentionZeroMeansKeepForever(t *testing.T) {
	db, router, uid := trashRouter(t, 0)
	trashContact(t, db, router, uid, "Keep")
	var list models.TrashListResponse
	require.NoError(t, json.Unmarshal(trashDo(router, http.MethodGet, "/trash/contacts").Body.Bytes(), &list))
	require.Len(t, list.Contacts, 1)
	assert.Nil(t, list.Contacts[0].PurgeAt)
}

func TestTrash_CrossUserIs404WithNoSideEffect(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&theirs).Error)
	require.NoError(t, db.Delete(&theirs).Error)
	_ = uid

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", theirs.ID)},
		{http.MethodDelete, fmt.Sprintf("/trash/contacts/%d", theirs.ID)},
	} {
		w := trashDo(router, tc.method, tc.path)
		assert.Equal(t, http.StatusNotFound, w.Code, tc.method+" "+tc.path)
	}
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ? AND deleted_at IS NOT NULL", theirs.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)

	// Empty-trash as the caller never touches another user's trash.
	other2 := trashContactFor(t, db, other.ID)
	w := trashDo(router, http.MethodDelete, "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", other2.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n)
}

func trashContactFor(t *testing.T, db *gorm.DB, userID uint) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: "Snap"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, services.DeleteContact(db, c, userID, time.Now()))
	return c
}

func TestTrash_RestoreUIDConflictIs409(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	c := trashContact(t, db, router, uid, "Dup")
	live := models.Contact{UserID: uid, VCardUID: c.VCardUID, Firstname: "Reborn"}
	require.NoError(t, db.Create(&live).Error)

	w := trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ? AND deleted_at IS NOT NULL", c.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "still trashed")
}

func TestTrash_PurgeAndEmpty(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	a := trashContact(t, db, router, uid, "A")
	trashContact(t, db, router, uid, "B")
	trashContact(t, db, router, uid, "C")

	w := trashDo(router, http.MethodDelete, fmt.Sprintf("/trash/contacts/%d", a.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, auditCount(t, db, a.VCardUID, models.AuditOpPurge))
	assert.Equal(t, http.StatusNotFound, trashDo(router, http.MethodDelete, fmt.Sprintf("/trash/contacts/%d", a.ID)).Code)

	w = trashDo(router, http.MethodDelete, "/trash/contacts")
	require.Equal(t, http.StatusOK, w.Code)
	var res models.TrashPurgeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 2, res.Purged)
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("user_id = ?", uid).Count(&n).Error)
	assert.Zero(t, n)
}

func TestTrash_RestoreFiresContactRestoredWebhookOnce(t *testing.T) {
	db, router, uid := trashRouter(t, 30)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, db.Create(&models.Webhook{
		UserID: uid, Name: "trash", URL: srv.URL,
		Events: []string{"contact.restored"}, Secret: "s", IsActive: true,
	}).Error)
	c := trashContact(t, db, router, uid, "Webhook")

	w := trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	services.WaitForWebhookGoroutines()
	assert.EqualValues(t, 1, atomic.LoadInt32(&hits), "restore must fire contact.restored once")

	// The snapshot is consumed, so the repeat is a checked 404 with no second event.
	w = trashDo(router, http.MethodPost, fmt.Sprintf("/trash/contacts/%d/restore", c.ID))
	require.Equal(t, http.StatusNotFound, w.Code)
	services.WaitForWebhookGoroutines()
	assert.EqualValues(t, 1, atomic.LoadInt32(&hits), "a repeat restore must not fire the webhook again")
}

func TestTrash_BadIDIs404(t *testing.T) {
	_, router, _ := trashRouter(t, 30)
	assert.Equal(t, http.StatusNotFound, trashDo(router, http.MethodPost, "/trash/contacts/999999/restore").Code)
	assert.Equal(t, http.StatusBadRequest, trashDo(router, http.MethodPost, "/trash/contacts/abc/restore").Code)
}
