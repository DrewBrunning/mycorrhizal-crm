package services

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSchedulerContention_ManyUsersOneWriter is the "many users on one
// instance" axis of issue #498. The scheduler is a single in-process gocron
// instance and every write in the app funnels through one SQLite writer;
// per-user background jobs (cadence, reach-out detection, webhook retries,
// storage sampling, purge) fan their queries out across the user set, so ten
// users on the same cadence is a contention profile one user with ten times
// the data never produces.
//
// The contract this pins: under that contention the shared writer never
// returns SQLITE_BUSY / "database is locked" (CLAUDE.md trap #9 —
// _txlock=immediate + busy_timeout must hold), the jobs complete without
// error, and the database stays structurally and semantically intact
// (PRAGMA integrity_check + ADR-0012 invariants).
func TestSchedulerContention_ManyUsersOneWriter(t *testing.T) {
	db := dbtest.New(t)
	cfg := config.Config{
		ProfilePhotoDir:               t.TempDir(),
		DBIntegrityCheckEnabled:       false,
		DBIntegrityCheckIntervalHours: 24,
	}

	const users = 8
	userIDs := make([]uint, users)
	for i := 0; i < users; i++ {
		u := models.User{
			Username: fmt.Sprintf("contention-%d", i),
			Email:    fmt.Sprintf("contention-%d@example.com", i),
			Password: "x",
		}
		require.NoError(t, db.Create(&u).Error)
		userIDs[i] = u.ID
		for c := 0; c < 4; c++ {
			require.NoError(t, db.Create(&models.Contact{
				UserID:    u.ID,
				Firstname: fmt.Sprintf("C%d-%d", i, c),
				Lastname:  "Seed",
			}).Error)
		}
	}

	var (
		mu   sync.Mutex
		errs []error
	)
	record := func(context string, err error) {
		if err == nil {
			return
		}
		// ErrJobSkipped is the *defined* outcome when the shared job lock is
		// already held or the job ran within its de-dup window (issue #526,
		// ADR 0011) — exactly what should happen when many invocations race.
		// It is not a contention failure.
		if errors.Is(err, ErrJobSkipped) {
			return
		}
		mu.Lock()
		errs = append(errs, fmt.Errorf("%s: %w", context, err))
		mu.Unlock()
	}

	var wg sync.WaitGroup

	// Writer goroutines: one per user, each appending rows through the shared
	// connection while the jobs run.
	for i, uid := range userIDs {
		wg.Add(1)
		go func(i int, uid uint) {
			defer wg.Done()
			for n := 0; n < 15; n++ {
				c := models.Contact{UserID: uid, Firstname: fmt.Sprintf("W%d-%d", i, n), Lastname: "Live"}
				record("contact create", db.Create(&c).Error)
				record("note create", db.Create(&models.Note{UserID: uid, Content: fmt.Sprintf("n%d-%d", i, n)}).Error)
			}
		}(i, uid)
	}

	// Job goroutines: the per-user scheduled jobs, each run several times so
	// they overlap each other and the writers on the one writer.
	jobRunners := []struct {
		name string
		fn   func() error
	}{
		{"ProcessOverdueCadences", func() error { _, err := ProcessOverdueCadences(db, cfg); return err }},
		{"DetectReachOutSuggestions", func() error { _, err := DetectReachOutSuggestions(db, cfg); return err }},
		{"ProcessWebhookRetries", func() error { ProcessWebhookRetries(db, cfg); return nil }},
		{"RecordStorageSampleScheduled", func() error { RecordStorageSampleScheduled(db, cfg); return nil }},
		{"PurgeDeletedRows", func() error { PurgeDeletedRows(db, cfg); return nil }},
		{"SendRemindersWithRateLimit", func() error { _, err := SendRemindersWithRateLimit(db, cfg); return err }},
	}
	for _, jr := range jobRunners {
		wg.Add(1)
		go func(name string, fn func() error) {
			defer wg.Done()
			for n := 0; n < 4; n++ {
				record(name, fn())
			}
		}(jr.name, jr.fn)
	}

	wg.Wait()

	for _, err := range errs {
		msg := strings.ToLower(err.Error())
		assert.NotContains(t, msg, "database is locked", "the shared writer must queue, never return SQLITE_BUSY: %v", err)
		assert.NotContains(t, msg, "sqlite_busy", "the shared writer must queue, never return SQLITE_BUSY: %v", err)
		// Any other error is also a failure here — these jobs must tolerate
		// concurrent execution.
		t.Errorf("job/writer error under contention: %v", err)
	}

	assertDatabaseHealthy(t, db)

	// Every seeded user's rows are still there and coherent.
	var total int64
	require.NoError(t, db.Model(&models.Contact{}).Count(&total).Error)
	assert.GreaterOrEqual(t, total, int64(users*(4+15)), "no writes were lost to contention")
}
