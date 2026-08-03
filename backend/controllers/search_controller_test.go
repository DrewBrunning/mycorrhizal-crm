package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// searchResult is the subset of services.SearchResult this test decodes.
type searchResult struct {
	Query            string          `json:"query"`
	ResolvedRelation string          `json:"resolved_relation"`
	Contacts         []searchContact `json:"contacts"`
	Notes            []searchNote    `json:"notes"`
	Activities       []searchAct     `json:"activities"`
}
type searchContact struct {
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
}
type searchNote struct {
	Content     string `json:"content"`
	ContactName string `json:"contact_name"`
}
type searchAct struct {
	Title string `json:"title"`
}

// searchRealRouter builds a real-schema router (database.InitDB — the FTS5
// tables + triggers live in migration 000007, invisible to AutoMigrate) with
// the search routes wired.
func searchRealRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "search-ctrl.db"))
	require.NoError(t, err)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.GET("/search", SearchAll)
	router.POST("/admin/search/rebuild", RebuildSearchIndexHandler)
	return db, router
}

func TestSearchAll_GroupedResults(t *testing.T) {
	db, router := searchRealRouter(t)

	user := models.User{Username: "search-ctrl", Password: "password123!A", Email: "search-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Mozart", Lastname: "Symphony"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "review the symphony score", Date: time.Now(), ContactID: &contact.ID}).Error)
	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: "Symphony concert", Date: time.Now()}).Error)

	req, _ := http.NewRequest("GET", "/search?q=symphony", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp searchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Contacts, 1)
	require.Len(t, resp.Notes, 1)
	assert.Equal(t, "Mozart Symphony", resp.Notes[0].ContactName)
	require.Len(t, resp.Activities, 1)
}

func TestSearchAll_Validation(t *testing.T) {
	_, router := searchRealRouter(t)

	// Missing q.
	req, _ := http.NewRequest("GET", "/search", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Bad limit.
	req2, _ := http.NewRequest("GET", "/search?q=abc&limit=banana", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestSearchAll_HouseholdScope(t *testing.T) {
	db, router := searchRealRouter(t)
	user := models.User{Username: "search-ctrl-hh", Password: "password123!A", Email: "search-ctrl-hh@example.com"}
	require.NoError(t, db.Create(&user).Error)

	member := models.Contact{UserID: user.ID, Firstname: "Ann", Lastname: "Smith"}
	outsider := models.Contact{UserID: user.ID, Firstname: "Zoe", Lastname: "Smith"}
	require.NoError(t, db.Create(&member).Error)
	require.NoError(t, db.Create(&outsider).Error)

	household := models.Household{UserID: user.ID, Name: "Smith household", Type: models.HouseholdTypeFamilyUnit}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: member.VCardUID}).Error)

	// Unscoped: both Smiths.
	req, _ := http.NewRequest("GET", "/search?q=smith", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var all searchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &all))
	require.Len(t, all.Contacts, 2)

	// Scoped: only the member.
	req2, _ := http.NewRequest("GET", "/search?q=smith&household_id="+household.ID, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var scoped searchResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &scoped))
	require.Len(t, scoped.Contacts, 1)

	// A household the caller does not own → 404.
	req3, _ := http.NewRequest("GET", "/search?q=smith&household_id=00000000-0000-0000-0000-000000000000", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code)
}

func TestRebuildSearchIndexHandler(t *testing.T) {
	db, router := searchRealRouter(t)
	user := models.User{Username: "search-ctrl2", Password: "password123!A", Email: "search-ctrl2@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Anna", Lastname: "Mozart"}).Error)

	// Drop the index contents directly (simulating a bulk operation that
	// bypassed the triggers), then rebuild via the admin endpoint.
	require.NoError(t, db.Exec("DELETE FROM contacts_fts").Error)

	req, _ := http.NewRequest("POST", "/admin/search/rebuild", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	searchReq, _ := http.NewRequest("GET", "/search?q=anna", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, searchReq)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp searchResult
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.Len(t, resp.Contacts, 1)
}
