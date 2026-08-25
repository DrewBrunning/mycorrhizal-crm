package controllers

import (
	"mycorrhizal/config"
	"mycorrhizal/database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOtherControllers_MalformedID_NotServerError is issue #524 follow-up
// audit coverage. It does NOT change behavior — every case here already
// passes on main. Investigation (see PR discussion) found the
// "malformed :id -> GORM/driver error -> 500" bug class the rest of this
// file's package fixes is specific to ONE code shape:
// db.Where(...).First(&x, id) — GORM's "magic second positional arg" primary
// key shorthand, which for a non-numeric string against a uint PK produces a
// raw SQL parse error (confirmed empirically: GORM emits the string as bare
// SQL for a non-numeric id, e.g. `WHERE user_id = 1 AND not-a-number`,
// yielding "no such column" instead of ErrRecordNotFound).
//
// Every other handler in this package uses the different, SAFE shape
// db.Where("id = ? AND ...", id, ...).First(&x) -- id passed as an ordinary
// bound query parameter, never specially interpreted by GORM. Confirmed
// empirically (against the real migrated schema, per CLAUDE.md's "test
// against the real migrated schema, not AutoMigrate" trap) that this shape
// returns a clean ErrRecordNotFound for a garbage id regardless of whether
// the target column is TEXT (Circle/Household/Tag/Gift/LifeEvent/
// ConversationAgenda/LinkFieldType/RelationshipEdge/Preference/
// CadencePolicy/ExternalIdentity/ExternalActivity/ContactShare/
// ContactSyncConflict all use a UUID string PK) or INTEGER (AuditEvent) --
// SQLite's type-affinity comparison just finds no match, it never attempts
// a Go-level type conversion the way the magic-second-arg shape does.
// Webhook/ApiToken/Attachment (uint PK, gorm.Model) additionally
// pre-validate the id with strconv before it ever reaches a query.
//
// This test pins that finding as a permanent regression guard: if a future
// change reintroduces the vulnerable shape on any of these routes, this
// test starts failing with a 500 instead of the expected 400/404.
func TestOtherControllers_MalformedID_NotServerError(t *testing.T) {
	_, router := setupRouter()

	router.GET("/circles/:id", GetCircle)
	router.GET("/households/:id", GetHousehold)
	router.GET("/tags/:id", GetTag)
	router.GET("/gifts/:id", GetGift)
	router.GET("/life-events/:id", GetLifeEvent)
	router.GET("/conversation-agenda/:id", GetConversationAgenda)
	router.GET("/link-field-types/:id", GetLinkFieldType)
	router.GET("/relationship-edges/:id", GetRelationshipEdge)
	router.GET("/preferences/:id", GetPreference)
	router.GET("/cadence-policies/:id", GetCadencePolicy)
	router.GET("/external-identities/:id", GetExternalIdentity)
	router.GET("/external-activities/:id", GetExternalActivity)
	router.GET("/webhooks/:id", GetWebhook)
	router.DELETE("/api-tokens/:id", RevokeApiToken)

	// UUID-string-PK entities: a non-existent-but-plausible id is a clean
	// 404. A frankly malformed id ("null,null", the exact Schemathesis
	// payload that broke the uint-PK handlers elsewhere in this package)
	// must NOT 500 either -- it stays a 404 too, since a TEXT column simply
	// has no row matching that string.
	notFoundCases := []struct {
		method string
		path   string
	}{
		{"GET", "/circles/not-a-uuid"},
		{"GET", "/circles/null,null"},
		{"GET", "/households/not-a-uuid"},
		{"GET", "/tags/not-a-uuid"},
		{"GET", "/gifts/not-a-uuid"},
		{"GET", "/life-events/not-a-uuid"},
		{"GET", "/conversation-agenda/not-a-uuid"},
		{"GET", "/link-field-types/not-a-uuid"},
		{"GET", "/relationship-edges/not-a-uuid"},
		{"GET", "/preferences/not-a-uuid"},
		{"GET", "/cadence-policies/not-a-uuid"},
		{"GET", "/external-identities/not-a-uuid"},
		{"GET", "/external-activities/not-a-uuid"},
	}
	for _, tc := range notFoundCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusNotFound, w.Code, "%s %s: got %d, body %s", tc.method, tc.path, w.Code, w.Body.String())
		})
	}

	// uint-PK entities: these handlers already validate the id with strconv
	// before querying, so a non-numeric id is a 400.
	badRequestCases := []struct {
		method string
		path   string
	}{
		{"GET", "/webhooks/not-a-number"},
		{"DELETE", "/api-tokens/not-a-number"},
	}
	for _, tc := range badRequestCases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, "%s %s: got %d, body %s", tc.method, tc.path, w.Code, w.Body.String())
		})
	}
}

// TestDownloadAttachment_MalformedID_Returns400 covers attachment_controller.go's
// loadOwnedAttachment (uint PK, gorm.Model), which pre-validates via
// strconv before querying -- part of the same #524 follow-up audit as
// TestOtherControllers_MalformedID_NotServerError above.
func TestDownloadAttachment_MalformedID_Returns400(t *testing.T) {
	_, router := setupRouter()
	cfg := &config.Config{AttachmentsDir: t.TempDir()}
	router.GET("/attachments/:id/download", func(c *gin.Context) { DownloadAttachment(c, cfg) })

	req, _ := http.NewRequest("GET", "/attachments/not-a-number/download", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestUndoAuditEvent_MalformedID_Returns404 covers audit_controller.go's
// UndoAuditEvent against the real migrated schema (models.AuditEvent isn't
// in the AutoMigrate list setupRouter() uses elsewhere in this package, and
// CLAUDE.md's real-DB trap applies): AuditEvent.ID is a uint PK, but
// UndoAuditEvent uses the safe db.Where("id = ? AND ...", id, ...).First(&x)
// shape (not the vulnerable magic-second-arg one), so a malformed id is a
// clean 404, not a 500.
func TestUndoAuditEvent_MalformedID_Returns404(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "x.db"))
	require.NoError(t, err)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Next()
	})
	router.POST("/audit/:id/undo", UndoAuditEvent)

	req, _ := http.NewRequest("POST", "/audit/not-a-number/undo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	req2, _ := http.NewRequest("POST", "/audit/null,null/undo", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code, w2.Body.String())
}
