package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1471: the contact cascade (services.DeleteContactAssociations, shared by
// DeleteContact, CommitContactMerge and the bulk delete) issues bulk
// Where(...).Delete(&Model{}) calls whose model hooks fire on a zero-value
// receiver. GORM's soft-delete clause sets DeletedAt on that receiver, so the
// per-model DeletedAt guard did not skip it and auditAfterDelete queued an
// event with user_id 0 that the audit_events FK rejected (698 warnings per
// E2E run). The guard is now central (models.skipZeroIdentityAudit).
//
// Hand-verified: removing the skipZeroIdentityAudit calls in
// models/audit.go makes every cascade subtest here fail on the log assertion.

type cascadeAuditEnv struct {
	db     *gorm.DB
	router *gin.Engine
	user   models.User
}

func newCascadeAuditEnv(t *testing.T) *cascadeAuditEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	// Seeding must not record events; arm() installs the real recorder after.
	models.DisableAudit(db)
	user := models.User{Username: "cascadeaudit", Password: "password123!A", Email: "cascadeaudit@example.com"}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.DELETE("/contacts/:id", DeleteContact)
	router.POST("/contacts/merge", middleware.ValidateJSONMiddleware(&models.ContactMergeRequest{}), CommitContactMerge)
	router.POST("/contacts/bulk", middleware.ValidateJSONMiddleware(&models.BulkContactOperationInput{}), BulkContactOperation)
	return &cascadeAuditEnv{db: db, router: router, user: user}
}

// seedChildren gives a contact one of every cascade child that fires an audit
// hook on a bulk delete/update.
func (e *cascadeAuditEnv) seedChildren(t *testing.T, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: e.user.ID, Firstname: name}
	require.NoError(t, e.db.Create(&c).Error)
	require.NoError(t, e.db.Create(&models.Note{UserID: e.user.ID, ContactID: &c.ID, Content: "n"}).Error)
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.user.ID, ContactID: &c.ID, Message: "r", Recurrence: "once", RemindAt: time.Now().Add(24 * time.Hour)}).Error)
	require.NoError(t, e.db.Create(&models.LifeEvent{UserID: e.user.ID, EntityID: c.VCardUID, Type: "moved"}).Error)
	require.NoError(t, e.db.Create(&models.Gift{UserID: e.user.ID, EntityID: c.VCardUID, Description: "g", Status: "idea"}).Error)
	return c
}

// arm registers the audit recorder only after seeding so setup-time creates
// don't queue events whose completion races the assertions.
func (e *cascadeAuditEnv) arm(t *testing.T) *bytes.Buffer {
	t.Helper()
	models.NewAuditRecorder(e.db, models.WithSync())
	return captureTestLogger(t)
}

func (e *cascadeAuditEnv) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

var cascadeChildTypes = []string{
	models.AuditEntityNote, models.AuditEntityReminder, models.AuditEntityLifeEvent, models.AuditEntityGift,
}

func (e *cascadeAuditEnv) assertClean(t *testing.T, buf *bytes.Buffer, wantContactDeletes, wantChildEvents int64) {
	t.Helper()
	var n int64
	require.NoError(t, e.db.Model(&models.AuditEvent{}).
		Where("entity_type = ? AND operation = ?", models.AuditEntityContact, models.AuditOpDelete).Count(&n).Error)
	assert.Equal(t, wantContactDeletes, n, "exactly one contact/delete audit event per deleted contact")

	var children int64
	require.NoError(t, e.db.Model(&models.AuditEvent{}).
		Where("entity_type IN ?", cascadeChildTypes).Count(&children).Error)
	assert.Equal(t, wantChildEvents, children, "cascade children must produce no audit events (not undoable; the contact's own event is the record)")

	var zeroIdentity int64
	require.NoError(t, e.db.Model(&models.AuditEvent{}).
		Where("entity_type IN ? AND (entity_id = '' OR entity_id = '0')", cascadeChildTypes).Count(&zeroIdentity).Error)
	assert.Zero(t, zeroIdentity, "no child event may carry an empty/0 entity id")

	logged := buf.String()
	assert.NotContains(t, logged, "failed to persist audit event")
	assert.NotContains(t, logged, "FOREIGN KEY constraint failed")
}

func TestDeleteContactCascade_NoSpuriousChildAuditEvents(t *testing.T) {
	e := newCascadeAuditEnv(t)
	c := e.seedChildren(t, "Del")
	buf := e.arm(t)

	w := e.do(t, "DELETE", "/contacts/"+strconv.FormatUint(uint64(c.ID), 10), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	e.assertClean(t, buf, 1, 0)
}

func TestMergeCascade_NoSpuriousChildAuditEvents(t *testing.T) {
	e := newCascadeAuditEnv(t)
	keeper := e.seedChildren(t, "Keep")
	loser := e.seedChildren(t, "Lose")
	buf := e.arm(t)

	w := e.do(t, "POST", "/contacts/merge", models.ContactMergeRequest{
		KeepID: keeper.ID, MergeID: loser.ID,
		Resolutions: map[string]string{"firstname": "Keep"},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The gift/life-event re-point is the "gift create" bulk-update source.
	// The single child event allowed is the real merge-summary note the
	// handler creates on the keeper (a genuine single-row create).
	e.assertClean(t, buf, 1, 1)
}

func TestBulkDeleteCascade_NoSpuriousChildAuditEvents(t *testing.T) {
	e := newCascadeAuditEnv(t)
	a := e.seedChildren(t, "A")
	b := e.seedChildren(t, "B")
	buf := e.arm(t)

	w := e.do(t, "POST", "/contacts/bulk", models.BulkContactOperationInput{
		Action: "delete", VCardUIDs: []string{a.VCardUID, b.VCardUID},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	e.assertClean(t, buf, 2, 0)
}

// A single-row delete of a real child must still be audited: the central
// guard only drops zero-identity (bulk-hook) events.
func TestSingleRowChildDelete_StillAudited(t *testing.T) {
	e := newCascadeAuditEnv(t)
	c := e.seedChildren(t, "Single")
	var note models.Note
	require.NoError(t, e.db.Where("contact_id = ?", c.ID).First(&note).Error)
	buf := e.arm(t)

	require.NoError(t, e.db.Delete(&note).Error)

	var events []models.AuditEvent
	require.NoError(t, e.db.Where("entity_type = ?", models.AuditEntityNote).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, strconv.FormatUint(uint64(note.ID), 10), events[0].EntityID)
	assert.Equal(t, models.AuditOpDelete, events[0].Operation)
	assert.NotContains(t, buf.String(), "failed to persist audit event")
}
