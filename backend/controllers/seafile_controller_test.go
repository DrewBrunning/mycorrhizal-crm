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

func seafileControllerConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

func seafileTestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", seafileControllerConfig())
		c.Next()
	})
	router.GET("/seafile/config", GetSeafileConfig)
	router.PUT("/seafile/config", withValidated(func() any { return &models.SeafileConfigInput{} }), SaveSeafileConfig)
	router.DELETE("/seafile/config", DeleteSeafileConfig)
	router.POST("/seafile/test-connection", TestSeafileConnection)
	router.GET("/seafile/libraries", ListSeafileLibraries)
	router.GET("/seafile/libraries/:repo_id/dir", ListSeafileDir)
	router.POST("/seafile/contacts/:vcard_uid/link", LinkSeafileContact)
	router.DELETE("/seafile/contacts/:vcard_uid/links/:identity_id", UnlinkSeafileContact)
	return router
}

func seedSeafileControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.SeafileConfig{}, &models.ExternalIdentity{}))
	user := models.User{Username: "seafile-ctrl", Password: "password123!A", Email: "seafile-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Example"}).Error)
	return db
}

func seafileDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func connectSeafileControllerConfig(t *testing.T, db *gorm.DB, baseURL string) {
	t.Helper()
	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SeafileConfig{UserID: 1, BaseURL: baseURL, APITokenEncrypted: enc}).Error)
}

func TestGetSeafileConfig_EmptyDefault(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	w := seafileDoJSON(t, router, "GET", "/seafile/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp services.SeafileConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.HasAPIToken)
	assert.Empty(t, resp.BaseURL)
}

func TestSaveSeafileConfig_EncryptsTokenAndHidesIt(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	w := seafileDoJSON(t, router, "PUT", "/seafile/config", models.SeafileConfigInput{
		BaseURL: "https://seafile.example", APIToken: "plaintext-token",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp services.SeafileConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://seafile.example", resp.BaseURL)
	assert.True(t, resp.HasAPIToken)
	assert.NotContains(t, w.Body.String(), "plaintext-token", "the API token must never appear in a response")

	var stored models.SeafileConfig
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.Equal(t, "plaintext-token", mustDecrypt(t, stored.APITokenEncrypted))
}

func TestSaveSeafileConfig_CreateWithoutTokenIsRejected(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	w := seafileDoJSON(t, router, "PUT", "/seafile/config", models.SeafileConfigInput{BaseURL: "https://seafile.example"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTestSeafileConnection_SuccessAndFailure(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	fake := newSeafileTestServer(t, "sekret")
	defer fake.Close()

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SeafileConfig{UserID: 1, BaseURL: fake.URL(), APITokenEncrypted: enc}).Error)

	w := seafileDoJSON(t, router, "POST", "/seafile/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result services.SeafileConnectionTestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.True(t, result.OK)
	assert.Equal(t, "ok", result.Stage)

	// A diagnosed auth failure (wrong token) is still HTTP 200.
	require.NoError(t, db.Model(&models.SeafileConfig{}).Where("user_id = ?", uint(1)).
		Update("api_token_encrypted", mustDecryptEncrypt(t, "wrong")).Error)

	w2 := seafileDoJSON(t, router, "POST", "/seafile/test-connection", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var failed services.SeafileConnectionTestResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &failed))
	assert.False(t, failed.OK)
	assert.Equal(t, "auth", failed.Stage)
}

// mustDecryptEncrypt encrypts a new plaintext with the shared test JWT secret.
func mustDecryptEncrypt(t *testing.T, plaintext string) string {
	t.Helper()
	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", plaintext)
	require.NoError(t, err)
	return enc
}

func TestListSeafileLibrariesAndDir_BrowsesFake(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	fake := newSeafileTestServer(t, "sekret")
	defer fake.Close()
	fake.addLib("repo-1", "Personal")
	fake.addLib("repo-2", "Work")
	fake.addDirItem("repo-1", "/", "Documents", "dir", 0, 1000)
	fake.addDirItem("repo-1", "/", "contract.pdf", "file", 2048, 2000)
	connectSeafileControllerConfig(t, db, fake.URL())

	w := seafileDoJSON(t, router, "GET", "/seafile/libraries", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var libsResp struct {
		Libraries []services.SeafileLibrary `json:"libraries"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &libsResp))
	require.Len(t, libsResp.Libraries, 2)

	w2 := seafileDoJSON(t, router, "GET", "/seafile/libraries/repo-1/dir?path=/", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var dirResp struct {
		Items []services.SeafileItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &dirResp))
	require.Len(t, dirResp.Items, 2)
	assert.Equal(t, "contract.pdf", dirResp.Items[1].Name)
	assert.EqualValues(t, 2048, dirResp.Items[1].Size)
}

func TestLinkSeafileContact_StoresRepoRelativeLink(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	fake := newSeafileTestServer(t, "sekret")
	defer fake.Close()
	connectSeafileControllerConfig(t, db, fake.URL())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	w := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+contact.VCardUID+"/link", map[string]any{
		"repo_id": "repo-1", "path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file", "size": 2048,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Identity models.ExternalIdentity `json:"external_identity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, services.ExternalSystemSeafile, resp.Identity.System)
	assert.Equal(t, "repo-1:/Documents/contract.pdf", resp.Identity.ExternalID)
	assert.Equal(t, fake.URL()+"/lib/repo-1/Documents/contract.pdf", resp.Identity.URL)
	assert.Equal(t, "contract.pdf", resp.Identity.Metadata["name"])
	assert.Equal(t, "file", resp.Identity.Metadata["type"])
}

func TestLinkSeafileContact_DuplicateIsConflictAndForeignIs404(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	fake := newSeafileTestServer(t, "sekret")
	defer fake.Close()
	connectSeafileControllerConfig(t, db, fake.URL())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	body := map[string]any{"repo_id": "repo-1", "path": "/contract.pdf", "name": "contract.pdf", "type": "file"}
	first := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+contact.VCardUID+"/link", body)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+contact.VCardUID+"/link", body)
	assert.Equal(t, http.StatusConflict, second.Code)

	// Foreign contact.
	other := models.User{Username: "other", Password: "password123!A", Email: "other2@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&foreign).Error)
	wForeign := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+foreign.VCardUID+"/link", body)
	assert.Equal(t, http.StatusNotFound, wForeign.Code)
}

func TestUnlinkSeafileContact_RemovesOnlyThatIdentity(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	remove := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemSeafile, ExternalID: "repo-1:/a.pdf"}
	other := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemSeafile, ExternalID: "repo-1:/b.pdf"}
	require.NoError(t, db.Create(&remove).Error)
	require.NoError(t, db.Create(&other).Error)

	w := seafileDoJSON(t, router, "DELETE", "/seafile/contacts/"+contact.VCardUID+"/links/"+remove.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("user_id = ? AND entity_id = ?", uint(1), contact.VCardUID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, other.ID, mustLoadIdentity(t, db, other.ID).ID)

	// The unlink is scoped to the contact in the path: unlinking through a
	// different contact's vcard_uid must 404, not delete the identity.
	otherContact := models.Contact{UserID: 1, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	w3 := seafileDoJSON(t, router, "DELETE", "/seafile/contacts/"+otherContact.VCardUID+"/links/"+other.ID, nil)
	assert.Equal(t, http.StatusNotFound, w3.Code, w3.Body.String())
	assert.Equal(t, other.ID, mustLoadIdentity(t, db, other.ID).ID)
}
