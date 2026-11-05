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
// (writers-1) × writeHold — under half of busy. Any SQLITE_BUSY therefore
// means a writer was overtaken, not that the database was overloaded.
func TestConcurrentWritersAreServedFairly(t *testing.T) {
	const (
		writers   = 24
		writeHold = 6 * time.Millisecond
		busy      = 400 * time.Millisecond
		runFor    = 3 * time.Second
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
