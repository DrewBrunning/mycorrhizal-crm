// Package writegate serves SQLite writers in arrival order.
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
// A Gate puts a FIFO queue in front of the write lock inside this process: a
// GORM transaction (and therefore its BEGIN IMMEDIATE) starts only once the
// gate is held, and the gate passes to the longest waiter when the transaction
// commits or rolls back. The queue is an explicit list under a mutex rather
// than a channel's blocked senders, whose order the Go spec does not promise. SQLite's own locking still applies on
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

// Gate is a FIFO write queue for one database file.
type Gate struct {
	timeout time.Duration

	mu      sync.Mutex
	held    bool
	waiters list.List // of chan struct{}, closed to hand the gate over
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
	g := &Gate{timeout: timeout}
	registry[key] = g
	return g
}

// acquire waits for the gate in arrival order. It gives up with ctx's error
// when the caller's context ends, or with ErrQueueTimeout after the gate's
// timeout.
func (g *Gate) acquire(ctx context.Context) error {
	g.mu.Lock()
	if !g.held && g.waiters.Len() == 0 {
		g.held = true
		g.mu.Unlock()
		return nil
	}
	turn := make(chan struct{})
	e := g.waiters.PushBack(turn)
	g.mu.Unlock()

	timer := time.NewTimer(g.timeout)
	defer timer.Stop()
	var err error
	select {
	case <-turn:
		return nil
	case <-ctx.Done():
		err = ctx.Err()
	case <-timer.C:
		err = ErrQueueTimeout
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-turn:
		// The gate was handed over as we gave up: pass it on, or the
		// writers behind us would wait for a release that never comes.
		g.releaseLocked()
	default:
		g.waiters.Remove(e)
	}
	return err
}

// release hands the gate to the longest waiter, or frees it.
func (g *Gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.releaseLocked()
}

func (g *Gate) releaseLocked() {
	if front := g.waiters.Front(); front != nil {
		g.waiters.Remove(front)
		close(front.Value.(chan struct{})) // g.held stays true: ownership moves
		return
	}
	g.held = false
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
