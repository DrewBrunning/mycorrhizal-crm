package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doExternalLinkJSON is a tiny request helper for the substrate routes.
func doExternalLinkJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestCreateExternalIdentityRoundTrip covers the happy path: create → list
// (filtered by contact) → get → update → delete, all through the routes.
func TestCreateExternalIdentityRoundTrip(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-identities", withValidated(func() any { return &models.ExternalIdentityInput{} }), CreateExternalIdentity)
	router.GET("/external-identities", ListExternalIdentities)
	router.GET("/external-identities/:id", GetExternalIdentity)
	router.PUT("/external-identities/:id", withValidated(func() any { return &models.ExternalIdentityInput{} }), UpdateExternalIdentity)
	router.DELETE("/external-identities/:id", DeleteExternalIdentity)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// Create with a JSON metadata payload (system-specific data, not columns).
	resp := doExternalLinkJSON(t, router, "POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "immich", ExternalID: "person-123",
		URL:      "https://immich.example/people/person-123",
		Metadata: map[string]interface{}{"person_name": "Alice"},
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created struct {
		ExternalIdentity models.ExternalIdentity `json:"external_identity"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	identityID := created.ExternalIdentity.ID
	require.NotEmpty(t, identityID)
	assert.Equal(t, "immich", created.ExternalIdentity.System)
	assert.Equal(t, "idle", created.ExternalIdentity.SyncStatus)
	assert.Equal(t, "Alice", created.ExternalIdentity.Metadata["person_name"])

	// List filtered by contact.
	listResp := doExternalLinkJSON(t, router, "GET", "/external-identities?contact_id="+contact.VCardUID+"&system=immich", nil)
	require.Equal(t, http.StatusOK, listResp.Code)
	var listed struct {
		ExternalIdentities []models.ExternalIdentity `json:"external_identities"`
		Total              int64                     `json:"total"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	assert.EqualValues(t, 1, listed.Total)

	// Get.
	getResp := doExternalLinkJSON(t, router, "GET", "/external-identities/"+identityID, nil)
	require.Equal(t, http.StatusOK, getResp.Code)
	var got models.ExternalIdentity
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &got))
	assert.Equal(t, contact.VCardUID, got.EntityID)

	// Update (natural-key change must be uniqueness-checked, not blind).
	updResp := doExternalLinkJSON(t, router, "PUT", "/external-identities/"+identityID, models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "immich", ExternalID: "person-999",
		Metadata: map[string]interface{}{"person_name": "Alice Updated"},
	})
	require.Equal(t, http.StatusOK, updResp.Code, updResp.Body.String())
	var updated models.ExternalIdentity
	require.NoError(t, json.Unmarshal(updResp.Body.Bytes(), &updated))
	assert.Equal(t, "person-999", updated.ExternalID)
	assert.Equal(t, "Alice Updated", updated.Metadata["person_name"])

	// Delete (hard).
	delResp := doExternalLinkJSON(t, router, "DELETE", "/external-identities/"+identityID, nil)
	require.Equal(t, http.StatusOK, delResp.Code)
	var count int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("id = ?", identityID).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

// TestCreateExternalIdentityDuplicateIsConflict pins the natural-key 409.
func TestCreateExternalIdentityDuplicateIsConflict(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-identities", withValidated(func() any { return &models.ExternalIdentityInput{} }), CreateExternalIdentity)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	first := doExternalLinkJSON(t, router, "POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "immich", ExternalID: "person-123",
	})
	require.Equal(t, http.StatusCreated, first.Code)

	dup := doExternalLinkJSON(t, router, "POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "immich", ExternalID: "person-123",
	})
	assert.Equal(t, http.StatusConflict, dup.Code)
}

// TestCreateExternalIdentityRejectsForeignContact pins ownership scoping: an
// identity can only be created against a contact the user owns.
func TestCreateExternalIdentityRejectsForeignContact(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-identities", withValidated(func() any { return &models.ExternalIdentityInput{} }), CreateExternalIdentity)

	otherUser := models.User{Username: "other", Password: "x", Email: "other-ext@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	resp := doExternalLinkJSON(t, router, "POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: othersContact.VCardUID, System: "immich", ExternalID: "person-1",
	})
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// TestGetExternalIdentityScopesToUser pins that one user cannot read (or
// delete/update) another user's identity — a cross-user fetch is a 404, not
// a leak.
func TestGetExternalIdentityScopesToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/external-identities/:id", GetExternalIdentity)
	router.DELETE("/external-identities/:id", DeleteExternalIdentity)

	otherUser := models.User{Username: "other", Password: "x", Email: "other-ext2@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)
	otherIdentity := models.ExternalIdentity{UserID: otherUser.ID, EntityID: othersContact.VCardUID, System: "immich", ExternalID: "person-2"}
	require.NoError(t, db.Create(&otherIdentity).Error)

	getResp := doExternalLinkJSON(t, router, "GET", "/external-identities/"+otherIdentity.ID, nil)
	assert.Equal(t, http.StatusNotFound, getResp.Code)

	delResp := doExternalLinkJSON(t, router, "DELETE", "/external-identities/"+otherIdentity.ID, nil)
	assert.Equal(t, http.StatusNotFound, delResp.Code)
}

// TestExternalLinkSubstrate_IsGenericAcrossSystems demonstrates T14's "done
// when" requirement: the substrate must be demonstrably generic — a second,
// unrelated integration (Paperless-ngx) uses the exact same routes and
// schemas as Immich with zero schema or controller changes. "paperless" is
// just another value of the open `system` classifier.
func TestExternalLinkSubstrate_IsGenericAcrossSystems(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-identities", withValidated(func() any { return &models.ExternalIdentityInput{} }), CreateExternalIdentity)
	router.POST("/external-activities", withValidated(func() any { return &models.ExternalActivityInput{} }), CreateExternalActivity)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	// A Paperless-ngx identity link (the document "IS this thing in Paperless").
	idResp := doExternalLinkJSON(t, router, "POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "paperless", ExternalID: "doc-42",
		URL:      "https://paperless.example/documents/42",
		Metadata: map[string]interface{}{"title": "Immich setup notes"},
	})
	require.Equal(t, http.StatusCreated, idResp.Code, idResp.Body.String())

	// A Paperless activity (a document was OCR'd) — the same route Immich's
	// enrichment sync uses, no schema change.
	actResp := doExternalLinkJSON(t, router, "POST", "/external-activities", models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "paperless", ExternalID: "doc-42-ocr",
		Type: "document-processed", OccurredAt: time.Now(),
		Payload: map[string]interface{}{"document_id": "42"},
	})
	require.Equal(t, http.StatusCreated, actResp.Code, actResp.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("user_id = ? AND system = ?", user.ID, "paperless").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ? AND source_system = ?", user.ID, "paperless").Count(&count).Error)
	assert.EqualValues(t, 1, count, "a second integration adopts the substrate with no schema changes")
}

// TestCreateExternalActivityRoundTrip covers the enrichment-event happy path
// plus the timeline-facing ?contact_id= list filter.
func TestCreateExternalActivityRoundTrip(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-activities", withValidated(func() any { return &models.ExternalActivityInput{} }), CreateExternalActivity)
	router.GET("/external-activities", ListExternalActivities)
	router.GET("/external-activities/:id", GetExternalActivity)
	router.PUT("/external-activities/:id", withValidated(func() any { return &models.ExternalActivityInput{} }), UpdateExternalActivity)
	router.DELETE("/external-activities/:id", DeleteExternalActivity)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	when := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	resp := doExternalLinkJSON(t, router, "POST", "/external-activities", models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "asset-1",
		Type: "photo-appearance", OccurredAt: when,
		Payload: map[string]interface{}{"asset_id": "asset-1"},
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created struct {
		ExternalActivity models.ExternalActivity `json:"external_activity"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	activityID := created.ExternalActivity.ID
	require.NotEmpty(t, activityID)
	assert.Equal(t, "external", created.ExternalActivity.Provenance)

	// The contact timeline filter — what "attaches an ExternalActivity into a
	// contact's timeline" reads.
	listResp := doExternalLinkJSON(t, router, "GET", "/external-activities?contact_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code)
	var listed struct {
		ExternalActivities []models.ExternalActivity `json:"external_activities"`
		Total              int64                     `json:"total"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	assert.EqualValues(t, 1, listed.Total)
	assert.Equal(t, "asset-1", listed.ExternalActivities[0].ExternalID)

	// Update + delete.
	updResp := doExternalLinkJSON(t, router, "PUT", "/external-activities/"+activityID, models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "asset-1",
		Type: "photo-appearance", OccurredAt: when, Provenance: "user",
	})
	require.Equal(t, http.StatusOK, updResp.Code)
	delResp := doExternalLinkJSON(t, router, "DELETE", "/external-activities/"+activityID, nil)
	require.Equal(t, http.StatusOK, delResp.Code)
	var count int64
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("id = ?", activityID).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

// TestCreateExternalActivityDuplicateIsConflict pins the natural-key 409.
func TestCreateExternalActivityDuplicateIsConflict(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-activities", withValidated(func() any { return &models.ExternalActivityInput{} }), CreateExternalActivity)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	first := doExternalLinkJSON(t, router, "POST", "/external-activities", models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "asset-1",
		Type: "photo-appearance", OccurredAt: time.Now(),
	})
	require.Equal(t, http.StatusCreated, first.Code)

	dup := doExternalLinkJSON(t, router, "POST", "/external-activities", models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "asset-1",
		Type: "photo-appearance", OccurredAt: time.Now(),
	})
	assert.Equal(t, http.StatusConflict, dup.Code)
}

// TestCreateExternalActivityRejectsForeignContact pins ownership scoping on
// the enrichment path too.
func TestCreateExternalActivityRejectsForeignContact(t *testing.T) {
	db, router := setupRouter()
	router.POST("/external-activities", withValidated(func() any { return &models.ExternalActivityInput{} }), CreateExternalActivity)

	otherUser := models.User{Username: "other", Password: "x", Email: "other-ea@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&othersContact).Error)

	resp := doExternalLinkJSON(t, router, "POST", "/external-activities", models.ExternalActivityInput{
		EntityID: othersContact.VCardUID, SourceSystem: "immich", ExternalID: "asset-9",
		Type: "photo-appearance", OccurredAt: time.Now(),
	})
	assert.Equal(t, http.StatusNotFound, resp.Code)
}
