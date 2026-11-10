package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #651: GET /contacts/import/history returns the caller's own import
// runs, newest first, and serialises as [] (not null) when empty. Exercised
// against the real migrated schema (dbtest.New — CLAUDE.md backend trap #1).

func importHistoryRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Next()
	})
	r.GET("/contacts/import/history", GetImportHistory)
	r.GET("/contacts/import/history/:id", GetImportRun)
	return r
}

func seedImportRun(t *testing.T, db *gorm.DB, userID uint, format string, created time.Time) {
	t.Helper()
	run := models.ImportRun{UserID: userID, Format: format, TotalProcessed: 1, Created: 1}
	require.NoError(t, db.Create(&run).Error)
	// created_at is stamped by RecordImportRun in production; the endpoint
	// orders by it, so set it explicitly here.
	require.NoError(t, db.Model(&models.ImportRun{}).Where("id = ?", run.ID).
		UpdateColumn("created_at", created).Error)
}

func TestGetImportHistory_EmptySerialisesAsArray(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "eh", Email: "eh@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "[]", w.Body.String(),
		"empty history must be a JSON array, never null (frontend trap #8)")
}

func TestGetImportHistory_NewestFirst(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "nf", Email: "nf@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now().UTC()
	seedImportRun(t, db, user.ID, models.ImportFormatCSV, now.Add(-2*time.Hour))
	seedImportRun(t, db, user.ID, models.ImportFormatVCF, now.Add(-1*time.Hour))
	seedImportRun(t, db, user.ID, models.ImportFormatJSContact, now)

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got []models.ImportRun
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 3)
	assert.Equal(t, models.ImportFormatJSContact, got[0].Format)
	assert.Equal(t, models.ImportFormatVCF, got[1].Format)
	assert.Equal(t, models.ImportFormatCSV, got[2].Format)
}

func TestGetImportHistory_CapsAtFifty(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "cap", Email: "cap@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	base := time.Now().UTC().Add(-100 * time.Hour)
	for i := 0; i < 60; i++ {
		seedImportRun(t, db, user.ID, models.ImportFormatCSV, base.Add(time.Duration(i)*time.Minute))
	}

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got []models.ImportRun
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got, 50, "history is capped at importHistoryLimit")
	// Newest-first: the first row is the most recent seed (i=59).
	assert.True(t, got[0].CreatedAt.After(got[49].CreatedAt))
}

func TestGetImportHistory_ScopedToCaller(t *testing.T) {
	db := dbtest.New(t)
	a := models.User{Username: "a", Email: "a@example.com", Password: "x"}
	b := models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)

	now := time.Now().UTC()
	seedImportRun(t, db, a.ID, models.ImportFormatCSV, now)
	seedImportRun(t, db, b.ID, models.ImportFormatVCF, now)
	seedImportRun(t, db, b.ID, models.ImportFormatVCF, now.Add(-time.Minute))

	w := httptest.NewRecorder()
	importHistoryRouter(db, a.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got []models.ImportRun
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got, 1, "user A must never see user B's import runs")
	assert.Equal(t, a.ID, got[0].UserID)
}

func TestGetImportHistory_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)

	w := httptest.NewRecorder()
	importHistoryRouter(db, 0).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))

	alAssertError(t, w, http.StatusUnauthorized, "UNAUTHORIZED", "")
}

// Issue #2463: the list reports issue_count but never the issues themselves;
// the per-run detail returns them, scoped to the owner.

func TestGetImportHistory_ListHasIssueCountNotIssues(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "ic", Email: "ic@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	models.RecordImportRun(t.Context(), db, models.ImportRun{
		UserID: user.ID, Format: models.ImportFormatCSV,
		Issues: []models.ImportRowIssue{{Row: 2, Name: "A", Status: "skipped", Reason: "r"}},
	})

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Len(t, raw, 1)
	assert.EqualValues(t, 1, raw[0]["issue_count"])
	assert.NotContains(t, raw[0], "issues", "the list must stay small: no issues array")
	assert.NotContains(t, raw[0], "IssuesJSON")
}

func TestGetImportRun_ReturnsIssuesForOwner(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "gr", Email: "gr@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	models.RecordImportRun(t.Context(), db, models.ImportRun{
		UserID: user.ID, Format: models.ImportFormatCSV, TotalProcessed: 2, Skipped: 1,
		Issues: []models.ImportRowIssue{{Row: 2, Name: "A", Email: "a@x.io", Status: "skipped", Reason: "r"}},
	})
	var run models.ImportRun
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&run).Error)

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history/"+strconv.Itoa(int(run.ID)), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got struct {
		ID         uint                    `json:"id"`
		IssueCount int                     `json:"issue_count"`
		Issues     []models.ImportRowIssue `json:"issues"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, run.ID, got.ID)
	require.Len(t, got.Issues, 1)
	assert.Equal(t, models.ImportRowIssue{Row: 2, Name: "A", Status: "skipped", Reason: "r"}, got.Issues[0],
		"email/phone come back empty")
}

func TestGetImportRun_NoIssuesSerialisesAsEmptyArray(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "ge", Email: "ge@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	models.RecordImportRun(t.Context(), db, models.ImportRun{UserID: user.ID, Format: models.ImportFormatVCF})
	var run models.ImportRun
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&run).Error)

	w := httptest.NewRecorder()
	importHistoryRouter(db, user.ID).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/contacts/import/history/"+strconv.Itoa(int(run.ID)), nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"issues":[]`, "never null (frontend trap #8)")
}

func TestGetImportRun_ForeignOrMissingIs404(t *testing.T) {
	db := dbtest.New(t)
	owner := models.User{Username: "own", Email: "own@example.com", Password: "x"}
	other := models.User{Username: "oth", Email: "oth@example.com", Password: "x"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)
	models.RecordImportRun(t.Context(), db, models.ImportRun{
		UserID: owner.ID, Format: models.ImportFormatCSV,
		Issues: []models.ImportRowIssue{{Row: 1, Name: "Secret Name", Status: "failed", Reason: "r"}},
	})
	var run models.ImportRun
	require.NoError(t, db.Where("user_id = ?", owner.ID).First(&run).Error)

	for _, id := range []string{strconv.Itoa(int(run.ID)), "99999", "abc"} {
		w := httptest.NewRecorder()
		importHistoryRouter(db, other.ID).ServeHTTP(w,
			httptest.NewRequest(http.MethodGet, "/contacts/import/history/"+id, nil))
		assert.Equal(t, http.StatusNotFound, w.Code, id)
		assert.NotContains(t, w.Body.String(), "Secret Name")
	}
}
