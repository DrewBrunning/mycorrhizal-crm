package controllers

import (
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupCountingRouter builds a router whose handlers see a real migrated
// schema (CLAUDE.md backend trap 1) through a tallying GORM connection, while
// seeding happens on a separate connection to the same file so the tallied
// handler's count is exactly its own queries (issue #261). Mirrors the
// two-connection shape of database/concurrent_write_test.go and
// services/duplicate_service_real_db_test.go.
func setupCountingRouter(tb testing.TB) (*gorm.DB, *gin.Engine, *database.QueryCounter, uint) {
	tb.Helper()
	gin.SetMode(gin.ReleaseMode)

	dbPath := filepath.Join(tb.TempDir(), "hot-path.db")
	db, err := database.InitDB(dbPath)
	require.NoError(tb, err)

	user := models.User{Username: "perftest", Password: "password123!A", Email: "perftest@example.com"}
	require.NoError(tb, db.Create(&user).Error)

	counter := database.NewQueryCounter()
	countDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: counter})
	require.NoError(tb, err)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", countDB)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: tb.TempDir()})
		c.Next()
	})
	return db, router, counter, user.ID
}

// seedDashboardCorpus populates every block the composite reads: upcoming
// birthdays, plain + favorite contacts, reminders with contacts, cadence
// policies, and pending reach-out suggestions.
func seedDashboardCorpus(tb testing.TB, db *gorm.DB, userID uint) {
	tb.Helper()
	now := time.Now()
	// A date in the next calendar month is always caught by
	// GetUpcomingBirthdays' "current month from today / next month" filter.
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())

	contacts := make([]models.Contact, 0, 500)
	for i := 0; i < 500; i++ {
		contacts = append(contacts, models.Contact{
			UserID: userID, Firstname: fmt.Sprintf("Contact%d", i), Lastname: "Perf",
		})
	}
	for i := 0; i < 20; i++ {
		contacts[i].Birthday = nextMonth.AddDate(0, 0, 1+i).Format("2006-01-02")
	}
	for i := 0; i < 5; i++ {
		contacts[100+i].IsFavorite = true
	}
	require.NoError(tb, db.CreateInBatches(contacts, 500).Error)

	// 8 reminders in the next 7 days: GetUpcomingReminders returns after its
	// first query (len > 5), keeping the composite's count deterministic.
	reminders := make([]models.Reminder, 0, 8)
	for i := 0; i < 8; i++ {
		reminders = append(reminders, models.Reminder{
			UserID: userID, ContactID: &contacts[i].ID, Message: fmt.Sprintf("Perf reminder %d", i),
			RemindAt: now.Add(time.Duration(i) * time.Hour), Recurrence: "once",
		})
	}
	require.NoError(tb, db.CreateInBatches(reminders, 100).Error)

	policies := make([]models.CadencePolicy, 0, 20)
	for i := 0; i < 20; i++ {
		policies = append(policies, models.CadencePolicy{
			UserID: userID, EntityID: contacts[50+i].VCardUID, TargetIntervalDays: 30,
		})
	}
	require.NoError(tb, db.CreateInBatches(policies, 100).Error)

	suggestions := make([]models.ReachOutSuggestion, 0, 20)
	for i := 0; i < 20; i++ {
		suggestions = append(suggestions, models.ReachOutSuggestion{
			UserID: userID, ContactVCardUID: contacts[150+i].VCardUID, Kind: models.ReachOutKindOrganization,
			OldValue: "Old", NewValue: "New", AuditEventID: uint(1000 + i), Status: models.ReachOutStatusPending,
		})
	}
	require.NoError(tb, db.CreateInBatches(suggestions, 100).Error)
}

// TestGetDashboard_QueryCountIsBounded pins issue #261's guarantee that the
// /dashboard composite stays a small constant number of queries regardless of
// data volume. Seeded with every block populated, the expected tally is 10
// (birthdays + random + favorites + reminders + reminder names + cadence
// policies + last-interaction + cadence contacts + suggestions + suggestion
// contacts); the ≤ 14 bound leaves room for a constant addition while a
// per-item fetch (e.g. one query per reminder's contact) blows past it.
func TestGetDashboard_QueryCountIsBounded(t *testing.T) {
	db, router, counter, userID := setupCountingRouter(t)
	router.GET("/dashboard", GetDashboard)
	seedDashboardCorpus(t, db, userID)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	count := counter.Count()
	t.Logf("dashboard composite issued %d queries", count)
	assert.LessOrEqual(t, int(count), 14,
		"dashboard composite must stay a small constant number of queries regardless of data volume")
}

// BenchmarkGetDashboard measures the dashboard composite over a populated
// account (500 contacts plus birthdays/reminders/cadences/suggestions). CI
// runs it once via -benchtime=1x; the regression gate is the query-count test
// above, not wall-clock.
func BenchmarkGetDashboard(b *testing.B) {
	db, router, _, userID := setupCountingRouter(b)
	router.GET("/dashboard", GetDashboard)
	seedDashboardCorpus(b, db, userID)

	req, _ := http.NewRequest("GET", "/dashboard", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("dashboard returned %d", w.Code)
		}
	}
}

// TestGetContactsList_QueryCountIsBounded pins the contacts list endpoint's
// query shape over 500 contacts: one query for the plain page, and one per
// requested preload (notes/activities/reminders) on the includes= path — never
// one per contact.
func TestGetContactsList_QueryCountIsBounded(t *testing.T) {
	db, router, counter, userID := setupCountingRouter(t)
	router.GET("/contacts", GetContacts)

	contacts := make([]models.Contact, 0, 500)
	for i := 0; i < 500; i++ {
		contacts = append(contacts, models.Contact{
			UserID: userID, Firstname: fmt.Sprintf("Contact%d", i), Lastname: "Perf",
		})
	}
	require.NoError(t, db.CreateInBatches(contacts, 500).Error)

	// Some related rows so the includes= preloads actually resolve data.
	require.NoError(t, db.CreateInBatches([]models.Note{
		{UserID: userID, Content: "note", Date: time.Now(), ContactID: &contacts[0].ID},
	}, 100).Error)
	require.NoError(t, db.CreateInBatches([]models.Activity{
		{UserID: userID, Title: "call", Date: time.Now(), Type: "call", Contacts: []models.Contact{contacts[0]}},
	}, 100).Error)
	require.NoError(t, db.CreateInBatches([]models.Reminder{
		{UserID: userID, Message: "remind", RemindAt: time.Now(), Recurrence: "once", ContactID: &contacts[0].ID},
	}, 100).Error)

	w := doPerfGet(t, router, "/contacts")
	require.Equal(t, http.StatusOK, w.Code)
	t.Logf("plain contacts list issued %d queries", counter.Count())
	assert.LessOrEqual(t, int(counter.Count()), 2,
		"plain contacts list must be a single query regardless of contact count")

	counter.Reset()
	w = doPerfGet(t, router, "/contacts?includes=notes,activities,reminders")
	require.Equal(t, http.StatusOK, w.Code)
	t.Logf("contacts list with includes issued %d queries", counter.Count())
	assert.LessOrEqual(t, int(counter.Count()), 6,
		"includes= preloads must add a constant number of queries, never one per contact")
}

// TestGetGraph_QueryCountIsBounded pins the network graph endpoint over a
// populated account (200 contacts, 50 edges, 100 multi-contact activities):
// contacts + edges + activities + the activity preload's join and IN queries
// — a constant, never one query per activity/edge.
func TestGetGraph_QueryCountIsBounded(t *testing.T) {
	db, router, counter, userID := setupCountingRouter(t)
	router.GET("/graph", GetGraph)

	contacts := make([]models.Contact, 0, 200)
	for i := 0; i < 200; i++ {
		contacts = append(contacts, models.Contact{
			UserID: userID, Firstname: fmt.Sprintf("Contact%d", i), Lastname: "Perf",
		})
	}
	require.NoError(t, db.CreateInBatches(contacts, 200).Error)

	edges := make([]models.RelationshipEdge, 0, 50)
	for i := 0; i < 50; i++ {
		edges = append(edges, models.RelationshipEdge{
			UserID: userID, SourceID: contacts[i].VCardUID, TargetID: contacts[i+1].VCardUID,
			Type: "friend_of", Source: models.RelationshipSourceUserConfirmed,
			Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
		})
	}
	require.NoError(t, db.CreateInBatches(edges, 100).Error)

	activities := make([]models.Activity, 0, 100)
	for i := 0; i < 100; i++ {
		activities = append(activities, models.Activity{
			UserID: userID, Title: fmt.Sprintf("Activity %d", i), Date: time.Now(), Type: "call",
			Contacts: []models.Contact{contacts[i%200], contacts[(i+1)%200]},
		})
	}
	require.NoError(t, db.CreateInBatches(activities, 100).Error)

	w := doPerfGet(t, router, "/graph")
	require.Equal(t, http.StatusOK, w.Code)
	count := counter.Count()
	t.Logf("graph issued %d queries", count)
	assert.LessOrEqual(t, int(count), 7,
		"network graph must stay a small constant number of queries regardless of contact/edge/activity count")
}

// doPerfGet issues a GET against the router and returns the recorder.
func doPerfGet(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
