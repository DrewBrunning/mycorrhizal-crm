package auditwire

import (
	"context"
	"database/sql"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// Transaction-aware connection pool (issue #1547).
//
// GORM fires AfterCreate/AfterSave/AfterDelete hooks inside the write's
// transaction, before it commits. The async audit recorder must not persist an
// event for a write whose transaction later rolls back, but GORM exposes no
// "transaction finished" hook for an explicit db.Transaction / db.Begin block
// (it calls Commit/Rollback on the transaction's ConnPool directly). So the
// recorder wraps the root handle's ConnPool: BeginTx hands GORM a *Tx whose
// Commit/Rollback we own. The audit hook buffers its insert on that *Tx with
// DeferOn; Commit runs the buffered work only after the underlying commit
// succeeded, Rollback (or a failed commit) discards it. This one seam covers an
// implicit single-statement transaction and an explicit block alike, with no
// change at any call site.
//
// A SAVEPOINT / ROLLBACK TO SAVEPOINT issued through the *Tx (GORM's nested
// db.Transaction) is tracked too, so work buffered inside a rolled-back inner
// block is dropped while the outer block's own work survives.

// Wrap replaces db's ConnPool with a transaction-aware one. It is idempotent.
// Handles derived from db afterwards use it; handles derived earlier keep the
// raw pool and so are treated as "not in a wrapped transaction" by DeferOn
// (callers fall back to immediate execution).
func Wrap(db *gorm.DB) {
	if db == nil || db.Config == nil { //nolint:staticcheck // QF1008: the explicit Config nil check guards the promoted ConnPool access below
		return
	}
	poolWrapMu.Lock()
	defer poolWrapMu.Unlock()
	if _, ok := db.ConnPool.(*Pool); ok || db.ConnPool == nil {
		return
	}
	p := &Pool{ConnPool: db.ConnPool}
	db.ConnPool = p
	if db.Statement != nil {
		db.Statement.ConnPool = p
	}
}

var poolWrapMu sync.Mutex

// Pool wraps a gorm.ConnPool and returns a *Tx from BeginTx.
type Pool struct {
	gorm.ConnPool
}

// BeginTx satisfies gorm.ConnPoolBeginner.
func (p *Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	switch b := p.ConnPool.(type) {
	case gorm.TxBeginner:
		tx, err := b.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &Tx{ConnPool: tx, commit: tx.Commit, rollback: tx.Rollback}, nil
	case gorm.ConnPoolBeginner:
		inner, err := b.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}
		c, ok := inner.(gorm.TxCommitter)
		if !ok {
			return inner, nil
		}
		return &Tx{ConnPool: inner, commit: c.Commit, rollback: c.Rollback}, nil
	default:
		return nil, gorm.ErrInvalidTransaction
	}
}

// GetDBConn satisfies gorm.GetDBConnector so db.DB() keeps working.
func (p *Pool) GetDBConn() (*sql.DB, error) {
	if c, ok := p.ConnPool.(gorm.GetDBConnector); ok {
		return c.GetDBConn()
	}
	if d, ok := p.ConnPool.(*sql.DB); ok {
		return d, nil
	}
	return nil, gorm.ErrInvalidDB
}

// Tx is a transaction whose commit outcome gates buffered work.
type Tx struct {
	gorm.ConnPool
	commit   func() error
	rollback func() error

	mu        sync.Mutex
	pending   []func()
	savepoint map[string]int // savepoint name -> len(pending) when taken
}

// Commit commits, then runs the buffered work only if the commit succeeded.
func (t *Tx) Commit() error {
	err := t.commit()
	t.mu.Lock()
	fns := t.pending
	t.pending, t.savepoint = nil, nil
	t.mu.Unlock()
	if err == nil {
		for _, fn := range fns {
			fn()
		}
	}
	return err
}

// Rollback rolls back and discards the buffered work.
func (t *Tx) Rollback() error {
	t.mu.Lock()
	t.pending, t.savepoint = nil, nil
	t.mu.Unlock()
	return t.rollback()
}

// ExecContext tracks SAVEPOINT / ROLLBACK TO SAVEPOINT so a rolled-back nested
// block discards its own buffered work.
func (t *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	res, err := t.ConnPool.ExecContext(ctx, query, args...)
	if err == nil {
		t.trackSavepoint(query)
	}
	return res, err
}

func (t *Tx) trackSavepoint(query string) {
	q := strings.TrimSpace(query)
	upper := strings.ToUpper(q)
	const rollbackTo = "ROLLBACK TO SAVEPOINT "
	const savepoint = "SAVEPOINT "
	switch {
	case strings.HasPrefix(upper, rollbackTo):
		name := strings.TrimSpace(q[len(rollbackTo):])
		t.mu.Lock()
		if n, ok := t.savepoint[name]; ok && n <= len(t.pending) {
			t.pending = t.pending[:n]
		}
		t.mu.Unlock()
	case strings.HasPrefix(upper, savepoint):
		name := strings.TrimSpace(q[len(savepoint):])
		t.mu.Lock()
		if t.savepoint == nil {
			t.savepoint = map[string]int{}
		}
		t.savepoint[name] = len(t.pending)
		t.mu.Unlock()
	}
}

// DeferOn queues fn on the wrapped transaction tx is running inside and
// reports true; when tx is not inside a wrapped transaction it reports false
// and the caller must run the work itself.
func DeferOn(tx *gorm.DB, fn func()) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}
	t, ok := tx.Statement.ConnPool.(*Tx)
	if !ok {
		return false
	}
	t.mu.Lock()
	t.pending = append(t.pending, fn)
	t.mu.Unlock()
	return true
}
