package controllers

// Coverage tests for milestone-30 issue #1623 §3: the low-coverage
// validation-rejection, SSRF-guard and ownership error paths in
// webdav_controller.go. Kept in a separate file so the focused existing suite
// (webdav_controller_test.go) stays untouched.

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// m30CovJWTSecret matches the fake secret webdavControllerConfig() seeds, so
// credentials encrypted here decrypt in the handler.
const m30CovJWTSecret = "test-jwt-secret-0123456789abcdef0123456789abcdef"

// m30CovContext installs the middleware values every REST handler reads
// (db/userID/cfg), with userID omitted when 0 so the currentUserID !ok arm can
// be exercised.
func m30CovContext(db *gorm.DB, userID uint, cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Set("cfg", cfg)
		c.Next()
	}
}

// m30InjectValidated stands in for middleware.ValidateJSONMiddleware when a
// test needs to reach the handler with a value that bypasses struct-tag
// validation (the service-level normalization branch).
func m30InjectValidated(input any) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("validated", input)
		c.Next()
	}
}

func m30DoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func m30DoRaw(t *testing.T, router *gin.Engine, method, path, raw string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(method, path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// m30ClosedPortURL returns an http://127.0.0.1:<port> URL whose port was bound
// and immediately released, so a connection attempt is refused rather than
// hanging.
func m30ClosedPortURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "http://" + addr
}

func m30SeedEncrypted(t *testing.T, plaintext string) string {
	t.Helper()
	enc, err := services.EncryptCredential(m30CovJWTSecret, plaintext)
	require.NoError(t, err)
	return enc
}

func m30SeedWebDAVUser(t *testing.T, db *gorm.DB, tag string) models.User {
	t.Helper()
	user := models.User{
		Username: "webdav-cov-" + tag,
		Password: "password123!A",
		Email:    "webdav-cov-" + tag + "@example.com",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NotZero(t, user.ID)
	return user
}

func m30SeedWebDAVConfig(t *testing.T, db *gorm.DB, userID uint, baseURL, username, secret string) {
	t.Helper()
	require.NoError(t, db.Create(&models.WebDAVConfig{
		UserID: userID, BaseURL: baseURL, Username: username,
		AppPasswordEncrypted: m30SeedEncrypted(t, secret),
	}).Error)
}

func m30WebDAVRouter(t *testing.T, db *gorm.DB, userID uint, cfg config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(m30CovContext(db, userID, cfg))
	router.GET("/nextcloud/config", GetWebDAVConfig)
	router.PUT("/nextcloud/config", withValidated(func() any { return &models.WebDAVConfigInput{} }), SaveWebDAVConfig)
	router.DELETE("/nextcloud/config", DeleteWebDAVConfig)
	router.POST("/nextcloud/test-connection", TestWebDAVConnection)
	router.GET("/nextcloud/dir", ListWebDAVDir)
	router.POST("/nextcloud/contacts/:vcard_uid/link", LinkWebDAVContact)
	router.DELETE("/nextcloud/contacts/:vcard_uid/links/:identity_id", UnlinkWebDAVContact)
	return router
}

// TestWebDAVCov_SaveMissingValidatedContext exercises the GetValidated error
// arm: a request that reaches the handler with no validated payload is a 400.
func TestWebDAVCov_SaveMissingValidatedContext(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(m30CovContext(db, 1, webdavControllerConfig()))
	router.PUT("/nextcloud/config", SaveWebDAVConfig)

	w := m30DoJSON(t, router, "PUT", "/nextcloud/config", map[string]any{"base_url": "https://nc.example"})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "request")
}

// TestWebDAVCov_SaveInvalidURLIsValidation exercises the service-level
// NormalizeWebDAVBaseURL rejection path (the DTO skips struct validation here
// on purpose so the service branch is reached) and the
// abortWebDAVServiceError ErrWebDAVInvalidURL arm.
func TestWebDAVCov_SaveInvalidURLIsValidation(t *testing.T) {
	db := dbtest.New(t)
	router := gin.New()
	router.Use(m30CovContext(db, 1, webdavControllerConfig()))
	router.PUT("/nextcloud/config", m30InjectValidated(&models.WebDAVConfigInput{
		BaseURL: "not-a-valid-url", Username: "alice", AppPassword: "pass",
	}), SaveWebDAVConfig)

	w := m30DoJSON(t, router, "PUT", "/nextcloud/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeValidation, "")
}

// TestWebDAVCov_SaveOriginChangeWithoutPassword exercises the
// credential-exfiltration guard: the base URL moves to a new origin but no
// password is supplied, so the save is rejected (400 INVALID_INPUT) and the
// stored config is left untouched.
func TestWebDAVCov_SaveOriginChangeWithoutPassword(t *testing.T) {
	db := dbtest.New(t)
	user := m30SeedWebDAVUser(t, db, "origin")
	m30SeedWebDAVConfig(t, db, user.ID, "https://nc.example", "alice", "stored-secret")

	router := gin.New()
	router.Use(m30CovContext(db, user.ID, webdavControllerConfig()))
	router.PUT("/nextcloud/config", m30InjectValidated(&models.WebDAVConfigInput{
		BaseURL: "https://other.example", Username: "alice",
	}), SaveWebDAVConfig)

	w := m30DoJSON(t, router, "PUT", "/nextcloud/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "app_password")

	var stored models.WebDAVConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&stored).Error)
	assert.Equal(t, "https://nc.example", stored.BaseURL, "a rejected origin change must not persist")
}

// TestWebDAVCov_SaveGetConfigDBError exercises the SaveWebDAVConfig database
// branch: an unreadable config table surfaces as a 500 DATABASE_ERROR.
func TestWebDAVCov_SaveGetConfigDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "webdav_configs")

	router := gin.New()
	router.Use(m30CovContext(db, 1, webdavControllerConfig()))
	router.PUT("/nextcloud/config", m30InjectValidated(&models.WebDAVConfigInput{
		BaseURL: "https://nc.example", Username: "alice", AppPassword: "pass",
	}), SaveWebDAVConfig)

	w := m30DoJSON(t, router, "PUT", "/nextcloud/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestWebDAVCov_GetConfigPopulatedAndOwnership pins the populated success
// response and cross-user isolation: user A's config is never visible to user
// B, who sees the empty default instead.
func TestWebDAVCov_GetConfigPopulatedAndOwnership(t *testing.T) {
	db := dbtest.New(t)
	userA := m30SeedWebDAVUser(t, db, "a")
	userB := m30SeedWebDAVUser(t, db, "b")
	m30SeedWebDAVConfig(t, db, userA.ID, "https://nc.example", "alice", "sekret")

	wA := m30DoJSON(t, m30WebDAVRouter(t, db, userA.ID, webdavControllerConfig()), "GET", "/nextcloud/config", nil)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	var respA services.WebDAVConfigResponse
	require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &respA))
	assert.Equal(t, "https://nc.example", respA.BaseURL)
	assert.Equal(t, "alice", respA.Username)
	assert.True(t, respA.HasAppPassword)

	wB := m30DoJSON(t, m30WebDAVRouter(t, db, userB.ID, webdavControllerConfig()), "GET", "/nextcloud/config", nil)
	require.Equal(t, http.StatusOK, wB.Code, wB.Body.String())
	var respB services.WebDAVConfigResponse
	require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &respB))
	assert.Empty(t, respB.BaseURL, "user B must not see user A's Nextcloud config")
	assert.False(t, respB.HasAppPassword)
}

// TestWebDAVCov_GetConfigDBError exercises the retrieval-failure branch of
// GetWebDAVConfig.
func TestWebDAVCov_GetConfigDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "webdav_configs")

	w := m30DoJSON(t, m30WebDAVRouter(t, db, 1, webdavControllerConfig()), "GET", "/nextcloud/config", nil)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestWebDAVCov_DeleteOwnershipIsolated proves a delete scoped to the caller
// never removes another user's connection.
func TestWebDAVCov_DeleteOwnershipIsolated(t *testing.T) {
	db := dbtest.New(t)
	userA := m30SeedWebDAVUser(t, db, "a")
	userB := m30SeedWebDAVUser(t, db, "b")
	m30SeedWebDAVConfig(t, db, userA.ID, "https://nc.example", "alice", "sekret")

	w := m30DoJSON(t, m30WebDAVRouter(t, db, userB.ID, webdavControllerConfig()), "DELETE", "/nextcloud/config", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.WebDAVConfig{}).Where("user_id = ?", userA.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "user A's config must survive user B's delete")
}

// TestWebDAVCov_DeleteDBError exercises the delete-failure branch.
func TestWebDAVCov_DeleteDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "webdav_configs")

	w := m30DoJSON(t, m30WebDAVRouter(t, db, 1, webdavControllerConfig()), "DELETE", "/nextcloud/config", nil)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestWebDAVCov_ServiceErrorMapping drives abortWebDAVServiceError's not-found,
// redirect, request-failed, invalid-data and unreachable arms through
// ListWebDAVDir and asserts the mapped status + error code.
func TestWebDAVCov_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		failStatus int
		wantStatus int
		wantCode   string
	}{
		{"not_found", http.StatusNotFound, http.StatusNotFound, apperrors.ErrCodeNotFound},
		{"redirect", http.StatusTemporaryRedirect, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
		{"request_failed", http.StatusInternalServerError, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
		{"invalid_data", http.StatusMultiStatus, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := seedWebDAVControllerDB(t)
			router := webdavTestRouter(t, db)

			fake := newWebDAVTestServer(t, "testuser", "sekret")
			defer fake.Close()
			fake.FailWithStatus = tc.failStatus
			connectWebDAVControllerConfig(t, db, fake.URL())

			w := webdavDoJSON(t, router, "GET", "/nextcloud/dir", nil)
			mzErrorCode(t, w, tc.wantStatus, tc.wantCode)
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		db := seedWebDAVControllerDB(t)
		router := webdavTestRouter(t, db)
		connectWebDAVControllerConfig(t, db, m30ClosedPortURL(t))

		w := webdavDoJSON(t, router, "GET", "/nextcloud/dir", nil)
		mzErrorCode(t, w, http.StatusServiceUnavailable, apperrors.ErrCodeExternal)
	})
}

// TestWebDAVCov_PrivateURLGuard proves the SSRF guard: with
// WEBDAV_BLOCK_PRIVATE_URLS on, a loopback base URL is refused and the request
// never reaches the listening server -- via both the browse and the
// test-connection endpoints.
func TestWebDAVCov_PrivateURLGuard(t *testing.T) {
	var hits int32
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer listener.Close()

	cfg := webdavControllerConfig()
	cfg.WebDAVBlockPrivateURLs = true

	db := seedWebDAVControllerDB(t)
	m30SeedWebDAVConfig(t, db, 1, listener.URL, "testuser", "sekret")
	router := m30WebDAVRouter(t, db, 1, cfg)

	w := m30DoJSON(t, router, "GET", "/nextcloud/dir", nil)
	mzErrorCode(t, w, http.StatusServiceUnavailable, apperrors.ErrCodeExternal)

	// Test connection diagnoses the blocked address instead of erroring (200,
	// ok:false), and still must not dial.
	wTest := m30DoJSON(t, router, "POST", "/nextcloud/test-connection", nil)
	require.Equal(t, http.StatusOK, wTest.Code, wTest.Body.String())
	var result services.WebDAVConnectionTestResult
	require.NoError(t, json.Unmarshal(wTest.Body.Bytes(), &result))
	assert.False(t, result.OK)

	assert.Equal(t, int32(0), atomic.LoadInt32(&hits),
		"the SSRF guard must prevent the request from ever reaching the loopback listener")
}

// TestWebDAVCov_LinkValidationRejected covers the malformed-body, struct-tag
// and post-trim empty-path rejections of LinkWebDAVContact.
func TestWebDAVCov_LinkValidationRejected(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)
	base := "/nextcloud/contacts/" + contact.VCardUID + "/link"

	// Malformed JSON body.
	wBadJSON := m30DoRaw(t, router, "POST", base, "{not json")
	assertAppErrorResponse(t, wBadJSON, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "request body")

	// Missing required fields (path/name/type).
	wMissing := webdavDoJSON(t, router, "POST", base, map[string]any{"name": "contract.pdf"})
	mzErrorCode(t, wMissing, http.StatusBadRequest, apperrors.ErrCodeValidation)

	// Invalid enum value for type.
	wType := webdavDoJSON(t, router, "POST", base, map[string]any{
		"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "bogus",
	})
	mzErrorCode(t, wType, http.StatusBadRequest, apperrors.ErrCodeValidation)

	// Present but whitespace-only path (passes min=1 length, fails the trim).
	wBlank := webdavDoJSON(t, router, "POST", base, map[string]any{
		"path": "   ", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, wBlank, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "path")
}

// TestWebDAVCov_LinkLookupDBError exercises the non-ErrRecordNotFound lookup
// branch: an unreadable external_identities table is a 500 DATABASE_ERROR.
func TestWebDAVCov_LinkLookupDBError(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)
	dbtest.HideTable(t, db, "external_identities")

	w := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+contact.VCardUID+"/link", map[string]any{
		"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestWebDAVCov_LinkForeignContactNotFound pins the ownership rejection's
// specific NOT_FOUND code.
func TestWebDAVCov_LinkForeignContactNotFound(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	other := models.User{Username: "webdav-cov-foreign", Password: "password123!A", Email: "webdav-cov-foreign@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&foreign).Error)

	w := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+foreign.VCardUID+"/link", map[string]any{
		"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")
}

// TestWebDAVCov_UnlinkCrossUserAndDBError covers the not-found (cross-user) and
// database branches of UnlinkWebDAVContact.
func TestWebDAVCov_UnlinkCrossUserAndDBError(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	other := models.User{Username: "webdav-cov-owner", Password: "password123!A", Email: "webdav-cov-owner@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreignIdentity := models.ExternalIdentity{
		UserID: other.ID, EntityID: contact.VCardUID,
		System: services.ExternalSystemWebDAV, ExternalID: "/secret.pdf",
	}
	require.NoError(t, db.Create(&foreignIdentity).Error)

	// The default test router authenticates as user 1; the identity belongs to
	// another user, so it must be a 404 and must survive.
	router := webdavTestRouter(t, db)
	w := webdavDoJSON(t, router, "DELETE", "/nextcloud/contacts/"+contact.VCardUID+"/links/"+foreignIdentity.ID, nil)
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")
	assert.Equal(t, foreignIdentity.ID, mustLoadIdentity(t, db, foreignIdentity.ID).ID)

	// Unreadable identity table -> 500 DATABASE_ERROR.
	dbtest.HideTable(t, db, "external_identities")
	wDB := webdavDoJSON(t, router, "DELETE", "/nextcloud/contacts/"+contact.VCardUID+"/links/"+foreignIdentity.ID, nil)
	assertAppErrorResponse(t, wDB, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestWebDAVCov_Unauthenticated covers every handler's currentUserID !ok arm:
// a missing authenticated user is a 401 UNAUTHORIZED.
func TestWebDAVCov_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	router := m30WebDAVRouter(t, db, 0, webdavControllerConfig())
	router.PUT("/nextcloud/config/direct", m30InjectValidated(&models.WebDAVConfigInput{
		BaseURL: "https://nc.example", Username: "alice", AppPassword: "pass",
	}), SaveWebDAVConfig)

	cases := []struct {
		name, method, path string
	}{
		{"get_config", "GET", "/nextcloud/config"},
		{"save_config", "PUT", "/nextcloud/config/direct"},
		{"delete_config", "DELETE", "/nextcloud/config"},
		{"test_connection", "POST", "/nextcloud/test-connection"},
		{"list_dir", "GET", "/nextcloud/dir"},
		{"link", "POST", "/nextcloud/contacts/vcard-uid/link"},
		{"unlink", "DELETE", "/nextcloud/contacts/vcard-uid/links/identity-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := m30DoJSON(t, router, tc.method, tc.path, nil)
			assertAppErrorResponse(t, w, http.StatusUnauthorized, apperrors.ErrCodeUnauthorized, "")
		})
	}
}

// TestWebDAVCov_LinkNoConfigIsValidation covers the service-error arm of
// LinkWebDAVContact: linking without a configured connection is the caller's
// own setup, so it is a 400 VALIDATION_ERROR (not a 5xx).
func TestWebDAVCov_LinkNoConfigIsValidation(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	w := webdavDoJSON(t, router, "POST", "/nextcloud/contacts/"+contact.VCardUID+"/link", map[string]any{
		"path": "/Documents/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeValidation, "")
}

// TestWebDAVCov_ListDirNormalizesBarePath covers the path-without-leading-slash
// normalization branch of ListWebDAVDir.
func TestWebDAVCov_ListDirNormalizesBarePath(t *testing.T) {
	db := seedWebDAVControllerDB(t)
	router := webdavTestRouter(t, db)

	fake := newWebDAVTestServer(t, "testuser", "sekret")
	defer fake.Close()
	seedWebDAVFake(fake)
	connectWebDAVControllerConfig(t, db, fake.URL())

	w := webdavDoJSON(t, router, "GET", "/nextcloud/dir?path=Documents", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Items []services.WebDAVItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "contract.pdf", resp.Items[0].Name)
}
