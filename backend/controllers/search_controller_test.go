package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	db := dbtest.New(t)

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

// TestSearchAll_RejectsOversizedTerm pins issue #415's search-term bound: a
// term longer than services.MaxSearchTermLen must be rejected with a 400
// before any FTS5/LIKE work, and a term at the exact boundary must pass.
func TestSearchAll_RejectsOversizedTerm(t *testing.T) {
	_, router := searchRealRouter(t)

	long := strings.Repeat("a", services.MaxSearchTermLen+1)
	req, _ := http.NewRequest("GET", "/search?q="+long, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	exact := strings.Repeat("a", services.MaxSearchTermLen)
	req2, _ := http.NewRequest("GET", "/search?q="+exact, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
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

// TestRebuildSearchIndexHandler_ReportsCountsAndRecordsJobRun pins SEARCH-01
// (issue #461) recommended action 3: the endpoint returns the per-index row
// counts and records a job_runs row so the rebuild is visible on the admin
// job-run timeline (issue #391), not only in the HTTP response.
func TestRebuildSearchIndexHandler_ReportsCountsAndRecordsJobRun(t *testing.T) {
	db, router := searchRealRouter(t)
	user := models.User{Username: "search-ctrl3", Password: "password123!A", Email: "search-ctrl3@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Bianca", Lastname: "Bach"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "a note", Date: time.Now()}).Error)
	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: "an activity", Date: time.Now()}).Error)

	req, _ := http.NewRequest("POST", "/admin/search/rebuild", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Message string `json:"message"`
		Indexed struct {
			Contacts   int64 `json:"contacts"`
			Notes      int64 `json:"notes"`
			Activities int64 `json:"activities"`
		} `json:"indexed"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, int64(1), body.Indexed.Contacts)
	assert.Equal(t, int64(1), body.Indexed.Notes)
	assert.Equal(t, int64(1), body.Indexed.Activities)

	// RecordJobRun is a fire-and-forget goroutine; give it a beat.
	var run models.JobRun
	require.Eventually(t, func() bool {
		return db.Where("job_name = ?", models.JobNameSearchIndexRebuild).Order("id desc").First(&run).Error == nil
	}, 2*time.Second, 20*time.Millisecond, "a job_runs row must be recorded for the rebuild")

	assert.Equal(t, models.JobTriggerManual, run.Trigger)
	assert.Equal(t, models.JobRunResultSuccess, run.Result)
	require.NotNil(t, run.ItemsProcessed)
	assert.Equal(t, 3, *run.ItemsProcessed, "items_processed is the total rows indexed across the three tables")
}

// TestRebuildSearchIndexHandler_ConflictWhenInProgress pins recommended
// action 4 at the HTTP layer: a rebuild requested while one is already
// running in this process is refused with 409, not queued, and the
// in-progress rebuild still completes normally.
func TestRebuildSearchIndexHandler_ConflictWhenInProgress(t *testing.T) {
	db, router := searchRealRouter(t)
	user := models.User{Username: "search-ctrl4", Password: "password123!A", Email: "search-ctrl4@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Clara", Lastname: "Schumann"}).Error)

	// Wedge the first rebuild open: block inside its transaction, on the
	// contacts_fts INSERT, until the test releases it.
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	const cb = "search_ctrl_block_rebuild"
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(cb, func(d *gorm.DB) {
		if strings.Contains(d.Statement.SQL.String(), "INSERT INTO contacts_fts") {
			once.Do(func() { close(entered) })
			<-release
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(cb) })

	firstDone := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/admin/search/rebuild", nil)
		router.ServeHTTP(w, req)
		firstDone <- w.Code
	}()

	<-entered // the first rebuild now holds the in-process guard

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/admin/search/rebuild", nil)
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusConflict, w2.Code, "a concurrent rebuild is refused, not queued")

	close(release)
	assert.Equal(t, http.StatusOK, <-firstDone, "the first rebuild completes normally")
}

// TestRebuildSearchIndexHandler_FailureRecordedAndReported pins the failure
// path: a rebuild that errors returns 500, leaves the previous index in
// place, and is recorded as a failed job run.
func TestRebuildSearchIndexHandler_FailureRecordedAndReported(t *testing.T) {
	db, router := searchRealRouter(t)
	user := models.User{Username: "search-ctrl5", Password: "password123!A", Email: "search-ctrl5@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Dora", Lastname: "Pejacevic"}).Error)

	const cb = "search_ctrl_fail_rebuild"
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(cb, func(d *gorm.DB) {
		if strings.Contains(d.Statement.SQL.String(), "INSERT INTO contacts_fts") {
			_ = d.AddError(assert.AnError)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(cb) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/search/rebuild", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())

	require.NoError(t, db.Callback().Raw().Remove(cb))

	// The pre-existing index row survived the rolled-back rebuild.
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM contacts_fts WHERE rowid = (SELECT id FROM contacts WHERE firstname = 'Dora')`).Scan(&n).Error)
	assert.Equal(t, int64(1), n, "the failed rebuild left the previous index in place")

	require.Eventually(t, func() bool {
		var c int64
		db.Model(&models.JobRun{}).
			Where("job_name = ? AND result = ?", models.JobNameSearchIndexRebuild, models.JobRunResultFailure).
			Count(&c)
		return c == 1
	}, 2*time.Second, 20*time.Millisecond, "the failed rebuild is recorded as a failed job run")
}
