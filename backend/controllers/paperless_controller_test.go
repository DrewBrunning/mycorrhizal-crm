package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// paperlessControllerConfig is the test config for Paperless routes: a JWT
// secret (for credential encryption) and private addresses allowed (the fake
// server is on loopback).
func paperlessControllerConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

// paperlessTestRouter builds a router with the Paperless controller routes
// wired to the given db.
func paperlessTestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", paperlessControllerConfig())
		c.Next()
	})
	router.GET("/paperless/config", GetPaperlessConfig)
	router.PUT("/paperless/config", withValidated(func() any { return &models.PaperlessConfigInput{} }), SavePaperlessConfig)
	router.DELETE("/paperless/config", DeletePaperlessConfig)
	router.POST("/paperless/test-connection", TestPaperlessConnection)
	router.GET("/paperless/documents", ListPaperlessDocuments)
	router.POST("/paperless/contacts/:vcard_uid/link", LinkPaperlessContact)
	router.DELETE("/paperless/contacts/:vcard_uid/links/:identity_id", UnlinkPaperlessContact)
	return router
}

// seedPaperlessControllerDB seeds the models the Paperless controller touches
// and a contact for user 1.
func seedPaperlessControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.PaperlessConfig{}, &models.ExternalIdentity{}))
	user := models.User{Username: "paperless-ctrl", Password: "password123!A", Email: "paperless-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Example"}).Error)
	return db
}

func paperlessDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func TestGetPaperlessConfig_EmptyDefault(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "GET", "/paperless/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp services.PaperlessConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.HasAPIToken)
	assert.Empty(t, resp.BaseURL)
}

func TestSavePaperlessConfig_EncryptsTokenAndHidesIt(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "PUT", "/paperless/config", models.PaperlessConfigInput{
		BaseURL: "https://paperless.example", APIToken: "plaintext-token",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp services.PaperlessConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://paperless.example", resp.BaseURL)
	assert.True(t, resp.HasAPIToken)
	assert.NotContains(t, w.Body.String(), "plaintext-token", "the API token must never appear in a response")

	var stored models.PaperlessConfig
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.NotContains(t, stored.APITokenEncrypted, "plaintext-token")
	assert.Equal(t, "plaintext-token", mustDecrypt(t, stored.APITokenEncrypted))

	// Empty token on update keeps the stored one.
	w2 := paperlessDoJSON(t, router, "PUT", "/paperless/config", models.PaperlessConfigInput{BaseURL: "https://paperless.example"})
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.Equal(t, "plaintext-token", mustDecrypt(t, stored.APITokenEncrypted))
}

func TestSavePaperlessConfig_CreateWithoutTokenIsRejected(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "PUT", "/paperless/config", models.PaperlessConfigInput{BaseURL: "https://paperless.example"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSavePaperlessConfig_RejectsSchemelessBaseURL(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "PUT", "/paperless/config", models.PaperlessConfigInput{
		BaseURL: "paperless.example.com", APIToken: "k",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.PaperlessConfig{}).Where("user_id = ?", uint(1)).Count(&count).Error)
	assert.EqualValues(t, 0, count, "a rejected base URL must not be persisted")
}

// TestListPaperlessDocuments_NoConfigIs400 covers issue #524: no Paperless
// connection configured is the caller's own setup, not a server malfunction,
// so it must be a 400 — Schemathesis's not_a_server_error check flags any 5xx
// here.
func TestListPaperlessDocuments_NoConfigIs400(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "GET", "/paperless/documents?query=", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestTestPaperlessConnection_NoConfigIs400 covers issue #524: no Paperless
// connection configured is the caller's own setup, not a server malfunction,
// so it must be a 400 — Schemathesis's not_a_server_error check flags any 5xx
// here.
func TestTestPaperlessConnection_NoConfigIs400(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	w := paperlessDoJSON(t, router, "POST", "/paperless/test-connection", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTestPaperlessConnection_SuccessAndFailure(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	fake := newPaperlessTestServer(t, "sekret")
	defer fake.Close()
	fake.Me = map[string]any{"user_name": "alice", "id": 2}

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PaperlessConfig{UserID: 1, BaseURL: fake.URL(), APITokenEncrypted: enc}).Error)

	w := paperlessDoJSON(t, router, "POST", "/paperless/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result services.PaperlessConnectionTestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.True(t, result.OK)
	assert.Equal(t, "ok", result.Stage)
	assert.Contains(t, result.Message, "alice")

	// A diagnosed failure (wrong token) is still HTTP 200 — the diagnosis
	// itself succeeded.
	encBad, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.PaperlessConfig{}).Where("user_id = ?", uint(1)).Update("api_token_encrypted", encBad).Error)

	w2 := paperlessDoJSON(t, router, "POST", "/paperless/test-connection", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var failed services.PaperlessConnectionTestResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &failed))
	assert.False(t, failed.OK)
	assert.Equal(t, "auth", failed.Stage)
}

func TestListPaperlessDocuments_BrowsesFake(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	fake := newPaperlessTestServer(t, "sekret")
	defer fake.Close()
	fake.addDoc(1, "Lease Agreement", "lease.pdf", "2026-01-15", "2026-01-20T10:00:00Z")
	fake.addDoc(2, "Passport", "passport.pdf", "2026-02-01", "2026-02-05T10:00:00Z")

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PaperlessConfig{UserID: 1, BaseURL: fake.URL(), APITokenEncrypted: enc}).Error)

	w := paperlessDoJSON(t, router, "GET", "/paperless/documents", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Documents []services.PaperlessDocument `json:"documents"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Documents, 2)
	assert.Equal(t, "Lease Agreement", resp.Documents[0].Title)

	// query filters server-side.
	w2 := paperlessDoJSON(t, router, "GET", "/paperless/documents?query=passport", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Len(t, resp.Documents, 1)
	assert.Equal(t, "Passport", resp.Documents[0].Title)
}

func TestLinkPaperlessContact_FetchesAuthoritativeMetadata(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	fake := newPaperlessTestServer(t, "sekret")
	defer fake.Close()
	fake.addDoc(42, "Signed Contract", "contract.pdf", "2026-03-01", "2026-03-10T10:00:00Z")

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PaperlessConfig{UserID: 1, BaseURL: fake.URL(), APITokenEncrypted: enc}).Error)

	// The contact must be owned by user 1 — seedPaperlessControllerDB creates
	// it under the seeded user.
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	w := paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+contact.VCardUID+"/link", map[string]any{"document_id": "42"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Identity models.ExternalIdentity `json:"external_identity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, services.ExternalSystemPaperless, resp.Identity.System)
	assert.Equal(t, "42", resp.Identity.ExternalID)
	assert.Equal(t, fake.URL()+"/documents/42/details", resp.Identity.URL)
	title, _ := resp.Identity.Metadata["title"].(string)
	assert.Equal(t, "Signed Contract", title, "metadata must be fetched from Paperless, not trusted from the client")
}

func TestLinkPaperlessContact_DuplicateIsConflict(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	fake := newPaperlessTestServer(t, "sekret")
	defer fake.Close()
	fake.addDoc(42, "Signed Contract", "contract.pdf", "2026-03-01", "2026-03-10T10:00:00Z")

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PaperlessConfig{UserID: 1, BaseURL: fake.URL(), APITokenEncrypted: enc}).Error)

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	first := paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+contact.VCardUID+"/link", map[string]any{"document_id": "42"})
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	second := paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+contact.VCardUID+"/link", map[string]any{"document_id": "42"})
	assert.Equal(t, http.StatusConflict, second.Code)
}

func TestLinkPaperlessContact_ForeignContactIs404(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	// A contact owned by another user must be rejected.
	other := models.User{Username: "other", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&foreign).Error)

	w := paperlessDoJSON(t, router, "POST", "/paperless/contacts/"+foreign.VCardUID+"/link", map[string]any{"document_id": "1"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUnlinkPaperlessContact_RemovesOnlyThatIdentity(t *testing.T) {
	db := seedPaperlessControllerDB(t)
	router := paperlessTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	keep := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemPaperless, ExternalID: "1"}
	remove := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemPaperless, ExternalID: "2"}
	require.NoError(t, db.Create(&keep).Error)
	require.NoError(t, db.Create(&remove).Error)

	w := paperlessDoJSON(t, router, "DELETE", "/paperless/contacts/"+contact.VCardUID+"/links/"+remove.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("user_id = ? AND entity_id = ?", uint(1), contact.VCardUID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, keep.ID, mustLoadIdentity(t, db, keep.ID).ID)

	// Deleting an identity that is not a Paperless link (a different system)
	// must 404 — the route is scoped to the paperless system.
	immich := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: "immich", ExternalID: "person-x"}
	require.NoError(t, db.Create(&immich).Error)
	w2 := paperlessDoJSON(t, router, "DELETE", "/paperless/contacts/"+contact.VCardUID+"/links/"+immich.ID, nil)
	assert.Equal(t, http.StatusNotFound, w2.Code, w2.Body.String())

	// The unlink is scoped to the contact in the path: unlinking through a
	// different contact's vcard_uid must 404, not delete the identity.
	otherContact := models.Contact{UserID: 1, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	w3 := paperlessDoJSON(t, router, "DELETE", "/paperless/contacts/"+otherContact.VCardUID+"/links/"+keep.ID, nil)
	assert.Equal(t, http.StatusNotFound, w3.Code, w3.Body.String())
	// And the identity is still there.
	assert.Equal(t, keep.ID, mustLoadIdentity(t, db, keep.ID).ID)
}

func mustLoadIdentity(t *testing.T, db *gorm.DB, id string) models.ExternalIdentity {
	t.Helper()
	var identity models.ExternalIdentity
	require.NoError(t, db.First(&identity, "id = ?", id).Error)
	return identity
}
