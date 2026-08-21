package services

import (
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedReachOutCorpus inserts contactCount contacts and one update AuditEvent
// per contact whose before-snapshot matches the live state, so
// DetectReachOutSuggestions scans the whole batch without firing a suggestion.
// A fire-free corpus keeps the query tally deterministic: createReachOutSuggestion
// launches the webhook trigger in a goroutine, and its async query would race
// the counter (issue #261). The write path is covered functionally by
// reach_out_trigger_service_test.go; the query-count gate here pins the scan's
// algorithmic shape.
func seedReachOutCorpus(tb testing.TB, db *gorm.DB, userID uint, contactCount int) {
	tb.Helper()

	contacts := make([]models.Contact, 0, contactCount)
	events := make([]models.AuditEvent, 0, contactCount)
	snapshot, err := json.Marshal(models.ContactAuditSnapshot{
		Contact: models.Contact{Organization: "Acme", JobTitle: "Engineer"},
	})
	require.NoError(tb, err)

	for i := 0; i < contactCount; i++ {
		uid := uuid.New().String()
		contacts = append(contacts, models.Contact{
			UserID: userID, Firstname: "Contact", Lastname: fmt.Sprintf("%d", i),
			VCardUID: uid, Organization: "Acme", JobTitle: "Engineer",
		})
		events = append(events, models.AuditEvent{
			EntityType: models.AuditEntityContact, EntityID: uid,
			Operation: models.AuditOpUpdate, UserID: userID, BeforeSnapshot: string(snapshot),
		})
	}

	// SkipHooks so the 20k-scale corpus doesn't pay per-row etag UPDATE +
	// audit goroutine costs; VCardUID is set manually to satisfy the
	// partial-unique index.
	require.NoError(tb, db.Session(&gorm.Session{SkipHooks: true}).CreateInBatches(contacts, 500).Error)
	require.NoError(tb, db.CreateInBatches(events, 500).Error)
}

// TestDetectReachOutSuggestions_QueryCountIsBounded pins issue #261's
// guarantee that the daily detection job scans a user's audit batch in a
// bounded, linear number of queries — C + O(1) for C contacts with events — so
// a regression that adds a per-contact or per-event query (the N+1 shape that
// would silently stall the daily job at scale) fails deterministically.
func TestDetectReachOutSuggestions_QueryCountIsBounded(t *testing.T) {
	const contacts = 200

	dbPath := filepath.Join(t.TempDir(), "reach-out-count.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "reachoutcount", Password: "password123!A", Email: "reachoutcount@example.com"}
	require.NoError(t, db.Create(&user).Error)
	seedReachOutCorpus(t, db, user.ID, contacts)

	// A second connection to the same file with a tallying logger; seeding used
	// the InitDB connection, so it never pollutes the count.
	counter := database.NewQueryCounter()
	countDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: counter})
	require.NoError(t, err)

	DetectReachOutSuggestions(countDB, config.Config{ProfilePhotoDir: t.TempDir()})

	// One scan: lock acquire (SELECT+INSERT), user list, cursor + event batch,
	// one live-contact SELECT per baseline contact, cursor upsert, lock release
	// = contacts + 9 statements. The +25 slack absorbs constant additions while
	// a +1 query per contact (2C+9) still blows the bound by ~2x.
	count := counter.Count()
	t.Logf("detection job issued %d queries for %d contacts", count, contacts)
	assert.LessOrEqual(t, count, int64(contacts+25),
		"detection job must issue a linear number of queries in the contact count")
}

// BenchmarkDetectReachOutSuggestions measures the daily detection job's scan
// over a large synthetic corpus (tens of thousands of contacts + audit rows).
// No suggestions fire (snapshots match live state), so every iteration is pure
// scan cost; each iteration rewinds the cursor and the job lock's last-run
// stamp to simulate a fresh daily batch. CI runs this once via -benchtime=1x
// (.github/workflows/unit-tests.yml); the regression gate is the query-count
// test above, not wall-clock.
func BenchmarkDetectReachOutSuggestions(b *testing.B) {
	const contacts = 20000

	dbPath := filepath.Join(b.TempDir(), "reach-out-bench.db")
	db, err := database.InitDB(dbPath)
	require.NoError(b, err)
	sqlDB, err := db.DB()
	require.NoError(b, err)
	b.Cleanup(func() { _ = sqlDB.Close() })

	user := models.User{Username: "reachoutbench", Password: "password123!A", Email: "reachoutbench@example.com"}
	require.NoError(b, db.Create(&user).Error)
	seedReachOutCorpus(b, db, user.ID, contacts)

	// Pre-create the job-execution row so the lock takes the "job exists" path
	// (SELECT+UPDATE rather than SELECT+INSERT) on every iteration.
	require.NoError(b, db.Create(&models.JobExecution{
		JobName: models.JobNameReachOutDetection, LastRunAt: time.Now().Add(-48 * time.Hour),
	}).Error)

	cfg := config.Config{ProfilePhotoDir: b.TempDir()}
	rewind := func() {
		require.NoError(b, db.Model(&models.ReachOutCursor{}).Where("user_id = ?", user.ID).
			Update("last_audit_event_id", 0).Error)
		require.NoError(b, db.Model(&models.JobExecution{}).Where("job_name = ?", models.JobNameReachOutDetection).
			Update("last_run_at", time.Now().Add(-48*time.Hour)).Error)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rewind()
		DetectReachOutSuggestions(db, cfg)
	}
}
