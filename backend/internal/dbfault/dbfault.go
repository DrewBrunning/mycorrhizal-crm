// Package dbfault is a GORM callback plugin that fails chosen database
// statements on purpose (issue #1476), so a test can reach the
// `if err := db.X(...).Error; err != nil { return ErrDatabase(...) }` arms that
// no HTTP-level test can trigger — and, more importantly, prove that a failure
// in the MIDDLE of a request leaves the database exactly as it was.
//
// # How it is used
//
// Register it only under test, through dbtest.New(t, dbtest.WithFaults()), then
// fetch the injector with For(db). The plugin has three modes:
//
//   - idle (the default): every statement passes through, nothing is recorded.
//   - Record: every statement issued by the *calling goroutine* is logged
//     (verb + table) and passed through. The log's length is K, the number of
//     statements one request issues.
//   - FailNth / FailMatching: the n-th statement (1-based), or the first one
//     matching a verb + table, of the calling goroutine returns ErrInjected
//     instead of running.
//
// # Why goroutine-scoped
//
// Handlers spawn fire-and-forget goroutines (audit writes, webhook deliveries)
// that share the same *gorm.DB. Counting or failing their statements would make
// "statement i" nondeterministic from one run to the next, so the injector only
// sees statements issued on the goroutine that armed it — in a handler test that
// is the goroutine driving httptest's ServeHTTP, i.e. the request goroutine.
// GORM's own nested sessions (Preload, transaction closures) run on that same
// goroutine and are counted.
//
// # What it cannot fail
//
// Begin / Commit / Rollback are not GORM statements and never go through the
// callback chain, so a failing COMMIT is out of scope here. Everything that is a
// Create / Update / Delete / Query / Row / Raw (Exec) statement is in scope.
package dbfault

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// PluginName is the key under which the injector is stored in
// gorm.Config.Plugins; For looks it up by this name.
const PluginName = "dbfault"

// Marker is embedded in the injected error text. A response body containing it
// proves an internal database error string leaked to the client.
const Marker = "DBFAULT_INJECTED"

// ErrInjected is what a failed statement returns. Its text deliberately reads
// like the raw SQLite error a leak would expose.
var ErrInjected = errors.New(Marker + ": sqlite: SQL logic error: disk I/O error")

// Statement describes one database statement the injector saw.
type Statement struct {
	Verb  string // create | update | delete | query | row | raw
	Table string // best-effort table name ("" when GORM did not resolve one)
}

func (s Statement) String() string { return s.Verb + " " + s.Table }

type mode int

const (
	modeIdle mode = iota
	modeRecord
	modeFailNth
	modeFailMatching
)

// Injector is the per-database plugin instance.
type Injector struct {
	mu        sync.Mutex
	mode      mode
	gid       uint64 // goroutine the current mode applies to
	seen      []Statement
	failAt    int
	failVerb  string
	failTable string
	fired     bool
	firedStmt Statement
}

// New returns an idle injector.
func New() *Injector { return &Injector{} }

// Name implements gorm.Plugin.
func (i *Injector) Name() string { return PluginName }

// Initialize implements gorm.Plugin: it installs a "before" callback on every
// statement kind, ahead of GORM's own transaction/exec callbacks so a failed
// statement is rolled back exactly like a real driver error would be.
func (i *Injector) Initialize(db *gorm.DB) error {
	cb := db.Callback()
	regs := map[string]func(name string, fn func(*gorm.DB)) error{
		"create": cb.Create().Before("gorm:begin_transaction").Register,
		"update": cb.Update().Before("gorm:begin_transaction").Register,
		"delete": cb.Delete().Before("gorm:begin_transaction").Register,
		"query":  cb.Query().Before("gorm:query").Register,
		"row":    cb.Row().Before("gorm:row").Register,
		"raw":    cb.Raw().Before("gorm:raw").Register,
	}
	for verb, reg := range regs {
		v := verb
		if err := reg("dbfault:"+v, func(tx *gorm.DB) { i.intercept(tx, v) }); err != nil {
			return fmt.Errorf("dbfault: registering %s callback: %w", v, err) // # pragma: no cover — gorm only errors on a duplicate callback name; the names are fixed and unique per plugin
		}
	}
	return nil
}

func (i *Injector) intercept(tx *gorm.DB, verb string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.mode == modeIdle || tx.Error != nil || goroutineID() != i.gid {
		return
	}
	st := Statement{Verb: verb, Table: tableOf(tx)}
	i.seen = append(i.seen, st)
	switch i.mode {
	case modeFailNth:
		if len(i.seen) == i.failAt {
			i.fire(tx, st)
		}
	case modeFailMatching:
		if !i.fired && st.Verb == i.failVerb && st.Table == i.failTable {
			i.fire(tx, st)
		}
	}
}

func (i *Injector) fire(tx *gorm.DB, st Statement) {
	i.fired = true
	i.firedStmt = st
	_ = tx.AddError(ErrInjected)
}

// tableOf resolves a statement's table: GORM's resolved Table when present,
// else the first identifier after FROM/INTO/UPDATE in a raw statement.
func tableOf(tx *gorm.DB) string {
	if t := tx.Statement.Table; t != "" {
		return t
	}
	return rawTable(tx.Statement.SQL.String())
}

func rawTable(sql string) string {
	f := strings.Fields(strings.ToLower(sql))
	for idx, w := range f {
		switch w {
		case "from", "into", "update", "table":
			if idx+1 < len(f) {
				return strings.Trim(f[idx+1], "`\"[]();,")
			}
		}
	}
	return ""
}

// Record starts logging the calling goroutine's statements (passing them
// through) and clears any previous log or firing.
func (i *Injector) Record() { i.arm(modeRecord, 0, "", "") }

// FailNth makes the calling goroutine's n-th statement (1-based) fail. n < 1
// never fires.
func (i *Injector) FailNth(n int) { i.arm(modeFailNth, n, "", "") }

// FailMatching makes the first statement with this verb and table fail.
func (i *Injector) FailMatching(verb, table string) { i.arm(modeFailMatching, 0, verb, table) }

func (i *Injector) arm(m mode, n int, verb, table string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.mode, i.gid, i.failAt, i.failVerb, i.failTable = m, goroutineID(), n, verb, table
	i.seen, i.fired, i.firedStmt = nil, false, Statement{}
}

// Disarm returns the injector to idle; the log of the last run stays readable.
func (i *Injector) Disarm() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.mode = modeIdle
}

// Statements returns a copy of the statements seen since the last arm call.
func (i *Injector) Statements() []Statement {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]Statement(nil), i.seen...)
}

// Fired reports whether an injection happened and which statement it hit.
func (i *Injector) Fired() (Statement, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.firedStmt, i.fired
}

// For returns the injector registered on db, or nil when db was not built with
// dbtest.WithFaults.
func For(db *gorm.DB) *Injector {
	if p, ok := db.Plugins[PluginName]; ok {
		if inj, ok := p.(*Injector); ok {
			return inj
		}
	}
	return nil
}

// goroutineID parses the current goroutine's id out of runtime.Stack ("goroutine
// 123 [running]:"). Test-only machinery; the cost is irrelevant.
func goroutineID() uint64 {
	var buf [64]byte
	s := strings.TrimPrefix(string(buf[:runtime.Stack(buf[:], false)]), "goroutine ")
	id, _ := strconv.ParseUint(s[:strings.IndexByte(s, ' ')], 10, 64)
	return id
}
