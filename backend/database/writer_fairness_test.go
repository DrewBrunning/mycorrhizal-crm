package database

import (
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Writers must be served in arrival order, so no single write is starved past
// busy_timeout while the total queue is well inside it.
//
// SQLite's busy handler is a polling sleep (1, 2, 5, … up to 100 ms between
// retries), not a queue. Under steady contention a writer that has been
// waiting longest sleeps longest, and writers arriving between its polls keep
// taking the lock first. On a slow disk every write holds the lock longer and
// one of 20 concurrent writers can wait out the whole 5s busy_timeout even
// though the queue ahead of it at any moment was far shorter: the release
// battery's loadsmoke saw exactly that (1–2 of ~1,500 requests returned 503
// DATABASE_BUSY at half the usual throughput), and so would a self-hoster on
// slow storage.
//
// The test scales the production numbers down: each write holds the lock for
// ~writeHold, so with every writer queued the longest fair wait is about
// (writers-1) × writeHold plus the gate's starvation window. Any SQLITE_BUSY
// therefore means a writer was overtaken, not that the database was
// overloaded.
//
// "Not overloaded" is only true while a write really holds the lock for about
// writeHold. The hold is the sleep plus the commit, and on a loaded CI runner
// (the cross-package coverage pass runs four packages at once with atomic
// coverage on every one) the sleep overshoots and the commit waits on the disk,
// so a hold of ~18ms is realistic: 23 × 18ms is already past a 400ms budget
// with nobody overtaken, which is how this test flaked on unrelated PRs
// (issue #2398). busy is therefore several times the nominal queue, and the
// slow-hold case below pins that. A writer that is genuinely starved still
// waits out any budget: with the gate bypassed, ~6% of writes time out even at
// 2s (checked by hand when busy was widened).
func TestConcurrentWritersAreServedFairly(t *testing.T) {
	const writeHold = 6 * time.Millisecond
	t.Run("nominal hold", func(t *testing.T) { runWriterFairness(t, writeHold) })
	t.Run("slow runner hold", func(t *testing.T) { runWriterFairness(t, 3*writeHold) })
}

func runWriterFairness(t *testing.T, writeHold time.Duration) {
	const (
		writers = 24
		busy    = 2 * time.Second
		runFor  = 3 * time.Second
	)
	if (writers-1)*writeHold*2 >= busy {
		t.Fatalf("test misconfigured: a fair queue (%v) must fit well inside busy (%v)", (writers-1)*writeHold, busy)
	}

	dbPath := filepath.Join(t.TempDir(), "fairness.db")
	db, err := openGormDB(dbPath, busy, newGormLogger(io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec("PRAGMA journal_mode=WAL").Error)
	require.NoError(t, db.Exec("CREATE TABLE writes (id INTEGER PRIMARY KEY, worker INTEGER NOT NULL)").Error)

	var (
		wg       sync.WaitGroup
		total    atomic.Int64
		busyHits atomic.Int64
		mu       sync.Mutex
		other    []string
	)
	deadline := time.Now().Add(runFor)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				err := db.Transaction(func(tx *gorm.DB) error {
					if err := tx.Exec("INSERT INTO writes (worker) VALUES (?)", worker).Error; err != nil {
						return err
					}
					time.Sleep(writeHold) // a write that holds the lock, as on a slow disk
					return nil
				})
				total.Add(1)
				switch {
				case err == nil:
				case strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY"):
					busyHits.Add(1)
				default:
					mu.Lock()
					other = append(other, err.Error())
					mu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()

	require.Empty(t, other, "unexpected write errors")
	require.Zerof(t, busyHits.Load(),
		"%d/%d writes waited out busy_timeout (%v) although a fair queue of %d writers is at most ~%v: a writer was starved",
		busyHits.Load(), total.Load(), busy, writers, (writers-1)*writeHold)
}
