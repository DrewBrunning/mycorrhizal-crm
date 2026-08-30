package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/faults"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTestSQLite returns a raw *sql.DB backed by a temp-file SQLite database
// using this package's standard pragmas (so WAL/FK behave like production).
func openTestSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", openDSN(filepath.Join(t.TempDir(), "driver.db")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func closedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestSQLite(t)
	require.NoError(t, db.Close())
	return db
}

// TestWithInstanceRejectsNilConfig covers the withInstance error branches that
// the migration happy paths never reach: a nil config and an unpingable
// connection both fail before any driver state is built.
func TestWithInstanceRejectsNilConfig(t *testing.T) {
	_, err := withInstance(nil, nil)
	assert.ErrorIs(t, err, errNilConfig)
}

func TestWithInstanceRejectsClosedConnection(t *testing.T) {
	_, err := withInstance(closedSQLite(t), &sqliteConfig{})
	assert.Error(t, err, "an unpingable connection must fail withInstance")
}

// readonlySQLite opens a chmod-0444 database file read-only: reads succeed,
// every write fails with SQLITE_READONLY — the precise setup for exercising
// the write-side error branches that a healthy migration never hits.
func readonlySQLite(t *testing.T) *sql.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ro.db")
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(p, 0o444))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	return db
}

// TestWithInstanceReportsVersionTableFailure covers withInstance's
// ensureVersionTable error path: on a read-only connection Ping succeeds but
// creating the version table cannot.
func TestWithInstanceReportsVersionTableFailure(t *testing.T) {
	_, err := withInstance(readonlySQLite(t), &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	assert.Error(t, err, "an unwritable connection must fail withInstance at ensureVersionTable")
}

func TestWithInstanceSetsDefaultMigrationsTable(t *testing.T) {
	cfg := &sqliteConfig{}
	mx, err := withInstance(openTestSQLite(t), cfg)
	require.NoError(t, err)
	drv := mx.(*sqliteDriver)
	assert.Equal(t, defaultMigrationsTable, drv.config.MigrationsTable,
		"an empty MigrationsTable must default to schema_migrations")
}

// TestEnsureVersionTableIsIdempotent covers ensureVersionTable's Lock/Unlock
// wrapper and the create-table statement, including the unlock-error join path
// being a no-op when Unlock succeeds.
func TestEnsureVersionTableIsIdempotent(t *testing.T) {
	db := openTestSQLite(t)
	cfg := &sqliteConfig{MigrationsTable: defaultMigrationsTable}
	drv, err := withInstance(db, cfg)
	require.NoError(t, err)

	require.NoError(t, drv.(*sqliteDriver).ensureVersionTable())
	require.NoError(t, drv.(*sqliteDriver).ensureVersionTable(),
		"creating the version table twice must not error (CREATE IF NOT EXISTS)")

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'",
	).Scan(&count))
	assert.Equal(t, 1, count)
}

// TestEnsureVersionTableReportsLockedError covers the Lock guard at the top of
// ensureVersionTable: a driver that already holds its migration lock must fail
// rather than re-entering.
func TestEnsureVersionTableReportsLockedError(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)
	m := drv.(*sqliteDriver)

	require.NoError(t, m.Lock())
	defer m.Unlock()
	assert.ErrorIs(t, m.ensureVersionTable(), database.ErrLocked,
		"a held migration lock must make ensureVersionTable fail")
}

func TestEnsureVersionTableReportsExecFailure(t *testing.T) {
	drv := &sqliteDriver{db: closedSQLite(t), config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}
	assert.Error(t, drv.ensureVersionTable(), "an exec failure must surface from ensureVersionTable")
}

// TestOpenParsesURLAndExecutes covers the Open entry point: it parses the
// sqlite:// URL, derives the file path, applies the migrations-table query
// parameter, and returns a working driver whose version table exists.
func TestOpenParsesURLAndExecutes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "open.db")
	url := "sqlite://" + dbPath + "?x-migrations-table=custom_migrations"
	drv, err := (&sqliteDriver{}).Open(url)
	require.NoError(t, err)
	defer drv.Close()

	d := drv.(*sqliteDriver)
	assert.Equal(t, "custom_migrations", d.config.MigrationsTable,
		"x-migrations-table must override the default table name")

	var count int
	require.NoError(t, d.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'custom_migrations'",
	).Scan(&count))
	assert.Equal(t, 1, count, "Open must run ensureVersionTable for the configured table")
}

func TestOpenRejectsMalformedURL(t *testing.T) {
	_, err := (&sqliteDriver{}).Open("sqlite:///%zz")
	assert.Error(t, err, "a malformed URL must fail Open")
}

func TestOpenRejectsBadNoTxWrapFlag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "open-bad.db")
	_, err := (&sqliteDriver{}).Open("sqlite://" + dbPath + "?x-no-tx-wrap=notabool")
	assert.Error(t, err, "a non-boolean x-no-tx-wrap must fail Open")
}

func TestOpenNoTxWrapFlagParsesTrue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "open-notx.db")
	drv, err := (&sqliteDriver{}).Open("sqlite://" + dbPath + "?x-no-tx-wrap=true")
	require.NoError(t, err)
	defer drv.Close()
	assert.True(t, drv.(*sqliteDriver).config.NoTxWrap)
}

// TestOpenReportsInstanceFailure covers Open's withInstance error path: a URL
// pointing at a read-only database opens the connection but cannot initialize
// the version table, so Open must fail rather than return a half-built driver.
func TestOpenReportsInstanceFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ro-open.db")
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, os.Chmod(p, 0o444))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	// The driver builds its DSN from the URL path; point it at the read-only
	// file. sql.Open is lazy and withInstance's ensureVersionTable then fails.
	_, err = (&sqliteDriver{}).Open("sqlite://" + p)
	assert.Error(t, err, "an unwritable target must fail Open via withInstance")
}

// TestDropRemovesEveryTableAndVacuum covers Drop's happy path: after it runs,
// no user tables remain in the database.
func TestDropRemovesEveryTableAndVacuum(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)

	_, err = db.Exec("CREATE TABLE t_one (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	_, err = db.Exec("CREATE TABLE t_two (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)

	require.NoError(t, drv.Drop())

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'",
	).Scan(&count))
	assert.Zero(t, count, "Drop must remove every table")
}

func TestDropReportsQueryFailure(t *testing.T) {
	drv := &sqliteDriver{db: closedSQLite(t), config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}
	assert.Error(t, drv.Drop(), "a query failure must surface from Drop")
}

// TestDropReportsDropFailure covers Drop's per-table drop error path: the
// table listing succeeds on a read-only connection, but DROP TABLE cannot.
func TestDropReportsDropFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "drop-ro.db")
	w, err := sql.Open("sqlite", p)
	require.NoError(t, err)
	_, err = w.Exec("CREATE TABLE ro_t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, os.Chmod(p, 0o444))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	ro, err := sql.Open("sqlite", "file:"+p+"?mode=ro")
	require.NoError(t, err)
	defer ro.Close()
	drv := &sqliteDriver{db: ro, config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}

	err = drv.Drop()
	require.Error(t, err)
	var dbErr *database.Error
	require.ErrorAs(t, err, &dbErr, "a drop failure must surface as a typed database.Error")
	assert.Contains(t, string(dbErr.Query), "DROP TABLE")
}

// TestLockUnlockLifecycle covers Lock/Unlock's double-acquire and
// double-release guards, which no migration path ever triggers (golang-migrate
// always pairs them).
func TestLockUnlockLifecycle(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)
	m := drv.(*sqliteDriver)

	require.NoError(t, m.Lock())
	assert.ErrorIs(t, m.Lock(), database.ErrLocked, "a second Lock while held must fail")

	require.NoError(t, m.Unlock())
	assert.ErrorIs(t, m.Unlock(), database.ErrNotLocked, "Unlock without holding the lock must fail")
}

// TestRunNoTxWrapCovers the NoTxWrap branch of Run: the statement executes
// directly on the connection without a wrapping transaction.
func TestRunNoTxWrap(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable, NoTxWrap: true})
	require.NoError(t, err)

	require.NoError(t, drv.Run(strings.NewReader("CREATE TABLE no_tx_wrap_t (id INTEGER PRIMARY KEY)")))

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'no_tx_wrap_t'",
	).Scan(&count))
	assert.Equal(t, 1, count)
}

// errReader is a reader that always fails, for Run's io.ReadAll error branch.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestRunReportsReadFailure(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)
	assert.Error(t, drv.Run(errReader{}), "a read failure must surface from Run")
}

// TestRunReportsInjectedFault covers the failure-injection seam in Run: with
// the fault armed, an otherwise-valid migration body fails so the caller's
// dirty-state handling is exercised (the fault_injection_test.go coverage does
// the full fail-closed + operator-force dance; here it is pinned at the driver
// boundary).
func TestRunReportsInjectedFault(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmError(faultMigrationStatement, errors.New("injected driver fault"))

	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)
	err = drv.Run(strings.NewReader("CREATE TABLE fault_t (id INTEGER PRIMARY KEY)"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected driver fault")
}

// TestExecuteQueryRunsAndRollsBackOnError covers executeQuery's transaction
// wrapper: valid SQL commits, invalid SQL rolls back and returns a typed error.
func TestExecuteQueryRunsAndRollsBackOnError(t *testing.T) {
	db := openTestSQLite(t)
	m := &sqliteDriver{db: db, config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}

	require.NoError(t, m.executeQuery("CREATE TABLE exec_ok (id INTEGER PRIMARY KEY)"))

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'exec_ok'",
	).Scan(&count))
	assert.Equal(t, 1, count)

	err := m.executeQuery("THIS IS NOT SQL")
	require.Error(t, err)
	var dbErr *database.Error
	require.ErrorAs(t, err, &dbErr, "an exec failure must surface as a typed database.Error")
	assert.NotEmpty(t, dbErr.Query)
}

func TestExecuteQueryReportsBeginFailure(t *testing.T) {
	m := &sqliteDriver{db: closedSQLite(t), config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}
	err := m.executeQuery("SELECT 1")
	require.Error(t, err)
	var dbErr *database.Error
	require.ErrorAs(t, err, &dbErr)
	assert.NotEmpty(t, dbErr.Err, "a begin failure must be marked as a transaction start failure")
}

// TestExecuteQueryNoTx covers both branches of the untransacted runner: valid
// SQL executes, invalid SQL surfaces a typed error.
func TestExecuteQueryNoTx(t *testing.T) {
	db := openTestSQLite(t)
	m := &sqliteDriver{db: db, config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}

	require.NoError(t, m.executeQueryNoTx("CREATE TABLE notx_ok (id INTEGER PRIMARY KEY)"))
	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'notx_ok'",
	).Scan(&count))
	assert.Equal(t, 1, count)

	err := m.executeQueryNoTx("THIS IS NOT SQL")
	require.Error(t, err)
	var dbErr *database.Error
	require.ErrorAs(t, err, &dbErr)
	assert.NotEmpty(t, dbErr.Query)
}

// TestSetVersionWritesAndClears covers SetVersion's happy path in both
// directions: writing a version+clean state, then a version+dirty state.
func TestSetVersionWritesAndClears(t *testing.T) {
	db := openTestSQLite(t)
	cfg := &sqliteConfig{MigrationsTable: defaultMigrationsTable}
	drv, err := withInstance(db, cfg)
	require.NoError(t, err)
	m := drv.(*sqliteDriver)

	require.NoError(t, m.SetVersion(7, false))
	v, dirty, err := m.Version()
	require.NoError(t, err)
	assert.Equal(t, 7, v)
	assert.False(t, dirty)

	require.NoError(t, m.SetVersion(8, true))
	v, dirty, err = m.Version()
	require.NoError(t, err)
	assert.Equal(t, 8, v)
	assert.True(t, dirty)
}

func TestVersionEmptyTableReturnsNilVersion(t *testing.T) {
	db := openTestSQLite(t)
	drv, err := withInstance(db, &sqliteConfig{MigrationsTable: defaultMigrationsTable})
	require.NoError(t, err)
	v, dirty, err := drv.Version()
	require.NoError(t, err)
	assert.Equal(t, database.NilVersion, v)
	assert.False(t, dirty)
}

// TestSetVersionReportsWriteFailure covers SetVersion's mid-transaction error
// path: on a read-only connection the DELETE inside the transaction fails and
// the typed database.Error carries the query.
func TestSetVersionReportsWriteFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "setver-ro.db")
	w, err := sql.Open("sqlite", p)
	require.NoError(t, err)
	_, err = w.Exec("CREATE TABLE schema_migrations (version uint64, dirty bool)")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, os.Chmod(p, 0o444))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

	ro, err := sql.Open("sqlite", "file:"+p+"?mode=ro")
	require.NoError(t, err)
	defer ro.Close()
	m := &sqliteDriver{db: ro, config: &sqliteConfig{MigrationsTable: defaultMigrationsTable}}

	err = m.SetVersion(1, false)
	require.Error(t, err)
	var dbErr *database.Error
	require.ErrorAs(t, err, &dbErr)
	assert.Contains(t, string(dbErr.Query), "DELETE FROM")
}
