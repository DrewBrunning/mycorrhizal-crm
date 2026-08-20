package controllers

import (
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupReachOutSuggestionRouter builds a real-schema router (CLAUDE.md
// backend trap 1) with the list/dismiss endpoints for one authenticated user.
func setupReachOutSuggestionRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "reach-out-ctrl.db"))
	require.NoError(t, err)

	user := models.User{Username: "rosuser", Password: "password123!A", Email: "rosuser@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "rosother", Password: "password123!A", Email: "rosother@example.com"}
	require.NoError(t, db.Create(&other).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/reach-out-suggestions", ListReachOutSuggestions)
	router.POST("/reach-out-suggestions/:id/dismiss", DismissReachOutSuggestion)
	return db, router, user
}

func seedReachOutSuggestion(t *testing.T, db *gorm.DB, userID uint, contact models.Contact) models.ReachOutSuggestion {
	t.Helper()
	s := models.ReachOutSuggestion{
		UserID: userID, ContactVCardUID: contact.VCardUID, Kind: models.ReachOutKindOrganization,
		OldValue: "OldCo", NewValue: "NewCo", AuditEventID: 1, Status: models.ReachOutStatusPending,
	}
	require.NoError(t, db.Create(&s).Error)
	return s
}

func TestListReachOutSuggestions_ReturnsPendingOnly(t *testing.T) {
	db, router, user := setupReachOutSuggestionRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Smith"}
	require.NoError(t, db.Create(&contact).Error)

	pending := seedReachOutSuggestion(t, db, user.ID, contact)
	dismissed := seedReachOutSuggestion(t, db, user.ID, contact)
	require.NoError(t, db.Model(&dismissed).Update("status", models.ReachOutStatusDismissed).Error)

	req, _ := http.NewRequest("GET", "/reach-out-suggestions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Suggestions []models.ReachOutSuggestionResponse `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Suggestions, 1)
	assert.Equal(t, pending.ID, resp.Suggestions[0].ID)
	assert.Equal(t, contact.ID, resp.Suggestions[0].ContactID)
	assert.Equal(t, "Alice Smith", resp.Suggestions[0].ContactName)
}

// TestListReachOutSuggestions_ExcludesArchivedContacts covers a code review
// fix: a suggestion whose contact was archived after the suggestion was
// created must not keep showing on the dashboard, matching every other
// dashboard block's archived=false filter.
func TestListReachOutSuggestions_ExcludesArchivedContacts(t *testing.T) {
	db, router, user := setupReachOutSuggestionRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Archie", Archived: true}
	require.NoError(t, db.Create(&contact).Error)
	seedReachOutSuggestion(t, db, user.ID, contact)

	req, _ := http.NewRequest("GET", "/reach-out-suggestions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Suggestions []models.ReachOutSuggestionResponse `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Suggestions, "a suggestion for an archived contact must not appear on the dashboard")
}

func TestDismissReachOutSuggestion_MarksDismissed(t *testing.T) {
	db, router, user := setupReachOutSuggestionRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&contact).Error)
	s := seedReachOutSuggestion(t, db, user.ID, contact)

	req, _ := http.NewRequest("POST", "/reach-out-suggestions/"+s.ID+"/dismiss", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.ReachOutSuggestion
	require.NoError(t, db.First(&reloaded, "id = ?", s.ID).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, reloaded.Status)

	// Idempotent: dismissing again is still a 200, not an error.
	req2, _ := http.NewRequest("POST", "/reach-out-suggestions/"+s.ID+"/dismiss", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestDismissReachOutSuggestion_UnknownIDIs404(t *testing.T) {
	_, router, _ := setupReachOutSuggestionRouter(t)

	req, _ := http.NewRequest("POST", "/reach-out-suggestions/does-not-exist/dismiss", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDismissReachOutSuggestion_ForeignUserCannotDismiss(t *testing.T) {
	db, router, _ := setupReachOutSuggestionRouter(t)

	var other models.User
	require.NoError(t, db.Where("username = ?", "rosother").First(&other).Error)
	contact := models.Contact{UserID: other.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&contact).Error)
	foreignSuggestion := seedReachOutSuggestion(t, db, other.ID, contact)

	req, _ := http.NewRequest("POST", "/reach-out-suggestions/"+foreignSuggestion.ID+"/dismiss", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var reloaded models.ReachOutSuggestion
	require.NoError(t, db.First(&reloaded, "id = ?", foreignSuggestion.ID).Error)
	assert.Equal(t, models.ReachOutStatusPending, reloaded.Status, "a foreign user's dismiss attempt must not affect another user's suggestion")
}
