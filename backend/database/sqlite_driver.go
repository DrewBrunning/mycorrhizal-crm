package database

// sqliteDriver is a local golang-migrate database driver for SQLite.
// It is adapted from https://github.com/golang-migrate/migrate/blob/master/database/sqlite/sqlite.go (commit 89e308c)
// with the _ "modernc.org/sqlite" import removed (since "sqlite" driver is already registered by github.com/glebarez/go-sqlite (via glebarez/sqlite)).
// I've chosen this approach since a drift here is still less a risk than implementing a full custom driver

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	nurl "net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"mycorrhizal/internal/faults"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
)

var defaultMigrationsTable = "schema_migrations"

// faultMigrationStatement is the failure-injection seam for the migration
// driver's per-migration boundary (issue #434). Armed via the faults package,
// it fires inside golang-migrate's Run AFTER the migration body has committed
// but BEFORE golang-migrate marks the version clean — the crash signature a
// SIGKILL between a migration's commit and its clean-mark leaves behind:
// version dirty at N, migration N fully applied. The existing dirty-state
// recovery (force version + re-run from N+1) is provably correct for exactly
// this window, which is what the injection tests pin. The external-fault CI
// job uses `MYCORRHIZAL_FAULTS=database.migration.statement:pause:<dur>` to
// park a subprocess in this window and then SIGKILL it. See
// docs/development/fault-injection.md.
const faultMigrationStatement = "database.migration.statement"

var (
	errDatabaseDirty = fmt.Errorf("database is dirty")
	errNilConfig     = fmt.Errorf("no config")
)

type sqliteConfig struct {
	MigrationsTable string
	DatabaseName    string
	NoTxWrap        bool
}

type sqliteDriver struct {
	db       *sql.DB
	isLocked atomic.Bool
	config   *sqliteConfig
}

func withInstance(instance *sql.DB, config *sqliteConfig) (database.Driver, error) {
	if config == nil {
		return nil, errNilConfig
	}

	if err := instance.Ping(); err != nil {
		return nil, err
	}

	if len(config.MigrationsTable) == 0 {
		config.MigrationsTable = defaultMigrationsTable
	}

	mx := &sqliteDriver{
		db:     instance,
		config: config,
	}
	if err := mx.ensureVersionTable(); err != nil {
		return nil, err
	}
	return mx, nil
}

func (m *sqliteDriver) ensureVersionTable() (err error) {
	if err = m.Lock(); err != nil {
		return err
	}

	defer func() {
		if e := m.Unlock(); e != nil {
			err = errors.Join(err, e)
		}
	}()

	query := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (version uint64,dirty bool);
  CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON %s (version);
  `, m.config.MigrationsTable, m.config.MigrationsTable)

	if _, err := m.db.Exec(query); err != nil {
		return err
	}
	return nil
}

func (m *sqliteDriver) Open(url string) (database.Driver, error) {
	purl, err := nurl.Parse(url)
	if err != nil {
		return nil, err
	}
	dbfile := strings.Replace(migrate.FilterCustomQuery(purl).String(), "sqlite://", "", 1)
	db, err := sql.Open("sqlite", dbfile)
	if err != nil {
		return nil, err
	}

	qv := purl.Query()

	migrationsTable := qv.Get("x-migrations-table")
	if len(migrationsTable) == 0 {
		migrationsTable = defaultMigrationsTable
	}

	noTxWrap := false
	if v := qv.Get("x-no-tx-wrap"); v != "" {
		noTxWrap, err = strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("x-no-tx-wrap: %s", err)
		}
	}

	mx, err := withInstance(db, &sqliteConfig{
		DatabaseName:    purl.Path,
		MigrationsTable: migrationsTable,
		NoTxWrap:        noTxWrap,
	})
	if err != nil {
		return nil, err
	}
	return mx, nil
}

func (m *sqliteDriver) Close() error {
	return m.db.Close()
}

func (m *sqliteDriver) Drop() (err error) {
	query := `SELECT name FROM sqlite_master WHERE type = 'table';`
	tables, err := m.db.Query(query)
	if err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	defer func() {
		if errClose := tables.Close(); errClose != nil {
			err = errors.Join(err, errClose)
		}
	}()

	tableNames := make([]string, 0)
	for tables.Next() {
		var tableName string
		if err := tables.Scan(&tableName); err != nil {
			return err
		}
		if len(tableName) > 0 {
			tableNames = append(tableNames, tableName)
		}
	}
	if err := tables.Err(); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	if len(tableNames) > 0 {
		for _, t := range tableNames {
			query := "DROP TABLE " + t
			err = m.executeQuery(query)
			if err != nil {
				return &database.Error{OrigErr: err, Query: []byte(query)}
			}
		}
		query := "VACUUM"
		_, err = m.db.Query(query)
		if err != nil {
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	return nil
}

func (m *sqliteDriver) Lock() error {
	if !m.isLocked.CompareAndSwap(false, true) {
		return database.ErrLocked
	}
	return nil
}

func (m *sqliteDriver) Unlock() error {
	if !m.isLocked.CompareAndSwap(true, false) {
		return database.ErrNotLocked
	}
	return nil
}

func (m *sqliteDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return err
	}
	query := string(migr[:])

	if m.config.NoTxWrap {
		return m.executeQueryNoTx(query)
	}
	if err := m.executeQuery(query); err != nil {
		return err
	}

	// Issue #434 failure-injection seam. The migration body has just committed;
	// golang-migrate has NOT yet cleared the dirty flag (SetVersion(false)
	// happens after Run returns). An armed error fault therefore fails the run
	// with the version left dirty — the exact crash signature of a process
	// killed between a migration's commit and its clean-mark — and the
	// dirty-state recovery (force + re-run from N+1) is what the injection
	// tests assert. A pause fault parks here for the external-fault CI job to
	// SIGKILL. Unarmed, faults.Hook is a nil-returning map lookup.
	if err := faults.Hook(faultMigrationStatement); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	return nil
}

func (m *sqliteDriver) executeQuery(query string) error {
	tx, err := m.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}
	if _, err := tx.Exec(query); err != nil {
		if errRollback := tx.Rollback(); errRollback != nil {
			err = errors.Join(err, errRollback)
		}
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}
	return nil
}

func (m *sqliteDriver) executeQueryNoTx(query string) error {
	if _, err := m.db.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	return nil
}

func (m *sqliteDriver) SetVersion(version int, dirty bool) error {
	tx, err := m.db.Begin()
	if err != nil {
		return &database.Error{OrigErr: err, Err: "transaction start failed"}
	}

	query := "DELETE FROM " + m.config.MigrationsTable // #nosec G202 -- table name is an internal config constant, not user input
	if _, err := tx.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	if version >= 0 || (version == database.NilVersion && dirty) {
		query := fmt.Sprintf(`INSERT INTO %s (version, dirty) VALUES (?, ?)`, m.config.MigrationsTable) // #nosec G201 -- table name is an internal config constant, not user input
		if _, err := tx.Exec(query, version, dirty); err != nil {
			if errRollback := tx.Rollback(); errRollback != nil {
				err = errors.Join(err, errRollback)
			}
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	if err := tx.Commit(); err != nil {
		return &database.Error{OrigErr: err, Err: "transaction commit failed"}
	}

	return nil
}

func (m *sqliteDriver) Version() (version int, dirty bool, err error) {
	query := "SELECT version, dirty FROM " + m.config.MigrationsTable + " LIMIT 1"
	err = m.db.QueryRow(query).Scan(&version, &dirty)
	if err != nil {
		return database.NilVersion, false, nil
	}
	return version, dirty, nil
}

// Ensure sqliteDriver implements database.Driver at compile time.
var _ database.Driver = (*sqliteDriver)(nil)

// Suppress unused import warning for errDatabaseDirty in case it's not referenced.
var _ = errDatabaseDirty
