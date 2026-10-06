package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// geopulseTestSecret is deliberately low-entropy filler (see
// services/geopulse_service_test.go) — not secret-shaped, so gitleaks stays quiet.
const geopulseTestSecret = "geopulse-test-filler-geopulse-test-filler"

// geopulseTestRouter wires the GeoPulse routes plus POST /activities (the
// confirm path) to a real migrated DB. user is the authenticated user.
func geopulseTestRouter(t *testing.T, db *gorm.DB, userID uint, cfg config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	if cfg.JWTSecretKey == "" {
		cfg.JWTSecretKey = geopulseTestSecret
	}
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Set("cfg", cfg)
		c.Next()
	})
	router.GET("/geopulse/config", GetGeoPulseConfig)
	router.PUT("/geopulse/config", withValidated(func() any { return &models.GeoPulseConfigInput{} }), SaveGeoPulseConfig)
	router.DELETE("/geopulse/config", DeleteGeoPulseConfig)
	router.POST("/geopulse/test-connection", TestGeoPulseConnection)
	router.GET("/geopulse/suggestions", GetGeoPulseSuggestions)
	router.POST("/activities", withValidated(func() any { return &models.ActivityInput{} }), CreateActivity)
	return router
}

func seedGeoPulseControllerDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "geopulse-ctrl", Password: "password123!A", Email: "geopulse-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func geopulseDo(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func connectGeoPulseController(t *testing.T, router *gin.Engine, baseURL string) {
	t.Helper()
	w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: baseURL, APIKey: "the-key"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestGeoPulseConfig_EmptyDefaultSaveHideDelete(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})

	w := geopulseDo(t, router, "GET", "/geopulse/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var empty services.GeoPulseConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &empty))
	assert.False(t, empty.HasAPIKey)
	assert.Empty(t, empty.BaseURL)

	w = geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://gp.example/", APIKey: "plaintext-key"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var saved services.GeoPulseConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saved))
	assert.Equal(t, "https://gp.example", saved.BaseURL)
	assert.True(t, saved.HasAPIKey)
	assert.NotContains(t, w.Body.String(), "plaintext-key", "the API key must never appear in a response")

	var stored models.GeoPulseConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&stored).Error)
	assert.NotContains(t, stored.APIKeyEncrypted, "plaintext-key")
	plain, err := services.DecryptCredential(geopulseTestSecret, stored.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "plaintext-key", plain)

	w = geopulseDo(t, router, "GET", "/geopulse/config", nil)
	assert.NotContains(t, w.Body.String(), "plaintext-key")
	assert.Contains(t, w.Body.String(), `"has_api_key":true`)

	// Empty key on a same-origin update keeps the stored one.
	w = geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://gp.example/geopulse"})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&stored).Error)
	plain, _ = services.DecryptCredential(geopulseTestSecret, stored.APIKeyEncrypted)
	assert.Equal(t, "plaintext-key", plain)

	w = geopulseDo(t, router, "DELETE", "/geopulse/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = geopulseDo(t, router, "GET", "/geopulse/config", nil)
	assert.Contains(t, w.Body.String(), `"has_api_key":false`)

	// Delete → re-create works on the real schema (the partial unique index).
	connectGeoPulseController(t, router, "https://gp.example")
}

// A different origin with no token is a 400 on api_key and leaves the stored
// config (URL and token) untouched.
func TestSaveGeoPulseConfig_OriginChangeRequiresToken(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	connectGeoPulseController(t, router, "https://gp.example")

	w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://attacker.example"})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "api_key")
	assert.Contains(t, w.Body.String(), "re-enter the API token")

	var stored models.GeoPulseConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&stored).Error)
	assert.Equal(t, "https://gp.example", stored.BaseURL)

	w = geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://other.example", APIKey: "new-key"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestSaveGeoPulseConfig_Rejections(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})

	w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://gp.example"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "create without a key")

	w = geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "gp.example.com", APIKey: "k"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "scheme-less base URL: "+w.Body.String())

	var n int64
	require.NoError(t, db.Model(&models.GeoPulseConfig{}).Count(&n).Error)
	assert.Zero(t, n, "a rejected save must not persist anything")
}

func TestGeoPulseConfig_PerUserIsolation(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	other := models.User{Username: "geopulse-other", Password: "password123!A", Email: "geopulse-other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	mine := geopulseTestRouter(t, db, user.ID, config.Config{})
	theirs := geopulseTestRouter(t, db, other.ID, config.Config{})
	connectGeoPulseController(t, mine, "https://mine.example")

	w := geopulseDo(t, theirs, "GET", "/geopulse/config", nil)
	assert.Contains(t, w.Body.String(), `"has_api_key":false`, "another user must not see my connection")
	w = geopulseDo(t, theirs, "DELETE", "/geopulse/config", nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = geopulseDo(t, mine, "GET", "/geopulse/config", nil)
	assert.Contains(t, w.Body.String(), "mine.example", "their delete must not remove mine")
}

func TestTestGeoPulseConnection_Controller(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})

	w := geopulseDo(t, router, "POST", "/geopulse/test-connection", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, "no connection is the caller's setup, not a 5xx (issue #524)")

	f := newGeoPulseTestServer(t, "the-key")
	connectGeoPulseController(t, router, f.URL())
	w = geopulseDo(t, router, "POST", "/geopulse/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var res services.GeoPulseConnectionTestResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.True(t, res.OK)

	f.Key = "rotated"
	w = geopulseDo(t, router, "POST", "/geopulse/test-connection", nil)
	require.Equal(t, http.StatusOK, w.Code, "a diagnosed upstream failure is still a 200")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.False(t, res.OK)
	assert.Equal(t, "auth", res.Stage)
}

func TestGetGeoPulseSuggestions_Controller(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})

	// Validation + setup errors.
	assert.Equal(t, http.StatusBadRequest, geopulseDo(t, router, "GET", "/geopulse/suggestions", nil).Code, "date is required")
	assert.Equal(t, http.StatusBadRequest, geopulseDo(t, router, "GET", "/geopulse/suggestions?date=garbage", nil).Code)
	assert.Equal(t, http.StatusBadRequest, geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20&timezone=Nope/Zone", nil).Code)
	assert.Equal(t, http.StatusBadRequest, geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil).Code, "no connection configured")

	f := newGeoPulseTestServer(t, "the-key")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe Nero", "Leeds", 53.8, -1.55)
	f.Photos = []map[string]any{{"id": "p1", "originalFileName": "IMG_1.jpg", "takenAt": "2026-09-20T14:05:00Z"}}
	connectGeoPulseController(t, router, f.URL())

	w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20&timezone=UTC", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	sugs := raw["suggestions"].([]any)
	require.Len(t, sugs, 1)
	s := sugs[0].(map[string]any)
	assert.Equal(t, "geopulse:stay:7", s["external_ref"])
	assert.Equal(t, "Cafe Nero", s["location"])
	assert.Len(t, s["photos"], 1)
	assert.Equal(t, false, s["photos_unavailable"])
	assert.NotContains(t, s, "existing_activity_id")
	assert.NotContains(t, w.Body.String(), "the-key")

	// Empty day: the raw JSON is [], never null (frontend trap #8).
	f.Stays = nil
	w = geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-21", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"suggestions":[]`)

	// A photo failure degrades; the photos field stays an array.
	f.addStay(8, "2026-09-22T10:00:00Z", "Pub", "York", 53.9, -1.0)
	f.FailPhotos = http.StatusInternalServerError
	w = geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-22", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"photos":[]`)
	assert.Contains(t, w.Body.String(), `"photos_unavailable":true`)
}

// TestSaveGeoPulseConfig_MissingValidatedInput: if the validation middleware did
// not run, the handler refuses rather than saving an empty connection.
func TestSaveGeoPulseConfig_MissingValidatedInput(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{JWTSecretKey: geopulseTestSecret})
		c.Next()
	})
	router.PUT("/geopulse/config", SaveGeoPulseConfig)
	w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://gp.example", APIKey: "k"})
	assert.GreaterOrEqual(t, w.Code, 400)
	var n int64
	require.NoError(t, db.Model(&models.GeoPulseConfig{}).Count(&n).Error)
	assert.Zero(t, n)
}

// TestGeoPulse_RequiresAuthenticatedUser: with no authenticated user on the
// context every handler refuses with 401 and touches nothing.
func TestGeoPulse_RequiresAuthenticatedUser(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", config.Config{JWTSecretKey: geopulseTestSecret})
		c.Next()
	})
	router.GET("/geopulse/config", GetGeoPulseConfig)
	router.PUT("/geopulse/config", withValidated(func() any { return &models.GeoPulseConfigInput{} }), SaveGeoPulseConfig)
	router.DELETE("/geopulse/config", DeleteGeoPulseConfig)
	router.POST("/geopulse/test-connection", TestGeoPulseConnection)
	router.GET("/geopulse/suggestions", GetGeoPulseSuggestions)

	for _, r := range []struct{ method, path string }{
		{"GET", "/geopulse/config"}, {"PUT", "/geopulse/config"}, {"DELETE", "/geopulse/config"},
		{"POST", "/geopulse/test-connection"}, {"GET", "/geopulse/suggestions?date=2026-09-20"},
	} {
		body := any(nil)
		if r.method == "PUT" {
			body = models.GeoPulseConfigInput{BaseURL: "https://gp.example", APIKey: "k"}
		}
		w := geopulseDo(t, router, r.method, r.path, body)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", r.method, r.path)
	}
	var n int64
	require.NoError(t, db.Model(&models.GeoPulseConfig{}).Count(&n).Error)
	assert.Zero(t, n)
}

// TestAbortGeoPulseServiceError_StatusMapping pins every sentinel's HTTP mapping.
func TestAbortGeoPulseServiceError_StatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int // what the fake GeoPulse does
		raw    string
		want   int
	}{
		{"401 from GeoPulse → 400 (caller's credentials)", http.StatusUnauthorized, "", http.StatusBadRequest},
		{"404 from GeoPulse → 404", http.StatusNotFound, "", http.StatusNotFound},
		{"500 from GeoPulse → 503", http.StatusInternalServerError, "", http.StatusServiceUnavailable},
		{"unparseable body → 503", 0, "<html>", http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, user := seedGeoPulseControllerDB(t)
			router := geopulseTestRouter(t, db, user.ID, config.Config{})
			f := newGeoPulseTestServer(t, "")
			f.FailTimeline = tc.status
			f.RawBody = tc.raw
			connectGeoPulseController(t, router, f.URL())
			w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
			assert.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}

	t.Run("unreachable → 503", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		f := newGeoPulseTestServer(t, "")
		connectGeoPulseController(t, router, f.URL())
		f.Server.Close()
		w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("a stored URL that no longer parses → 400", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		connectGeoPulseController(t, router, "https://gp.example")
		require.NoError(t, db.Model(&models.GeoPulseConfig{}).Where("user_id = ?", user.ID).Update("base_url", "not a url").Error)
		w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// TestConfirmGeoPulseStay_IsIdempotent is the issue-160 confirm flow end to end:
// a suggestion is confirmed through the ordinary POST /activities, pre-filled
// from the suggestion, and confirming it again returns the same Activity.
func TestConfirmGeoPulseStay_IsIdempotent(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	other := models.User{Username: "geopulse-other2", Password: "password123!A", Email: "geopulse-other2@example.com"}
	require.NoError(t, db.Create(&other).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	f := newGeoPulseTestServer(t, "")
	f.addStay(7, "2026-09-20T14:00:00Z", "Cafe Nero", "Leeds", 53.8, -1.55)
	connectGeoPulseController(t, router, f.URL())

	w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var res services.GeoPulseSuggestionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Suggestions, 1)
	sug := res.Suggestions[0]

	confirm := models.ActivityInput{
		Title: "Coffee with Alice", Location: sug.Location, Date: sug.Timestamp,
		ExternalRef: ptrStr(sug.ExternalRef), ContactIDs: []uint{contact.ID},
	}
	first := geopulseDo(t, router, "POST", "/activities", confirm)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), "Activity created successfully")
	var created struct {
		Activity models.Activity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &created))
	assert.Equal(t, "geopulse:stay:7", created.Activity.ExternalRef)
	assert.Equal(t, "Cafe Nero", created.Activity.Location)
	assert.True(t, created.Activity.Date.Equal(time.Date(2026, 9, 20, 14, 0, 0, 0, time.UTC)))

	// Confirm again (a retry, a double click): the same Activity, no second row.
	confirm.Title = "A different title the second time"
	second := geopulseDo(t, router, "POST", "/activities", confirm)
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), "Activity already exists")
	var again struct {
		Activity models.Activity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &again))
	assert.Equal(t, created.Activity.ID, again.Activity.ID)
	assert.Equal(t, "Coffee with Alice", again.Activity.Title, "the existing activity is returned unchanged")
	require.Len(t, again.Activity.Contacts, 1)

	var count int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ? AND external_ref = ?", user.ID, "geopulse:stay:7").Count(&count).Error)
	assert.EqualValues(t, 1, count)

	// The suggestion list now marks the stay as already logged.
	w = geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.NotNil(t, res.Suggestions[0].ExistingActivityID)
	assert.Equal(t, created.Activity.ID, *res.Suggestions[0].ExistingActivityID)

	// Dedupe is scoped to user_id: another user confirming the same stay id gets their own.
	otherRouter := geopulseTestRouter(t, db, other.ID, config.Config{})
	third := geopulseDo(t, otherRouter, "POST", "/activities", models.ActivityInput{Title: "mine", Date: sug.Timestamp, ExternalRef: ptrStr("geopulse:stay:7")})
	require.Equal(t, http.StatusOK, third.Code)
	assert.Contains(t, third.Body.String(), "Activity created successfully")

	// After the user deletes the activity, confirming creates a fresh one.
	require.NoError(t, db.Delete(&models.Activity{}, created.Activity.ID).Error)
	fourth := geopulseDo(t, router, "POST", "/activities", confirm)
	require.Equal(t, http.StatusOK, fourth.Code)
	assert.Contains(t, fourth.Body.String(), "Activity created successfully")
}

// TestCreateActivity_NonGeoPulseRefsAreNotDeduped: only the geopulse:stay: namespace
// is idempotent; every other ExternalRef (and none) keeps creating a new row.
func TestCreateActivity_NonGeoPulseRefsAreNotDeduped(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	for _, ref := range []string{"", "calendar:abc", "calendar:abc", "geopulse:other:1"} {
		w := geopulseDo(t, router, "POST", "/activities", models.ActivityInput{Title: "x", Date: time.Now(), ExternalRef: ptrStr(ref)})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Activity created successfully", "ref %q", ref)
	}
	var count int64
	require.NoError(t, db.Model(&models.Activity{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.EqualValues(t, 4, count)
}

func TestConfirmGeoPulseStay_ContactOwnershipStillEnforced(t *testing.T) {
	db, user := seedGeoPulseControllerDB(t)
	other := models.User{Username: "geopulse-other3", Password: "password123!A", Email: "geopulse-other3@example.com"}
	require.NoError(t, db.Create(&other).Error)
	theirContact := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&theirContact).Error)

	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	w := geopulseDo(t, router, "POST", "/activities", models.ActivityInput{
		Title: "x", Date: time.Now(), ExternalRef: ptrStr("geopulse:stay:9"), ContactIDs: []uint{theirContact.ID},
	})
	assert.Equal(t, http.StatusNotFound, w.Code, "a contact the user does not own must be rejected before any dedupe")
	var count int64
	require.NoError(t, db.Model(&models.Activity{}).Count(&count).Error)
	assert.Zero(t, count)
}

// TestGeoPulse_LocalFailuresAreNotBlamedOnGeoPulse: with the table unreachable every handler
// reports a database error rather than panicking or pretending success.
func TestGeoPulse_LocalFailuresAreNotBlamedOnGeoPulse(t *testing.T) {
	t.Run("config handlers", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		dbtest.HideTable(t, db, "geopulse_configs")

		assert.Equal(t, http.StatusInternalServerError, geopulseDo(t, router, "GET", "/geopulse/config", nil).Code)
		assert.Equal(t, http.StatusInternalServerError, geopulseDo(t, router, "PUT", "/geopulse/config",
			models.GeoPulseConfigInput{BaseURL: "https://gp.example", APIKey: "k"}).Code)
		assert.Equal(t, http.StatusInternalServerError, geopulseDo(t, router, "DELETE", "/geopulse/config", nil).Code)
		assert.Equal(t, http.StatusInternalServerError, geopulseDo(t, router, "POST", "/geopulse/test-connection", nil).Code)
		assert.Equal(t, http.StatusInternalServerError, geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil).Code)
	})

	t.Run("undecryptable stored key", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		connectGeoPulseController(t, router, "https://gp.example")
		rotated := geopulseTestRouter(t, db, user.ID, config.Config{JWTSecretKey: geopulseTestSecret + "-rotated"})
		w := geopulseDo(t, rotated, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, "a rotated JWT_SECRET_KEY is fixed by re-entering the token: "+w.Body.String())
		assert.Contains(t, w.Body.String(), "re-enter")
	})

	t.Run("save fails after the lookup succeeded", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		connectGeoPulseController(t, router, "https://gp.example")
		// A trigger that aborts every update makes the save itself fail while the
		// preceding existence lookup still succeeds.
		require.NoError(t, db.Exec(`CREATE TRIGGER geopulse_block_update BEFORE UPDATE ON geopulse_configs BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)
		w := geopulseDo(t, router, "PUT", "/geopulse/config", models.GeoPulseConfigInput{BaseURL: "https://gp2.example", APIKey: "k2"})
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})

	t.Run("confirm lookup", func(t *testing.T) {
		db, user := seedGeoPulseControllerDB(t)
		router := geopulseTestRouter(t, db, user.ID, config.Config{})
		dbtest.HideTable(t, db, "activities")
		w := geopulseDo(t, router, "POST", "/activities", models.ActivityInput{Title: "x", Date: time.Now(), ExternalRef: ptrStr("geopulse:stay:1")})
		assert.Equal(t, http.StatusInternalServerError, w.Code, "a failed dedupe lookup must not fall through to creating a duplicate")
	})
}

// A GeoPulse that answers with a redirect surfaces as a clear 503 about the
// base URL, and the redirect target is never contacted.
func TestGeoPulseSuggestions_RedirectIsAClearError(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("redirect target must never be contacted (got %s)", r.URL.Path)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusMovedPermanently)
	}))
	defer redirector.Close()

	db, user := seedGeoPulseControllerDB(t)
	router := geopulseTestRouter(t, db, user.ID, config.Config{})
	connectGeoPulseController(t, router, redirector.URL)

	w := geopulseDo(t, router, "GET", "/geopulse/suggestions?date=2026-09-20", nil)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "redirect")
}
