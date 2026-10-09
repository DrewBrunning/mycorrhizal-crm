package auditwire

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openMem(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/a.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestWrap_NilSafeAndIdempotent(t *testing.T) {
	Wrap(nil)
	Wrap(&gorm.DB{})
	Wrap(&gorm.DB{Config: &gorm.Config{}}) // nil ConnPool: untouched

	db := openMem(t)
	Wrap(db)
	first := db.Config.ConnPool
	require.IsType(t, &Pool{}, first)
	Wrap(db)
	assert.Same(t, first, db.Config.ConnPool, "second Wrap must not double-wrap")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.NoError(t, sqlDB.Ping())
}

func TestPool_GetDBConnFallbacks(t *testing.T) {
	raw := openMem(t)
	sqlDB, err := raw.DB()
	require.NoError(t, err)

	p := &Pool{ConnPool: sqlDB} // *sql.DB branch
	got, err := p.GetDBConn()
	require.NoError(t, err)
	assert.Same(t, sqlDB, got)

	p = &Pool{ConnPool: &Pool{ConnPool: sqlDB}} // GetDBConnector branch
	got, err = p.GetDBConn()
	require.NoError(t, err)
	assert.Same(t, sqlDB, got)

	p = &Pool{ConnPool: &Tx{}} // neither
	_, err = p.GetDBConn()
	assert.ErrorIs(t, err, gorm.ErrInvalidDB)
}

// fakePool is a ConnPoolBeginner that is not a TxBeginner.
type fakePool struct {
	gorm.ConnPool
	inner gorm.ConnPool
	err   error
}

func (f fakePool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return f.inner, f.err
}

type fakeCommitter struct {
	gorm.ConnPool
	commits, rollbacks int
}

func (f *fakeCommitter) Commit() error   { f.commits++; return nil }
func (f *fakeCommitter) Rollback() error { f.rollbacks++; return nil }

type plainPool struct{ gorm.ConnPool }

func TestPool_BeginTxBranches(t *testing.T) {
	ctx := context.Background()

	// default: neither beginner
	_, err := (&Pool{ConnPool: &Tx{}}).BeginTx(ctx, nil)
	assert.ErrorIs(t, err, gorm.ErrInvalidTransaction)

	// ConnPoolBeginner returning an error
	boom := errors.New("boom")
	_, err = (&Pool{ConnPool: fakePool{err: boom}}).BeginTx(ctx, nil)
	assert.ErrorIs(t, err, boom)

	// ConnPoolBeginner returning a non-committer is passed through unwrapped
	plain := &plainPool{}
	got, err := (&Pool{ConnPool: fakePool{inner: plain}}).BeginTx(ctx, nil)
	require.NoError(t, err)
	assert.Same(t, plain, got)

	// ConnPoolBeginner returning a committer is wrapped and gated
	fc := &fakeCommitter{}
	got, err = (&Pool{ConnPool: fakePool{inner: fc}}).BeginTx(ctx, nil)
	require.NoError(t, err)
	tx := got.(*Tx)
	ran := 0
	tx.pending = append(tx.pending, func() { ran++ })
	require.NoError(t, tx.Commit())
	assert.Equal(t, 1, ran)
	assert.Equal(t, 1, fc.commits)

	// TxBeginner error (closed DB)
	raw := openMem(t)
	sqlDB, err := raw.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	_, err = (&Pool{ConnPool: sqlDB}).BeginTx(ctx, nil)
	assert.Error(t, err)
}

func TestTx_CommitRunsRollbackDropsFailedCommitDrops(t *testing.T) {
	db := openMem(t)
	Wrap(db)
	require.NoError(t, db.Exec("CREATE TABLE t(id integer)").Error)

	ran := 0
	run := func(commit bool) {
		tx := db.Begin()
		require.NoError(t, tx.Error)
		require.True(t, DeferOn(tx, func() { ran++ }))
		if commit {
			require.NoError(t, tx.Commit().Error)
		} else {
			require.NoError(t, tx.Rollback().Error)
		}
	}
	run(true)
	assert.Equal(t, 1, ran)
	run(false)
	assert.Equal(t, 1, ran, "rollback must drop buffered work")

	// A failed commit (already rolled back at the sql layer) must drop too.
	tx := db.Begin()
	require.NoError(t, tx.Error)
	require.True(t, DeferOn(tx, func() { ran++ }))
	require.NoError(t, tx.Statement.ConnPool.(*Tx).ConnPool.(*sql.Tx).Rollback())
	assert.ErrorIs(t, tx.Commit().Error, sql.ErrTxDone)
	assert.Equal(t, 1, ran, "failed commit must drop buffered work")
}

func TestTx_SavepointRollbackTruncatesOnlyInner(t *testing.T) {
	db := openMem(t)
	Wrap(db)
	var order []string
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		require.True(t, DeferOn(tx, func() { order = append(order, "outer") }))
		_ = tx.Transaction(func(tx2 *gorm.DB) error {
			require.True(t, DeferOn(tx2, func() { order = append(order, "inner") }))
			return errors.New("rollback inner")
		})
		require.True(t, DeferOn(tx, func() { order = append(order, "after") }))
		return nil
	}))
	assert.Equal(t, []string{"outer", "after"}, order)

	// Unknown savepoint name in ROLLBACK TO leaves pending alone.
	tx := &Tx{}
	tx.pending = []func(){func() {}}
	tx.trackSavepoint("ROLLBACK TO SAVEPOINT nope")
	assert.Len(t, tx.pending, 1)
	tx.trackSavepoint("select 1")
	assert.Len(t, tx.pending, 1)
}

func TestDeferOn_NotInWrappedTransaction(t *testing.T) {
	assert.False(t, DeferOn(nil, func() {}))
	assert.False(t, DeferOn(&gorm.DB{}, func() {}))
	db := openMem(t) // never wrapped
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback() //nolint:errcheck
	assert.False(t, DeferOn(tx, func() {}))
	assert.False(t, DeferOn(db, func() {}), "outside any transaction")
}

func TestDefaultInitialize_WrapsPool(t *testing.T) {
	db := openMem(t)
	require.NoError(t, db.Use(&Default{}))
	assert.IsType(t, &Pool{}, db.Config.ConnPool)
}
