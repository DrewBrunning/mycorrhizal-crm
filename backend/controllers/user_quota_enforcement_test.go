package controllers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// These are the handler-level half of issue #950's per-user quota: the service
// unit tests (services/user_quota_test.go) prove the counting, these prove the
// REST create paths actually call it, return 507, do not write, and — the
// cross-user guarantee — never let one user's usage block another.

// quotaTestResponder is the minimal body decoder for the error envelope
// AbortWithError writes.
type quotaErrEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func setupQuotaRouter(t *testing.T, cfg config.Config) (*gorm.DB, *gin.Engine, models.User, models.User, string) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)
	alice := models.User{Username: "quota-router-alice", Email: "quota-router-alice@example.com", Password: "password123!A"}
	bob := models.User{Username: "quota-router-bob", Email: "quota-router-bob@example.com", Password: "password123!A"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)

	attachmentsDir := filepath.Join(t.TempDir(), "attachments")
	cfg.ProfilePhotoDir = t.TempDir()
	cfg.AttachmentsDir = attachmentsDir

	router := gin.Default()
	router.MaxMultipartMemory = 10 << 20
	router.Use(func(c *gin.Context) {
		userID := alice.ID
		if c.GetHeader("X-Test-User") == "bob" {
			userID = bob.ID
		}
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.POST("/contacts/:id/notes", withValidated(func() any { return &models.NoteInput{} }), CreateNote)
	router.POST("/notes", withValidated(func() any { return &models.NoteInput{} }), CreateUnassignedNote)
	router.POST("/relationship-edges", withValidated(func() any { return &models.RelationshipEdgeInput{} }), CreateRelationshipEdge)
	router.POST("/contacts/:id/attachments", func(c *gin.Context) { UploadAttachment(c, &cfg) })
	return db, router, alice, bob, attachmentsDir
}

func quotaPost(t *testing.T, router *gin.Engine, path, asUser string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if asUser != "" {
		req.Header.Set("X-Test-User", asUser)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func contactBody(name string) models.ContactRecordInput {
	return models.ContactRecordInput{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: name}}},
	}}
}

func assertQuotaRefusal(t *testing.T, w *httptest.ResponseRecorder, envVar string) {
	t.Helper()
	require.Equal(t, http.StatusInsufficientStorage, w.Code, "over-quota create must be 507, got %d: %s", w.Code, w.Body.String())
	var body quotaErrEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, body.Error.Code)
	assert.Contains(t, body.Error.Message, envVar, "the refusal names the config variable an operator would raise")
	assert.Contains(t, body.Error.Message, "quota", "the refusal explains it is a quota, not an IO failure")
}

// TestCreateContact_QuotaAndIsolation drives the contact create path: the
// limit is enforced, the refusal is a typed 507, and a second user is entirely
// unaffected by the first user's usage.
func TestCreateContact_QuotaAndIsolation(t *testing.T) {
	cfg := config.Config{PerUserContactLimit: 1}
	db, router, alice, _, _ := setupQuotaRouter(t, cfg)

	first := quotaPost(t, router, "/contacts", "alice", contactBody("Alice One"))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := quotaPost(t, router, "/contacts", "alice", contactBody("Alice Two"))
	assertQuotaRefusal(t, second, "PER_USER_CONTACT_LIMIT")

	var aliceCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", alice.ID).Count(&aliceCount).Error)
	assert.Equal(t, int64(1), aliceCount, "the refused create must not have written a row")

	// bob has his own budget.
	bobFirst := quotaPost(t, router, "/contacts", "bob", contactBody("Bob One"))
	assert.Equal(t, http.StatusCreated, bobFirst.Code, "alice's usage must never count against bob: %s", bobFirst.Body.String())
}

// TestCreateNote_QuotaAndIsolation covers both note entry points — the
// contact-scoped one and the unassigned one — sharing one per-user count.
func TestCreateNote_QuotaAndIsolation(t *testing.T) {
	cfg := config.Config{PerUserNoteLimit: 1}
	db, router, alice, _, _ := setupQuotaRouter(t, cfg)
	contact := models.Contact{UserID: alice.ID, Firstname: "Noted"}
	require.NoError(t, db.Create(&contact).Error)

	first := quotaPost(t, router, "/contacts/"+itoa2(contact.ID)+"/notes", "alice", models.NoteInput{Content: "one"})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	// The unassigned endpoint must see the same count.
	second := quotaPost(t, router, "/notes", "alice", models.NoteInput{Content: "two"})
	assertQuotaRefusal(t, second, "PER_USER_NOTE_LIMIT")

	var count int64
	require.NoError(t, db.Model(&models.Note{}).Where("user_id = ?", alice.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	bobNote := quotaPost(t, router, "/notes", "bob", models.NoteInput{Content: "bob's note"})
	assert.Equal(t, http.StatusOK, bobNote.Code, "alice's notes must not count against bob: %s", bobNote.Body.String())
}

// TestCreateRelationshipEdge_QuotaAndIsolation covers the edge create path.
func TestCreateRelationshipEdge_QuotaAndIsolation(t *testing.T) {
	cfg := config.Config{PerUserRelationshipEdgeLimit: 1}
	db, router, alice, bob, _ := setupQuotaRouter(t, cfg)
	a := models.Contact{UserID: alice.ID, Firstname: "A"}
	b := models.Contact{UserID: alice.ID, Firstname: "B"}
	c := models.Contact{UserID: alice.ID, Firstname: "C"}
	for _, x := range []*models.Contact{&a, &b, &c} {
		require.NoError(t, db.Create(x).Error)
	}

	first := quotaPost(t, router, "/relationship-edges", "alice", models.RelationshipEdgeInput{SourceID: a.VCardUID, TargetID: b.VCardUID, Type: "parent_of"})
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := quotaPost(t, router, "/relationship-edges", "alice", models.RelationshipEdgeInput{SourceID: a.VCardUID, TargetID: c.VCardUID, Type: "sibling_of"})
	assertQuotaRefusal(t, second, "PER_USER_RELATIONSHIP_EDGE_LIMIT")

	var count int64
	require.NoError(t, db.Model(&models.RelationshipEdge{}).Where("user_id = ?", alice.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// bob's own two contacts and edge are independent.
	ba := models.Contact{UserID: bob.ID, Firstname: "BA"}
	bb := models.Contact{UserID: bob.ID, Firstname: "BB"}
	require.NoError(t, db.Create(&ba).Error)
	require.NoError(t, db.Create(&bb).Error)
	bobEdge := quotaPost(t, router, "/relationship-edges", "bob", models.RelationshipEdgeInput{SourceID: ba.VCardUID, TargetID: bb.VCardUID, Type: "parent_of"})
	assert.Equal(t, http.StatusCreated, bobEdge.Code, bobEdge.Body.String())
}

// TestCreateRelationshipEdge_ThinContactRespectsContactQuota is the bypass
// guard: a thin endpoint creates a Contact row, so it must obey
// PER_USER_CONTACT_LIMIT too, even though the edge itself is unlimited. The
// whole transaction must roll back — no edge, no thin contact.
func TestCreateRelationshipEdge_ThinContactRespectsContactQuota(t *testing.T) {
	cfg := config.Config{PerUserContactLimit: 1}
	db, router, alice, _, _ := setupQuotaRouter(t, cfg)
	existing := models.Contact{UserID: alice.ID, Firstname: "Existing"}
	require.NoError(t, db.Create(&existing).Error)

	w := quotaPost(t, router, "/relationship-edges", "alice", models.RelationshipEdgeInput{
		SourceID:   existing.VCardUID,
		Type:       "sibling_of",
		TargetThin: &models.ThinContactInput{Name: "Thin"},
	})
	assertQuotaRefusal(t, w, "PER_USER_CONTACT_LIMIT")

	var contactCount, edgeCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", alice.ID).Count(&contactCount).Error)
	require.NoError(t, db.Model(&models.RelationshipEdge{}).Where("user_id = ?", alice.ID).Count(&edgeCount).Error)
	assert.Equal(t, int64(1), contactCount, "the thin contact must not have been created")
	assert.Zero(t, edgeCount, "the edge write must have rolled back with the thin contact")
}

// uploadQuotaFile posts a multipart attachment of the given size.
func uploadQuotaFile(t *testing.T, router *gin.Engine, asUser, contactID string, size int) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "cv.pdf")
	require.NoError(t, err)
	// A PDF magic prefix keeps DetectContentType on application/pdf, off the
	// image metadatastrip path.
	data := append([]byte("%PDF-1.4 "), bytes.Repeat([]byte("a"), size)...)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, _ := http.NewRequest("POST", "/contacts/"+contactID+"/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if asUser != "" {
		req.Header.Set("X-Test-User", asUser)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestUploadAttachment_QuotaStopsBeforeDisk proves storage is summed across
// uploads and the refusal happens before the file is written: no orphan bytes
// survive a 507.
func TestUploadAttachment_QuotaStopsBeforeDisk(t *testing.T) {
	cfg := config.Config{PerUserAttachmentQuotaMB: 1} // 1 MiB
	db, router, alice, bob, dir := setupQuotaRouter(t, cfg)
	contact := models.Contact{UserID: alice.ID, Firstname: "Files"}
	require.NoError(t, db.Create(&contact).Error)

	halfMiB := 600 * 1024
	first := uploadQuotaFile(t, router, "alice", itoa2(contact.ID), halfMiB)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := uploadQuotaFile(t, router, "alice", itoa2(contact.ID), halfMiB)
	assertQuotaRefusal(t, second, "PER_USER_ATTACHMENT_QUOTA_MB")

	var count int64
	require.NoError(t, db.Model(&models.Attachment{}).Where("user_id = ?", alice.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "no second attachment row")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the refused upload must not leave a file behind")

	// A different user's storage is their own.
	bobContact := models.Contact{UserID: bob.ID, Firstname: "Bob Files"}
	require.NoError(t, db.Create(&bobContact).Error)
	bobUpload := uploadQuotaFile(t, router, "bob", itoa2(bobContact.ID), halfMiB)
	assert.Equal(t, http.StatusCreated, bobUpload.Code, "alice's bytes must not count against bob: %s", bobUpload.Body.String())
}

// TestCreateContact_QuotaDisabledByDefault pins the opt-in contract: with
// every PER_USER_* unset (all zero), creates are never refused.
func TestCreateContact_QuotaDisabledByDefault(t *testing.T) {
	db, router, alice, _, _ := setupQuotaRouter(t, config.Config{})
	for i := 0; i < 3; i++ {
		w := quotaPost(t, router, "/contacts", "alice", contactBody("Contact"))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}
	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", alice.ID).Count(&count).Error)
	assert.Equal(t, int64(3), count)
}
