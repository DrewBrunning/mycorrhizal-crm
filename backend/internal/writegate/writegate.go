// Package writegate keeps SQLite writers from being starved of the write lock.
//
// SQLite allows one writer at a time, and the app's DSN makes every
// transaction take the write lock up front (_txlock=immediate, backend trap 9).
// A writer that finds the lock held waits in SQLite's busy handler, which is a
// polling sleep (1, 2, 5, … up to 100 ms between retries), not a queue: the
// writer that has waited longest sleeps longest, and writers arriving between
// its polls take the lock first. Under steady contention on slow storage one
// writer can wait out the whole busy_timeout and fail with SQLITE_BUSY (a 503
// DATABASE_BUSY) while the queue ahead of it at any moment was short.
//
// A Gate puts a queue in front of the write lock inside this process: a GORM
// transaction (and therefore its BEGIN IMMEDIATE) starts only once the gate is
// held, and the longest waiter is woken when the transaction commits or rolls
// back. Like sync.Mutex, it lets a running writer take a free gate first for
// throughput, and switches to strict arrival order once a waiter has been
// overtaken for a tenth of the timeout, so none is starved into SQLITE_BUSY. SQLite's own locking still applies on
// top, so writers outside the gate — another process, a raw *sql.DB
// transaction — stay correct, just not queued.
package writegate

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"
)

// ErrQueueTimeout is returned when a writer waited the gate's whole timeout
// without its turn. It means the same thing as SQLite's SQLITE_BUSY after
// busy_timeout (overload, retry shortly), so errors.IsDatabaseBusy maps it to
// the same 503, and its text keeps the "database is locked" marker that
// string-matching callers (the job lock retry, loadsmoke, perfbench) look for.
var ErrQueueTimeout = errors.New("database is locked (timed out waiting for the write queue)")

// Gate is a write queue for one database file. It runs like sync.Mutex:
//
//   - Normal mode: a release frees the gate and wakes the longest waiter, which
//     competes with writers arriving at that moment. A writer that is already
//     running usually wins, so the write lock does not idle while a woken
//     goroutine is scheduled.
//   - Starvation mode: once a woken writer loses after waiting starveAfter, each
//     release hands the gate straight to the longest waiter and new arrivals
//     queue behind it, so no writer can be overtaken past starveAfter plus the
//     queue ahead of it. The gate returns to normal mode when the queue empties
//     or a writer is handed the gate having waited less than starveAfter.
type Gate struct {
	timeout     time.Duration
	starveAfter time.Duration

	mu       sync.Mutex
	held     bool
	starving bool
	waiters  list.List // of *waiter, longest-waiting first
}

// waiter is one writer blocked in acquire.
type waiter struct {
	wake    chan struct{} // closed when this writer is woken or handed the gate
	granted bool          // the gate was handed over, not just freed
	since   time.Time     // when the writer started waiting, kept across re-queues
}

// starvationFraction sets starveAfter as a share of the timeout: a writer is
// overtaken for at most a tenth of its budget before it is served in order.
const starvationFraction = 10

func newGate(timeout time.Duration) *Gate {
	return &Gate{timeout: timeout, starveAfter: timeout / starvationFraction}
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Gate{}
)

// For returns the process-wide gate for the database file at path, creating it
// with timeout on first use. Every handle on the same file must share one gate
// or two handles would each admit a writer and SQLite's unfair busy handler
// would arbitrate between them again.
func For(path string, timeout time.Duration) *Gate {
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if g, ok := registry[key]; ok {
		return g
	}
	g := newGate(timeout)
	registry[key] = g
	return g
}

// acquire waits for the gate. It gives up with ctx's error when the caller's
// context ends, or with ErrQueueTimeout after the gate's timeout.
func (g *Gate) acquire(ctx context.Context) error {
	g.mu.Lock()
	if !g.held && !g.starving {
		g.held = true
		g.mu.Unlock()
		return nil
	}
	w := &waiter{since: time.Now()}
	g.enqueueLocked(w, false)
	g.mu.Unlock()

	timer := time.NewTimer(g.timeout)
	defer timer.Stop()
	for {
		select {
		case <-w.wake:
			g.mu.Lock()
			if w.granted {
				g.mu.Unlock()
				return nil
			}
			// Woken by a release in normal mode: compete for the free gate.
			if !g.held {
				g.held = true
				g.mu.Unlock()
				return nil
			}
			// A running writer took it first. Waited too long: stop that.
			if time.Since(w.since) >= g.starveAfter {
				g.starving = true
			}
			g.enqueueLocked(w, true)
			g.mu.Unlock()
		case <-ctx.Done():
			return g.giveUp(w, ctx.Err())
		case <-timer.C:
			return g.giveUp(w, ErrQueueTimeout)
		}
	}
}

// enqueueLocked queues w with a fresh wake channel: at the back when it has
// just arrived, at the front when it was woken and lost the race, since it
// has still waited longest.
func (g *Gate) enqueueLocked(w *waiter, front bool) {
	w.wake = make(chan struct{})
	if front {
		g.waiters.PushFront(w)
	} else {
		g.waiters.PushBack(w)
	}
}

// giveUp takes w out of the queue after its timeout or context ended.
func (g *Gate) giveUp(w *waiter, err error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-w.wake:
		if w.granted {
			// The gate was handed over as we gave up: pass it on, or the
			// writers behind us would wait for a release that never comes.
			g.releaseLocked()
		} else if !g.held {
			// We were woken to compete for a free gate: wake the next
			// waiter in our place.
			g.wakeFrontLocked()
		}
	default:
		for e := g.waiters.Front(); e != nil; e = e.Next() {
			if e.Value.(*waiter) == w {
				g.waiters.Remove(e)
				break
			}
		}
	}
	return err
}

// release frees the gate or hands it to the longest waiter.
func (g *Gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseLocked()
}

func (g *Gate) releaseLocked() {
	if g.starving {
		if front := g.waiters.Front(); front != nil {
			g.waiters.Remove(front)
			w := front.Value.(*waiter)
			w.granted = true // g.held stays true: ownership moves
			close(w.wake)
			if g.waiters.Len() == 0 || time.Since(w.since) < g.starveAfter {
				g.starving = false
			}
			return
		}
		g.starving = false
	}
	g.held = false
	g.wakeFrontLocked()
}

// wakeFrontLocked wakes the longest waiter to compete for the free gate.
func (g *Gate) wakeFrontLocked() {
	if front := g.waiters.Front(); front != nil {
		g.waiters.Remove(front)
		close(front.Value.(*waiter).wake)
	}
}

// Pool is the gorm.ConnPool the app's GORM handle runs on. Reads go straight
// to the *sql.DB (WAL keeps them lock-free); transactions and autocommit Exec
// statements wait for the gate first.
//
// It deliberately implements gorm.ConnPoolBeginner and not gorm.TxBeginner:
// GORM picks TxBeginner first, and a bare *sql.Tx gives no hook to release the
// gate at commit.
type Pool struct {
	db   *sql.DB
	gate *Gate
}

// NewPool wraps db so its writers queue on gate.
func NewPool(db *sql.DB, gate *Gate) *Pool { return &Pool{db: db, gate: gate} }

// PrepareContext implements gorm.ConnPool.
func (p *Pool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return p.db.PrepareContext(ctx, query)
}

// ExecContext implements gorm.ConnPool. An Exec outside a transaction is an
// autocommit write (GORM wraps its own creates, updates and deletes in a
// transaction, so this is raw SQL), so it queues like a transaction does.
func (p *Pool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if err := p.gate.acquire(ctx); err != nil {
		return nil, err
	}
	defer p.gate.release()
	return p.db.ExecContext(ctx, query, args...)
}

// QueryContext implements gorm.ConnPool.
func (p *Pool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

// QueryRowContext implements gorm.ConnPool.
func (p *Pool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

// BeginTx implements gorm.ConnPoolBeginner: it waits for the gate, then begins
// the transaction. The returned Tx holds the gate until Commit, Rollback, or
// the end of ctx (database/sql rolls the transaction back then too).
func (p *Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	if err := p.gate.acquire(ctx); err != nil {
		return nil, err
	}
	tx, err := p.db.BeginTx(ctx, opts)
	if err != nil {
		p.gate.release()
		return nil, err
	}
	t := &Tx{tx: tx, db: p.db}
	var once sync.Once
	t.release = func() { once.Do(p.gate.release) }
	t.stopWatch = context.AfterFunc(ctx, t.release)
	return t, nil
}

// GetDBConn implements gorm.GetDBConnector, so gorm.DB.DB() still returns the
// underlying *sql.DB.
func (p *Pool) GetDBConn() (*sql.DB, error) { return p.db, nil }

// Ping lets gorm.Open verify the connection, as it does for a bare *sql.DB.
// GORM's pinger interface carries no context, and gorm.Open is startup code
// with none to thread.
func (p *Pool) Ping() error { return p.db.PingContext(context.Background()) }

// Tx is a transaction that holds its pool's gate until it ends.
type Tx struct {
	tx        *sql.Tx
	db        *sql.DB
	release   func()
	stopWatch func() bool
}

// PrepareContext implements gorm.ConnPool.
func (t *Tx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, query)
}

// ExecContext implements gorm.ConnPool.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

// QueryContext implements gorm.ConnPool.
func (t *Tx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}

// QueryRowContext implements gorm.ConnPool.
func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

// StmtContext implements gorm.Tx.
func (t *Tx) StmtContext(ctx context.Context, stmt *sql.Stmt) *sql.Stmt {
	return t.tx.StmtContext(ctx, stmt)
}

// Commit implements gorm.TxCommitter and releases the gate.
func (t *Tx) Commit() error {
	defer t.done()
	return t.tx.Commit()
}

// Rollback implements gorm.TxCommitter and releases the gate. Rolling back a
// transaction that already ended (GORM does after a failed commit) still
// returns sql.ErrTxDone and releases nothing twice.
func (t *Tx) Rollback() error {
	defer t.done()
	return t.tx.Rollback()
}

func (t *Tx) done() {
	t.stopWatch()
	t.release()
}

// GetDBConn implements gorm.GetDBConnector, so gorm.DB.DB() works inside a
// transaction as it does for a *sql.Tx.
func (t *Tx) GetDBConn() (*sql.DB, error) { return t.db, nil }
