package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAuditRouter builds a real-schema router with the audit endpoints and
// the audit recorder registered, so hooks fire into the same DB the handler
// reads.
func setupAuditRouter(t *testing.T, cfg config.Config) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "audit-ctrl.db"))
	require.NoError(t, err)
	models.RegisterAuditDB(db)

	user := models.User{Username: "auditctrl", Password: "password123!A", Email: "auditctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.GET("/audit", ListAuditEvents)
	router.POST("/audit/:id/undo", UndoAuditEvent)
	return db, router, user
}

func TestUndoAuditEvent_RestoresContact(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	models.AuditFlush()

	contact := models.Contact{UserID: user.ID, Firstname: "Original", Lastname: "Name"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Firstname = "Changed"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).First(&event).Error)

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", contact.VCardUID).First(&reloaded).Error)
	assert.Equal(t, "Original", reloaded.Firstname, "undo must restore the pre-update firstname")
}

// TestUndoAuditEvent_PreservesCardOnlyData pins T75 trigger 3 at the handler
// level: undoContact rebuilds from the event's before snapshot, which has
// never carried Card/CRM/Passthrough (all json:"-", see T82). Before T75 the
// stopgap's predecessor overwrote the contact's Card with a Record rebuilt
// from that snapshot, deleting every Card-only member; the T75 stopgap
// restores only the flat state and lets BeforeSave's merge carry the
// Card-only data through.
func TestUndoAuditEvent_PreservesCardOnlyData(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	models.AuditFlush()

	contact := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(contact, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(contact).Error)
	models.AuditFlush()

	// A flat-only edit produces an update event whose snapshot can express
	// only flat fields.
	contact.Firstname = "Changed"
	require.NoError(t, db.Save(contact).Error)
	models.AuditFlush()

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).Order("id desc").First(&event).Error)

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", contact.VCardUID).First(&reloaded).Error)
	assert.Equal(t, "Ada", reloaded.Firstname, "undo must restore the pre-update firstname from the flat snapshot")
	assertCardOnlyDataPreserved(t, reloaded)
}

func TestUndoAuditEvent_RejectsDeleteOperation(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})

	contact := models.Contact{UserID: user.ID, Firstname: "To Delete"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Delete(&contact).Error)
	models.AuditFlush()

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpDelete).First(&event).Error)

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "undo of a delete event must be rejected")
}

func TestUndoAuditEvent_RejectsPastRetention(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 1})

	contact := models.Contact{UserID: user.ID, Firstname: "Old"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Firstname = "Newer"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).First(&event).Error)

	// Age the event beyond the 1-day retention window. The append-only UPDATE
	// trigger rejects UPDATE, so delete and re-insert the row with a past
	// created_at (DELETE is not trigger-blocked; the purge is the sanctioned
	// deleter).
	oldCreated := time.Now().AddDate(0, 0, -3)
	require.NoError(t, db.Delete(&event).Error)
	event.CreatedAt = oldCreated
	event.UpdatedAt = oldCreated
	event.ID = 0
	require.NoError(t, db.Create(&event).Error)

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusGone, w.Code, "undo past retention must be rejected with 410")
}

func TestUndoAuditEvent_RejectsAnotherUsersEvent(t *testing.T) {
	db, router, _ := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})

	// Another user's contact + audit event.
	other := models.User{Username: "auditother", Password: "password123!A", Email: "auditother@example.com"}
	require.NoError(t, db.Create(&other).Error)
	contact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Firstname = "Edited"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_id = ?", contact.VCardUID).First(&event).Error)

	req, _ := http.NewRequest("POST", "/audit/"+auditItoa(event.ID)+"/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "another user's audit event must not be visible")
}

func TestListAuditEvents_ScopedToUser(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})

	other := models.User{Username: "auditlist", Password: "password123!A", Email: "auditlist@example.com"}
	require.NoError(t, db.Create(&other).Error)
	mine := models.Contact{UserID: user.ID, Firstname: "Mine"}
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&mine).Error)
	require.NoError(t, db.Create(&theirs).Error)
	models.AuditFlush()

	req, _ := http.NewRequest("GET", "/audit?entity_type="+models.AuditEntityContact+"&entity_id="+mine.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Events []models.AuditEvent `json:"audit_events"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 1)
	assert.Equal(t, mine.VCardUID, resp.Events[0].EntityID)
}

func auditItoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
