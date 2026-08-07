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

// immichControllerConfig is the test config for Immich routes: a JWT secret
// (for credential encryption) and private addresses allowed (the fake server
// is on loopback; this is also the realistic self-hosted default).
func immichControllerConfig() config.Config {
	return config.Config{
		JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef",
	}
}

// immichTestRouter builds a router with the Immich controller routes wired to
// the given db, configured with immichControllerConfig.
func immichTestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", immichControllerConfig())
		c.Next()
	})
	router.GET("/immich/config", GetImmichConfig)
	router.PUT("/immich/config", withValidated(func() any { return &models.ImmichConfigInput{} }), SaveImmichConfig)
	router.DELETE("/immich/config", DeleteImmichConfig)
	router.POST("/immich/test-connection", TestImmichConnection)
	router.GET("/immich/people", ListImmichPeople)
	router.POST("/immich/sync", SyncImmichNow)
	router.POST("/immich/contacts/:vcard_uid/link", LinkImmichContact)
	router.DELETE("/immich/contacts/:vcard_uid/link", UnlinkImmichContact)
	router.GET("/immich/contacts/:vcard_uid/summary", GetImmichContactSummary)
	router.GET("/immich/contacts/:vcard_uid/thumbnail", GetImmichThumbnail)
	router.GET("/immich/contacts/:vcard_uid/assets", ListImmichContactAssets)
	router.GET("/immich/contacts/:vcard_uid/assets/:asset_id/image", GetImmichAssetImage)
	return router
}

// seedImmichControllerDB seeds the models the Immich controller touches and a
// contact for user 1.
func seedImmichControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.ImmichConfig{}, &models.ExternalIdentity{}, &models.ExternalActivity{}))
	user := models.User{Username: "immich-ctrl", Password: "password123!A", Email: "immich-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Example"}).Error)
	return db
}

func immichDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func TestGetImmichConfig_EmptyDefault(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "GET", "/immich/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp services.ImmichConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.HasAPIKey)
	assert.Empty(t, resp.BaseURL)
}

func TestSaveImmichConfig_EncryptsKeyAndHidesIt(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{
		BaseURL: "https://immich.example", APIKey: "plaintext-key",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp services.ImmichConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://immich.example", resp.BaseURL)
	assert.True(t, resp.HasAPIKey)
	assert.NotContains(t, w.Body.String(), "plaintext-key", "the API key must never appear in a response")

	// At rest it is encrypted.
	var stored models.ImmichConfig
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.NotContains(t, stored.APIKeyEncrypted, "plaintext-key")
	assert.Equal(t, "plaintext-key", mustDecrypt(t, stored.APIKeyEncrypted))

	// Empty key on update keeps the stored one.
	w2 := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{BaseURL: "https://immich.example"})
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&stored).Error)
	assert.Equal(t, "plaintext-key", mustDecrypt(t, stored.APIKeyEncrypted))
}

func mustDecrypt(t *testing.T, encrypted string) string {
	t.Helper()
	plain, err := services.DecryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", encrypted)
	require.NoError(t, err)
	return plain
}

func TestSaveImmichConfig_CreateWithoutKeyIsRejected(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{BaseURL: "https://immich.example"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestSaveImmichConfig_RejectsSchemelessBaseURL pins the fix for a real gap:
// a scheme-less base URL used to save successfully and only fail later,
// confusingly, the first time the connection was actually used. It must now
// be rejected immediately, at save time.
func TestSaveImmichConfig_RejectsSchemelessBaseURL(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{
		BaseURL: "immich.example.com", APIKey: "k",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.ImmichConfig{}).Where("user_id = ?", uint(1)).Count(&count).Error)
	assert.EqualValues(t, 0, count, "a rejected base URL must not be persisted")
}

// TestSaveImmichConfig_TrimsTrailingAPISegment pins the one deliberate
// auto-correction: a base URL ending in "/api" is saved with that segment
// stripped, since the client always appends "/api/..." itself.
func TestSaveImmichConfig_TrimsTrailingAPISegment(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "PUT", "/immich/config", models.ImmichConfigInput{
		BaseURL: "https://immich.example/api", APIKey: "k",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp services.ImmichConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://immich.example", resp.BaseURL)
}

func TestTestImmichConnection_NoConfigIs503(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	w := immichDoJSON(t, router, "POST", "/immich/test-connection", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTestImmichConnection_SuccessAndFailure(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "sekret")
	defer fake.Close()
	fake.Me = &testMe{Email: "alice@example.com"}

	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)

	w := immichDoJSON(t, router, "POST", "/immich/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result services.ImmichConnectionTestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.True(t, result.OK)
	assert.Equal(t, "ok", result.Stage)
	assert.Contains(t, result.Message, "alice@example.com")

	// A diagnosed failure (wrong key) is still HTTP 200 — the diagnosis
	// itself succeeded.
	badEnc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "wrong-key")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ImmichConfig{}).Where("user_id = ?", uint(1)).Update("api_key_encrypted", badEnc).Error)

	w2 := immichDoJSON(t, router, "POST", "/immich/test-connection", nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var failResult services.ImmichConnectionTestResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &failResult))
	assert.False(t, failResult.OK)
	assert.Equal(t, "auth", failResult.Stage)
}

func TestListImmichPeople(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "sekret")
	defer fake.Close()
	fake.addTestPerson("p1", "Alice", 5, nil, nil)
	fake.addTestPerson("p2", "Bob", 2, nil, nil)

	// Configure the connection to point at the fake.
	require.NoError(t, db.Create(&models.ImmichConfig{
		UserID: 1, BaseURL: fake.URL(),
	}).Error)
	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ImmichConfig{}).Where("user_id = ?", uint(1)).Update("api_key_encrypted", enc).Error)

	w := immichDoJSON(t, router, "GET", "/immich/people", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		People []services.ImmichPerson `json:"people"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.People, 2)
	assert.Equal(t, "sekret", fake.LastAPIKey, "the server must receive the user's API key")
}

// TestListImmichPeople_RequestFailedVsUnreachable pins T42: a stubbed 400
// response from a live Immich instance must surface a distinct message from a
// stubbed connection failure — both used to render the same generic "Could
// not reach Immich. Is the instance up?" text.
func TestListImmichPeople_RequestFailedVsUnreachable(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "sekret")
	fake.FailWithStatus = http.StatusBadRequest
	enc, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)

	w := immichDoJSON(t, router, "GET", "/immich/people", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Immich returned an error (400", w.Body.String())
	assert.NotContains(t, w.Body.String(), "Could not reach Immich", "a real response from a live instance must not read as unreachable")
	fake.Close()

	// A second connection pointed at an actually-closed server must still get
	// the original generic "unreachable" message.
	unreachable := newImmichTestServer(t, "")
	unreachableURL := unreachable.URL()
	unreachable.Close()
	enc2, err := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "k")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ImmichConfig{}).Where("user_id = ?", uint(1)).Updates(map[string]any{
		"base_url": unreachableURL, "api_key_encrypted": enc2,
	}).Error)

	w2 := immichDoJSON(t, router, "GET", "/immich/people", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w2.Code)
	assert.Contains(t, w2.Body.String(), "Could not reach Immich")
}

func TestLinkAndUnlinkImmichContact(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)
	enc, _ := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "k")
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: "https://immich.example", APIKeyEncrypted: enc}).Error)

	w := immichDoJSON(t, router, "POST", "/immich/contacts/"+contact.VCardUID+"/link", map[string]string{
		"person_id": "person-alice", "person_name": "Alice",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		ExternalIdentity models.ExternalIdentity `json:"external_identity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, services.ExternalSystemImmich, created.ExternalIdentity.System)
	assert.Equal(t, contact.VCardUID, created.ExternalIdentity.EntityID)
	assert.Equal(t, "https://immich.example/people/person-alice", created.ExternalIdentity.URL)

	// Duplicate link is a 409.
	w2 := immichDoJSON(t, router, "POST", "/immich/contacts/"+contact.VCardUID+"/link", map[string]string{
		"person_id": "person-alice", "person_name": "Alice",
	})
	assert.Equal(t, http.StatusConflict, w2.Code)

	// Summary reflects the link (no Immich configured reachable — degrades to
	// cached name).
	w3 := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/summary", nil)
	require.Equal(t, http.StatusOK, w3.Code)
	var summaryResp struct {
		Summary services.ImmichPersonSummary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &summaryResp))
	assert.Equal(t, "Alice", summaryResp.Summary.PersonName)

	// Unlink.
	w4 := immichDoJSON(t, router, "DELETE", "/immich/contacts/"+contact.VCardUID+"/link", nil)
	require.Equal(t, http.StatusOK, w4.Code)

	// Summary after unlink: null.
	w5 := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/summary", nil)
	require.Equal(t, http.StatusOK, w5.Code)
	var nullResp struct {
		Summary *services.ImmichPersonSummary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(w5.Body.Bytes(), &nullResp))
	assert.Nil(t, nullResp.Summary)
}

func TestLinkImmichContact_RejectsForeignContact(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	otherContact := models.Contact{UserID: 999, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&otherContact).Error)

	w := immichDoJSON(t, router, "POST", "/immich/contacts/"+otherContact.VCardUID+"/link", map[string]string{
		"person_id": "person-1", "person_name": "Nope",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSyncImmichNow_EndToEnd(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "sekret")
	defer fake.Close()
	fake.addTestPerson("person-alice", "Alice", 2, []testAsset{
		{ID: "a1", FileCreatedAt: "2026-08-03T10:00:00Z"},
	}, nil)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	enc, _ := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)
	require.NoError(t, db.Create(&models.ExternalIdentity{
		UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemImmich, ExternalID: "person-alice",
		URL: fake.URL() + "/people/person-alice", Metadata: map[string]interface{}{"person_name": "Alice"},
	}).Error)

	w := immichDoJSON(t, router, "POST", "/immich/sync", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var activities []models.ExternalActivity
	require.NoError(t, db.Where("user_id = ? AND source_system = ?", uint(1), services.ExternalSystemImmich).Find(&activities).Error)
	require.Len(t, activities, 1)
	assert.Equal(t, "a1", activities[0].ExternalID)
	assert.Equal(t, contact.VCardUID, activities[0].EntityID)
}

func TestGetImmichThumbnail(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "")
	defer fake.Close()
	fake.addTestPerson("person-alice", "Alice", 1, nil, []byte{0xff, 0xd8, 0xff, 0xe0})

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	enc, _ := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "k")
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)
	require.NoError(t, db.Create(&models.ExternalIdentity{
		UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemImmich, ExternalID: "person-alice",
	}).Error)

	w := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/thumbnail", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Equal(t, []byte{0xff, 0xd8, 0xff, 0xe0}, w.Body.Bytes())
}

func TestListImmichContactAssets(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "sekret")
	defer fake.Close()
	fake.addTestPerson("person-alice", "Alice", 2, []testAsset{
		{ID: "asset-old", FileCreatedAt: "2026-01-01T10:00:00Z"},
		{ID: "asset-new", FileCreatedAt: "2026-08-03T10:00:00Z"},
	}, nil)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	enc, _ := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "sekret")
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)
	require.NoError(t, db.Create(&models.ExternalIdentity{
		UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemImmich, ExternalID: "person-alice",
	}).Error)

	w := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/assets", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Assets []services.ImmichAssetSummary `json:"assets"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Assets, 2)
	assert.Equal(t, "asset-new", resp.Assets[0].ID, "newest first")
}

func TestListImmichContactAssets_NoLinkIs404(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	w := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/assets", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetImmichAssetImage(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	fake := newImmichTestServer(t, "")
	defer fake.Close()
	fake.addTestAssetThumbnail("asset-1", []byte{0xff, 0xd8, 0xff, 0xcc})

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	enc, _ := services.EncryptCredential("test-jwt-secret-0123456789abcdef0123456789abcdef", "k")
	require.NoError(t, db.Create(&models.ImmichConfig{UserID: 1, BaseURL: fake.URL(), APIKeyEncrypted: enc}).Error)
	require.NoError(t, db.Create(&models.ExternalIdentity{
		UserID: 1, EntityID: contact.VCardUID, System: services.ExternalSystemImmich, ExternalID: "person-alice",
	}).Error)

	w := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/assets/asset-1/image", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))
	assert.Equal(t, []byte{0xff, 0xd8, 0xff, 0xcc}, w.Body.Bytes())
}

func TestGetImmichAssetImage_NoLinkIs404(t *testing.T) {
	db := seedImmichControllerDB(t)
	router := immichTestRouter(t, db)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", uint(1)).First(&contact).Error)

	w := immichDoJSON(t, router, "GET", "/immich/contacts/"+contact.VCardUID+"/assets/asset-1/image", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
