package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func webdavControllerConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

func webdavTestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", webdavControllerConfig())
		c.Next()
	})
	router.GET("/nextcloud/config", GetWebDAVConfig)
	router.PUT("/nextcloud/config", withValidated(func() any { return &models.WebDAVConfigInput{} }), SaveWebDAVConfig)
	router.DELETE("/nextcloud/config", DeleteWebDAVConfig)
	router.POST("/nextcloud/test-connection", TestWebDAVConnection)
	router.GET("/nextcloud/dir", ListWebDAVDir)
	router.POST("/nextcloud/contacts/:vcard_uid/link", LinkWebDAVContact)
	router.DELETE("/nextcloud/contacts/:vcard_uid/links/:identity_id", UnlinkWebDAVContact)
	return router
}

func seedWebDAVControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.WebDAVConfig{}, &models.ExternalIdentity{}))
	user := models.User{Username: "webdav-ctrl", Password: "password123!A", Email: "webdav-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Example"}).Error)
	return db
}

func webdavDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func connectWebDAVControllerConfig(t *testing.T, db *gorm.DB, baseURL string) {
	t.Helper()
	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.WebDAVConfig{UserID: 1, BaseURL: baseURL, Username: "testuser", AppPasswordEncrypted: enc}).Error)
}

// seedWebDAVFake populates the fake with a root, a Documents folder, and a
// file inside it.
func seedWebDAVFake(fake *fakeWebDAVController) {
	rootHref := strings.TrimRight("/remote.php/dav/files/testuser/", "/")
	modified := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC1123)
	fake.Items[rootHref] = &fakeWebDAVItem{Name: "testuser", IsDir: true}
	fake.Items["/remote.php/dav/files/testuser/Documents"] = &fakeWebDAVItem{Name: "Documents", IsDir: true, Modified: modified}
	fake.Items["/remote.php/dav/files/testuser/Documents/contract.pdf"] = &fakeWebDAVItem{Name: "contract.pdf", IsDir: false, Size: 4096, Modified: modified, FileID: "00000123"}
}

func TestGetWebDAVConfig_EmptyDefault(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	w := webdavDoJSON(t, router, "GET", "/nextcloud/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp services.WebDAVConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.HasAppPassword)
	assert.Empty(t, resp.BaseURL)
	assert.Empty(t, resp.Username)
}

func TestSaveWebDAVConfig_EncryptsPasswordAndHidesIt(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	w := webdavDoJSON(t, router, "PUT", "/nextcloud/config", models.WebDAVConfigInput{
		BaseURL: "https://nc.example", Username: "alice", AppPassword: "plaintext-pass",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp services.WebDAVConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://nc.example", resp.BaseURL)
	assert.Equal(t, "alice", resp.Username)
	assert.True(t, resp.HasAppPassword)
	assert.NotContains(t, w.Body.String(), "plaintext-pass", "the app password must never appear in a response")

	var stored models.WebDAVConfig
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.Equal(t, "plaintext-pass", mustDecrypt(t, stored.AppPasswordEncrypted))
}

func TestSaveWebDAVConfig_CreateWithoutPasswordIsRejected(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	w := webdavDoJSON(t, router, "PUT", "/nextcloud/config", models.WebDAVConfigInput{BaseURL: "https://nc.example", Username: "alice"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTestWebDAVConnection_SuccessAndFailure(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	fake := newWebDAVTestServer(t, "testuser", "sekret")
	defer fake.Close()
	seedWebDAVFake(fake)

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.WebDAVConfig{UserID: 1, BaseURL: fake.URL(), Username: "testuser", AppPasswordEncrypted: enc}).Error)

	w := webdavDoJSON(t, router, "POST", "/nextcloud/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result services.WebDAVConnectionTestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.True(t, result.OK)
	assert.Equal(t, "testuser", fake.LastUser)
	assert.Equal(t, "sekret", fake.LastPass)

	// A diagnosed auth failure (wrong app password) is still HTTP 200.
	encBad, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.WebDAVConfig{}).Where("user_id = ?", uint(1)).Update("app_password_encrypted", encBad).Error)

	w2 := webdavDoJSON(t, router, "POST", "/nextcloud/test-connection", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var failed services.WebDAVConnectionTestResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &failed))
	assert.False(t, failed.OK)
}

func TestListWebDAVDir_BrowsesFake(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	fake := newWebDAVTestServer(t, "testuser", "sekret")
	defer fake.Close()
	seedWebDAVFake(fake)
	connectWebDAVControllerConfig(t, db, fake.URL())

	// Root listing: the Documents folder (the root itself is excluded).
	w := webdavDoJSON(t, router, "GET", "/nextcloud/dir", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Items []services.WebDAVItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Documents", resp.Items[0].Name)
	assert.Equal(t, "dir", resp.Items[0].Type)
	assert.Equal(t, "/Documents/", resp.Items[0].Path)

	// Inside Documents: the file.
	w2 := webdavDoJSON(t, router, "GET", "/nextcloud/dir?path=/Documents", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "contract.pdf", resp.Items[0].Name)
	assert.Equal(t, "file", resp.Items[0].Type)
	assert.EqualValues(t, 4096, resp.Items[0].Size)
	assert.Equal(t, "/Documents/contract.pdf", resp.Items[0].Path)
	assert.Equal(t, "00000123", resp.Items[0].FileID)
	assert.NotEmpty(t, resp.Items[0].ModifiedAt)
}

func TestLinkWebDAVContact_StoresPathAndDeepLink(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	fake := newWebDAVTestServer(t, "testuser", "sekret")
	defer fake.Close()
	connectWebDAVControllerConfig(t, db, fake.URL())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	w := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+contact.VCardUID+"/link", map[string]any{
		"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file",
		"size": 4096, "file_id": "00000123",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Identity models.ExternalIdentity `json:"external_identity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, services.ExternalSystemWebDAV, resp.Identity.System)
	assert.Equal(t, "/Documents/contract.pdf", resp.Identity.ExternalID)
	// A file with a file id deep-links with openfile.
	assert.Contains(t, resp.Identity.URL, "/apps/files/?dir=%2FDocuments&openfile=00000123")
	assert.Equal(t, "contract.pdf", resp.Identity.Metadata["name"])
}

func TestLinkWebDAVContact_DuplicateIsConflictAndForeignIs404(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	fake := newWebDAVTestServer(t, "testuser", "sekret")
	defer fake.Close()
	connectWebDAVControllerConfig(t, db, fake.URL())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	body := map[string]any{"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file"}
	first := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+contact.VCardUID+"/link", body)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	second := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+contact.VCardUID+"/link", body)
	assert.Equal(t, http.StatusConflict, second.Code)

	other := models.User{Username: "other", Password: "password123!A", Email: "other3@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&foreign).Error)
	wForeign := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+foreign.VCardUID+"/link", body)
	assert.Equal(t, http.StatusNotFound, wForeign.Code)
}

func TestUnlinkWebDAVContact_RemovesOnlyThatIdentity(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	remove := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemWebDAV, ExternalID: "/a.pdf"}
	other := models.ExternalIdentity{UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemWebDAV, ExternalID: "/b.pdf"}
	require.NoError(t, db.Create(&remove).Error)
	require.NoError(t, db.Create(&other).Error)

	w := webdavDoJSON(t, router, "DELETE", "/nextcloud/contacts/"+contact.VCardUID+"/links/"+remove.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("user_id = ? AND entity_id = ?", uint(1), contact.VCardUID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Equal(t, other.ID, mustLoadIdentity(t, db, other.ID).ID)

	// The unlink is scoped to the contact in the path: unlinking through a
	// different contact's vcard_uid must 404, not delete the identity.
	otherContact := models.Contact{UserID: 1, Firstname: "Bob"}
	require.NoError(t, db.Create(&otherContact).Error)
	w3 := webdavDoJSON(t, router, "DELETE", "/nextcloud/contacts/"+otherContact.VCardUID+"/links/"+other.ID, nil)
	assert.Equal(t, http.StatusNotFound, w3.Code, w3.Body.String())
	assert.Equal(t, other.ID, mustLoadIdentity(t, db, other.ID).ID)
}
