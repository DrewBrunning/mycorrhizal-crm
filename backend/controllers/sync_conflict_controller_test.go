package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupSyncConflictRouter builds a real-schema router (CLAUDE.md backend trap
// 1) with the list/restore/dismiss endpoints for one authenticated user.
func setupSyncConflictRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db := dbtest.New(t)

	user := models.User{Username: "sccuser", Password: "password123!A", Email: "sccuser@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "sccother", Password: "password123!A", Email: "sccother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/contact-sync-conflicts", ListContactSyncConflicts)
	router.POST("/contact-sync-conflicts/:id/restore", RestoreContactSyncConflict)
	router.POST("/contact-sync-conflicts/:id/dismiss", DismissContactSyncConflict)
	return db, router, user
}

func seedControllerConflict(t *testing.T, db *gorm.DB, userID uint, contact models.Contact, sub models.ContactSubscription, field, local, remote string) models.ContactSyncConflict {
	t.Helper()
	conflict := models.ContactSyncConflict{
		UserID: userID, SubscriptionID: sub.ID, ContactID: contact.ID,
		Field: field, LocalValue: local, RemoteValue: remote, Status: models.SyncConflictStatusPending,
	}
	require.NoError(t, db.Create(&conflict).Error)
	return conflict
}

func TestListContactSyncConflicts_ReturnsPendingEnriched(t *testing.T) {
	db, router, user := setupSyncConflictRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Grace", Lastname: "Hopper"}
	require.NoError(t, db.Create(&contact).Error)
	sub := models.ContactSubscription{UserID: user.ID, Name: "Work address book", URL: "https://example.com/dav/"}
	require.NoError(t, db.Create(&sub).Error)

	pending := seedControllerConflict(t, db, user.ID, contact, sub, models.SyncConflictFieldJobTitle, "Local", "Remote")
	dismissed := seedControllerConflict(t, db, user.ID, contact, sub, models.SyncConflictFieldPhone, "X", "Y")
	require.NoError(t, db.Model(&dismissed).Update("status", models.SyncConflictStatusDismissed).Error)

	req, _ := http.NewRequest("GET", "/contact-sync-conflicts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		SyncConflicts []models.ContactSyncConflictResponse `json:"sync_conflicts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.SyncConflicts, 1)
	assert.Equal(t, pending.ID, resp.SyncConflicts[0].ID)
	assert.Equal(t, contact.ID, resp.SyncConflicts[0].ContactID)
	assert.Equal(t, "Grace Hopper", resp.SyncConflicts[0].ContactName)
	assert.Equal(t, "Work address book", resp.SyncConflicts[0].SubscriptionName)
}

func TestRestoreContactSyncConflict_RestoresAndDismisses(t *testing.T) {
	db, router, user := setupSyncConflictRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Grace", Lastname: "Hopper"}
	require.NoError(t, db.Create(&contact).Error)
	sub := models.ContactSubscription{UserID: user.ID, Name: "Work", URL: "https://example.com/dav/"}
	require.NoError(t, db.Create(&sub).Error)

	conflict := seedControllerConflict(t, db, user.ID, contact, sub, models.SyncConflictFieldNickname, "Amazing Grace", "Grace")

	req, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+conflict.ID+"/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Equal(t, "Amazing Grace", reloaded.Nickname, "the local value must be written back onto the contact")

	var conflictRow models.ContactSyncConflict
	require.NoError(t, db.First(&conflictRow, "id = ?", conflict.ID).Error)
	assert.Equal(t, models.SyncConflictStatusDismissed, conflictRow.Status)

	// Restoring again is now a 409.
	req2, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+conflict.ID+"/restore", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusConflict, w2.Code)
}

func TestRestoreContactSyncConflict_ScopedToOwner(t *testing.T) {
	db, router, _ := setupSyncConflictRouter(t)

	var other models.User
	require.NoError(t, db.Where("username = ?", "sccother").First(&other).Error)
	theirContact := models.Contact{UserID: other.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&theirContact).Error)
	theirSub := models.ContactSubscription{UserID: other.ID, Name: "Their", URL: "https://example.com/dav/"}
	require.NoError(t, db.Create(&theirSub).Error)
	foreign := seedControllerConflict(t, db, other.ID, theirContact, theirSub, models.SyncConflictFieldPhone, "A", "B")

	req, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+foreign.ID+"/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, theirContact.ID).Error)
	assert.Empty(t, reloaded.Phone, "another user's conflict must not be restorable")

	var conflictRow models.ContactSyncConflict
	require.NoError(t, db.First(&conflictRow, "id = ?", foreign.ID).Error)
	assert.Equal(t, models.SyncConflictStatusPending, conflictRow.Status)
}

func TestDismissContactSyncConflict_IsIdempotent(t *testing.T) {
	db, router, user := setupSyncConflictRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Grace"}
	require.NoError(t, db.Create(&contact).Error)
	sub := models.ContactSubscription{UserID: user.ID, Name: "Work", URL: "https://example.com/dav/"}
	require.NoError(t, db.Create(&sub).Error)

	conflict := seedControllerConflict(t, db, user.ID, contact, sub, models.SyncConflictFieldPhone, "A", "B")

	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+conflict.ID+"/dismiss", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	var conflictRow models.ContactSyncConflict
	require.NoError(t, db.First(&conflictRow, "id = ?", conflict.ID).Error)
	assert.Equal(t, models.SyncConflictStatusDismissed, conflictRow.Status)
}

func TestDismissContactSyncConflict_UnknownIDIs404(t *testing.T) {
	_, router, _ := setupSyncConflictRouter(t)

	req, _ := http.NewRequest("POST", "/contact-sync-conflicts/does-not-exist/dismiss", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestContactSyncConflicts_Unauthenticated covers currentUserID's early-return
// branch: without a userID in context every handler 401s before touching the
// DB.
func TestContactSyncConflicts_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/contact-sync-conflicts", ListContactSyncConflicts)
	router.POST("/contact-sync-conflicts/:id/restore", RestoreContactSyncConflict)
	router.POST("/contact-sync-conflicts/:id/dismiss", DismissContactSyncConflict)

	for _, tc := range []struct {
		method, path string
	}{
		{"GET", "/contact-sync-conflicts"},
		{"POST", "/contact-sync-conflicts/x/restore"},
		{"POST", "/contact-sync-conflicts/x/dismiss"},
	} {
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
}

// TestSyncConflictEndpoints_DBError exercises the AbortWithError branches by
// closing the underlying *sql.DB out from under gorm before the request
// (mirrors contact_subscription_controller_test.go's TestListContactSubscriptions_DBError).
func TestSyncConflictEndpoints_DBError(t *testing.T) {
	db, router, user := setupSyncConflictRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Grace"}
	require.NoError(t, db.Create(&contact).Error)
	sub := models.ContactSubscription{UserID: user.ID, Name: "Work", URL: "https://example.com/dav/"}
	require.NoError(t, db.Create(&sub).Error)
	conflict := seedControllerConflict(t, db, user.ID, contact, sub, models.SyncConflictFieldPhone, "A", "B")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("GET", "/contact-sync-conflicts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	req2, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+conflict.ID+"/restore", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusInternalServerError, w2.Code)

	req3, _ := http.NewRequest("POST", "/contact-sync-conflicts/"+conflict.ID+"/dismiss", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusInternalServerError, w3.Code)
}
