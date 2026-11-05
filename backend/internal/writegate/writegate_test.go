package writegate

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// open returns a GORM handle on a fresh file database running on a Pool with
// the given gate timeout, with one table to write to.
func open(t *testing.T, timeout time.Duration) (*gorm.DB, *Gate) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gate.db")
	sqlDB, err := sql.Open(sqlite.DriverName, path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	gate := For(path, timeout)
	db, err := gorm.Open(sqlite.Dialector{Conn: NewPool(sqlDB, gate)}, &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE rows (id INTEGER PRIMARY KEY, v INTEGER NOT NULL)").Error)
	return db, gate
}

// queued reports how many writers are blocked waiting for the gate.
func queued(g *Gate) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.waiters.Len()
}

// starving reports whether the gate is in strict hand-over mode.
func starving(g *Gate) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.starving
}

// held reports whether the gate is currently taken, without waiting.
func held(g *Gate) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.held
}

func TestForSharesOneGatePerFile(t *testing.T) {
	dir := t.TempDir()
	a := For(filepath.Join(dir, "x.db"), time.Second)
	b := For(filepath.Join(dir, ".", "x.db"), time.Minute)
	c := For(filepath.Join(dir, "y.db"), time.Second)
	require.Same(t, a, b, "two handles on one file must queue on one gate")
	require.Equal(t, time.Second, b.timeout, "the first opener's timeout stands")
	require.NotSame(t, a, c)
}

func TestWaitersAreWokenInArrivalOrder(t *testing.T) {
	g := newGate(5 * time.Second)
	require.NoError(t, g.acquire(context.Background()))

	const waiters = 8
	var (
		mu    sync.Mutex
		order []int
		wg    sync.WaitGroup
	)
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			require.NoError(t, g.acquire(context.Background()))
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			g.release()
		}(i)
		// Let waiter i block on the gate before waiter i+1 arrives.
		require.Eventually(t, func() bool { return queued(g) == i+1 }, time.Second, time.Millisecond)
	}
	g.release()
	wg.Wait()
	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7}, order)
}

func TestAcquireTimesOutWithQueueTimeout(t *testing.T) {
	db, gate := open(t, 50*time.Millisecond)
	require.NoError(t, gate.acquire(context.Background()))
	defer gate.release()

	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO rows (v) VALUES (1)").Error
	})
	require.ErrorIs(t, err, ErrQueueTimeout)
	require.Contains(t, err.Error(), "database is locked", "string-matching busy detectors must still see it")

	require.ErrorIs(t, db.Exec("INSERT INTO rows (v) VALUES (2)").Error, ErrQueueTimeout,
		"an autocommit write queues too")
}

func TestAcquireReturnsContextError(t *testing.T) {
	db, gate := open(t, 5*time.Second)
	require.NoError(t, gate.acquire(context.Background()))
	defer gate.release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO rows (v) VALUES (1)").Error
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestTransactionHoldsTheGateUntilItEnds(t *testing.T) {
	for name, end := range map[string]error{"commit": nil, "rollback": errors.New("boom")} {
		t.Run(name, func(t *testing.T) {
			db, gate := open(t, time.Second)
			err := db.Transaction(func(tx *gorm.DB) error {
				require.True(t, held(gate), "the gate is held inside the transaction")
				require.NoError(t, tx.Exec("INSERT INTO rows (v) VALUES (1)").Error)
				return end
			})
			require.Equal(t, end, err)
			require.False(t, held(gate), "the gate is released when the transaction ends")
		})
	}
}

func TestNestedTransactionUsesASavepointNotTheGate(t *testing.T) {
	db, gate := open(t, 200*time.Millisecond)
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("INSERT INTO rows (v) VALUES (1)").Error; err != nil {
			return err
		}
		// A nested Transaction must reuse the held write lock; asking the gate
		// again would wait out its own timeout.
		inner := tx.Transaction(func(tx2 *gorm.DB) error {
			if err := tx2.Exec("INSERT INTO rows (v) VALUES (2)").Error; err != nil {
				return err
			}
			return errors.New("roll back to the savepoint")
		})
		require.EqualError(t, inner, "roll back to the savepoint")
		return nil
	})
	require.NoError(t, err)
	require.False(t, held(gate))

	var vs []int
	require.NoError(t, db.Raw("SELECT v FROM rows ORDER BY v").Scan(&vs).Error)
	require.Equal(t, []int{1}, vs, "the outer write committed and the savepoint rolled back")
}

func TestManualBeginCommitAndRollbackRelease(t *testing.T) {
	db, gate := open(t, time.Second)

	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.True(t, held(gate))
	require.NoError(t, tx.Exec("INSERT INTO rows (v) VALUES (1)").Error)
	require.NoError(t, tx.Commit().Error)
	require.False(t, held(gate))
	// A second Rollback after Commit (GORM's deferred-rollback idiom) must not
	// release a gate someone else now holds.
	require.NoError(t, gate.acquire(context.Background()))
	_ = tx.Rollback()
	require.True(t, held(gate), "a late Rollback must not release another writer's turn")
	gate.release()
}

func TestCancelledContextReleasesTheGate(t *testing.T) {
	db, gate := open(t, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	require.True(t, held(gate))
	cancel() // the caller went away without Commit or Rollback
	require.Eventually(t, func() bool { return !held(gate) }, time.Second, time.Millisecond)
}

func TestReadsDoNotWaitForTheGate(t *testing.T) {
	db, gate := open(t, 20*time.Millisecond)
	require.NoError(t, gate.acquire(context.Background()))
	defer gate.release()
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM rows").Scan(&n).Error)
	require.NoError(t, db.Table("rows").Count(&n).Error)
}

func TestDBReturnsTheUnderlyingHandle(t *testing.T) {
	db, _ := open(t, time.Second)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		inner, err := tx.DB()
		require.NoError(t, err)
		require.Same(t, sqlDB, inner)
		return nil
	}))
}

func TestTimedOutWaiterLeavesTheQueue(t *testing.T) {
	g := newGate(20 * time.Millisecond)
	require.NoError(t, g.acquire(context.Background()))
	require.ErrorIs(t, g.acquire(context.Background()), ErrQueueTimeout)
	require.Zero(t, queued(g), "a writer that gave up must not be handed the gate later")
	g.release()
	require.False(t, held(g))
	require.NoError(t, g.acquire(context.Background()), "the gate is free again")
	g.release()
}

// park starts acquire on g in the background and returns once it is queued.
func park(t *testing.T, g *Gate) <-chan error {
	t.Helper()
	before := queued(g)
	done := make(chan error, 1)
	go func() { done <- g.acquire(context.Background()) }()
	require.Eventually(t, func() bool { return queued(g) == before+1 }, time.Second, time.Millisecond)
	return done
}

func TestARunningWriterMayTakeAFreedGate(t *testing.T) {
	g := newGate(time.Minute)
	require.NoError(t, g.acquire(context.Background()))
	done := park(t, g)

	g.mu.Lock()
	g.releaseLocked() // normal mode: frees the gate and wakes the waiter
	require.False(t, g.held, "normal mode frees the gate rather than handing it over")
	g.held = true // a writer already on the CPU takes it before the waiter runs
	g.mu.Unlock()

	require.Eventually(t, func() bool { return queued(g) == 1 }, time.Second, time.Millisecond,
		"the woken waiter lost the race and queues again")
	require.False(t, starving(g), "it has not waited starveAfter yet")
	g.release()
	require.NoError(t, <-done)
	g.release()
	require.False(t, held(g))
}

func TestAnOvertakenWaiterSwitchesToStrictOrder(t *testing.T) {
	g := newGate(time.Minute)
	g.starveAfter = 0 // any lost race counts as starvation
	require.NoError(t, g.acquire(context.Background()))
	first := park(t, g)
	second := park(t, g)

	g.mu.Lock()
	g.releaseLocked()
	g.held = true // the first waiter is overtaken
	g.mu.Unlock()
	require.Eventually(t, func() bool { return starving(g) && queued(g) == 2 }, time.Second, time.Millisecond)

	require.ErrorIs(t, g.acquire(ctxWithin(t, 20*time.Millisecond)), context.DeadlineExceeded,
		"in starvation mode a new arrival queues instead of taking the gate")

	g.release() // hands the gate straight to the first waiter
	require.NoError(t, <-first)
	require.True(t, held(g))
	require.True(t, starving(g), "the second waiter is still queued and the first waited past starveAfter")
	g.release()
	require.NoError(t, <-second)
	require.False(t, starving(g), "the queue emptied")
	g.release()
	require.False(t, held(g))
}

func TestStarvationEndsForAFreshWaiter(t *testing.T) {
	g := newGate(time.Minute)
	require.NoError(t, g.acquire(context.Background()))
	first := park(t, g)
	second := park(t, g)
	g.mu.Lock()
	g.starving = true
	g.mu.Unlock()

	g.release() // the first waiter waited far less than starveAfter
	require.NoError(t, <-first)
	require.False(t, starving(g), "a hand-over to a writer that barely waited ends starvation mode")
	g.release()
	require.NoError(t, <-second)
	g.release()
}

// ctxWithin returns a context that ends after d.
func ctxWithin(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// A waiter that gives up in the same moment the gate is handed to it must
// pass the gate on rather than strand everyone behind it.
func TestHandoverRacingATimeoutIsNotLost(t *testing.T) {
	const timeout = 100 * time.Millisecond
	g := newGate(timeout)
	require.NoError(t, g.acquire(context.Background()))
	done := park(t, g)
	behind := park(t, g)
	g.mu.Lock()
	g.starving = true
	time.Sleep(2 * timeout) // both timers fire; the waiters block on mu
	g.releaseLocked()       // hand the gate to the first waiter as it gives up
	g.mu.Unlock()

	for _, ch := range []<-chan error{done, behind} {
		if err := <-ch; err == nil {
			// This waiter saw its turn before its timer: it owns the gate.
			require.True(t, held(g))
			g.release()
		} else {
			require.ErrorIs(t, err, ErrQueueTimeout)
		}
	}
	require.False(t, held(g), "the gate must not leak")
	require.Zero(t, queued(g))
}

// A waiter woken to compete for a free gate that gives up instead must wake
// the next waiter, or the free gate would sit unclaimed.
func TestWakeRacingATimeoutIsPassedOn(t *testing.T) {
	g := newGate(time.Minute)
	require.NoError(t, g.acquire(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() { gaveUp <- g.acquire(ctx) }()
	require.Eventually(t, func() bool { return queued(g) == 1 }, time.Second, time.Millisecond)
	next := park(t, g)

	g.mu.Lock()
	cancel()
	time.Sleep(50 * time.Millisecond) // the first waiter's ctx fires; it blocks on mu
	g.releaseLocked()                 // frees the gate and wakes the first waiter
	g.mu.Unlock()

	if err := <-gaveUp; err == nil {
		g.release() // it saw the wake first and took the gate
	} else {
		require.ErrorIs(t, err, context.Canceled)
	}
	require.NoError(t, <-next, "the next waiter must be woken")
	g.release()
	require.False(t, held(g))
}

func TestFailedBeginReleasesTheGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.db")
	sqlDB, err := sql.Open(sqlite.DriverName, path)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	gate := For(path, time.Second)
	_, err = NewPool(sqlDB, gate).BeginTx(context.Background(), nil)
	require.Error(t, err)
	require.False(t, held(gate))
}

func TestTxForwardsStatements(t *testing.T) {
	db, _ := open(t, time.Second)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	pool := NewPool(sqlDB, For(t.Name(), time.Second))
	ctx := context.Background()

	stmt, err := pool.PrepareContext(ctx, "SELECT count(*) FROM rows")
	require.NoError(t, err)
	defer stmt.Close()

	c, err := pool.BeginTx(ctx, nil)
	require.NoError(t, err)
	tx := c.(*Tx)
	_, err = tx.ExecContext(ctx, "INSERT INTO rows (v) VALUES (7)")
	require.NoError(t, err)

	var n int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM rows").Scan(&n))
	require.Equal(t, 1, n, "the transaction sees its own write")
	rows, err := tx.QueryContext(ctx, "SELECT v FROM rows")
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	txStmt, err := tx.PrepareContext(ctx, "SELECT v FROM rows")
	require.NoError(t, err)
	require.NoError(t, txStmt.Close())
	require.NoError(t, tx.StmtContext(ctx, stmt).QueryRowContext(ctx).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, tx.Rollback())
}

// Every starving waiter can give up, leaving starvation mode set with nobody
// to hand the gate to: the release must free it and end the mode.
func TestStarvationWithAnEmptyQueueFreesTheGate(t *testing.T) {
	g := newGate(time.Minute)
	require.NoError(t, g.acquire(context.Background()))
	g.mu.Lock()
	g.starving = true
	g.mu.Unlock()
	g.release()
	require.False(t, held(g))
	require.False(t, starving(g))
	require.NoError(t, g.acquire(context.Background()), "a new writer takes the free gate at once")
	g.release()
}
