package database

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mycorrhizal/internal/writegate"
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
// slow-hold case below pins that.
//
// A host stall is not unfair queueing either: a multi-second disk flush (or
// the whole process being descheduled) keeps the head writer on the gate while
// everyone behind it waits out busy_timeout. The test therefore tolerates
// queue timeouts only when some write was actually in flight for at least
// busy; a timeout with no such stall means a waiter was overtaken. A bare
// SQLITE_BUSY is always a failure: everything in this process writes behind
// the gate, so only a bypassed write can reach SQLite's busy handler.
// TestOpenGormDBRoutesWritesThroughTheSharedGate pins that wiring
// deterministically, and the writegate package's unit tests pin arrival order.
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
		wg           sync.WaitGroup
		total        atomic.Int64
		gateTimeouts atomic.Int64
		sqliteBusy   atomic.Int64
		maxWork      atomic.Int64 // longest time one write was in flight, excluding its queue wait
		mu           sync.Mutex
		other        []string
	)
	recordMax := func(d time.Duration) {
		for {
			cur := maxWork.Load()
			if int64(d) <= cur || maxWork.CompareAndSwap(cur, int64(d)) {
				return
			}
		}
	}
	deadline := time.Now().Add(runFor)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				var entered time.Time
				err := db.Transaction(func(tx *gorm.DB) error {
					entered = time.Now() // the gate is held from here until Transaction returns
					if err := tx.Exec("INSERT INTO writes (worker) VALUES (?)", worker).Error; err != nil {
						return err
					}
					time.Sleep(writeHold) // a write that holds the lock, as on a slow disk
					return nil
				})
				if !entered.IsZero() {
					recordMax(time.Since(entered))
				}
				total.Add(1)
				switch {
				case err == nil:
				case errors.Is(err, writegate.ErrQueueTimeout):
					gateTimeouts.Add(1)
				case strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY"):
					sqliteBusy.Add(1)
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
	require.Zerof(t, sqliteBusy.Load(),
		"%d/%d writes bypassed the write gate and hit SQLite's busy handler", sqliteBusy.Load(), total.Load())
	if timeouts := gateTimeouts.Load(); timeouts > 0 {
		stall := time.Duration(maxWork.Load())
		require.GreaterOrEqualf(t, stall, busy,
			"%d/%d writes waited out busy_timeout (%v) although a fair queue of %d writers is at most ~%v and no write was in flight longer than %v: a writer was starved",
			timeouts, total.Load(), busy, writers, (writers-1)*writeHold, stall)
		t.Logf("tolerated %d/%d queue timeouts behind a %v host stall", timeouts, total.Load(), stall)
	}
}

// TestOpenGormDBRoutesWritesThroughTheSharedGate pins the production wiring
// deterministically: two handles opened through openGormDB on one file share
// the file's writegate.Gate, so a write on the second waits in the queue
// instead of falling through to SQLite's busy handler. If openGormDB ever
// hands GORM the bare *sql.DB again, the second write gets SQLITE_BUSY (not
// writegate.ErrQueueTimeout) and this test fails without depending on timing
// or on a writer actually being starved.
func TestOpenGormDBRoutesWritesThroughTheSharedGate(t *testing.T) {
	const timeout = 100 * time.Millisecond
	dbPath := filepath.Join(t.TempDir(), "shared-gate.db")

	holder, err := openGormDB(dbPath, timeout, newGormLogger(io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := holder.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	waiter, err := openGormDB(dbPath, timeout, newGormLogger(io.Discard))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := waiter.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, holder.Exec("CREATE TABLE writes (id INTEGER PRIMARY KEY)").Error)

	holding := make(chan struct{})
	unblock := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- holder.Transaction(func(tx *gorm.DB) error {
			close(holding)
			<-unblock
			return nil
		})
	}()
	<-holding

	err = waiter.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO writes DEFAULT VALUES").Error
	})
	close(unblock)
	require.NoError(t, <-held)
	require.ErrorIs(t, err, writegate.ErrQueueTimeout,
		"a write on a second handle must queue on the file's gate; SQLITE_BUSY here means the gate was bypassed")
}
