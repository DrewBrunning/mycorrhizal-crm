package schemafixture

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"gorm.io/gorm"
)

// Expectation files (MIG-03, issue #438) — the "migrations that intentionally
// change data carry an explicit expectation file" half of the semantic
// migration suite.
//
// A file named `NNNNNN_name.expect.yaml` sits next to `NNNNNN_name.up.sql` and
// declares the data transformation that migration performs, at the granularity
// the content snapshot reads: one entry per changed cell, naming the table,
// the row (rowid), the column, and the concrete before->after values. The
// default — no file, or no entry — is "nothing changes": any cell whose value
// differs across the migration, any dropped column, any added/removed row, and
// any backfill into a NEW table fails the suite unless the file declares it.
//
// The suite compares the declared changeset against the actual one in both
// directions, so the file cannot rot:
//
//   - a declared `from` that never matched the before-state fails (the
//     expectation is stale — the fixture or the migration changed);
//   - a declared `to` the migration no longer produces fails (behavior drift);
//   - an actual change the file does not declare fails (the file understates
//     what the migration does).
//
// This format is the authoritative spec. A migration that must transform data
// mirrors a real case into a temporary file, runs the suite, and confirms the
// declared change passes while the two anti-rot directions above fail.

// expectChange is one declared cell-level data transformation.
type expectChange struct {
	Table  string `yaml:"table"`
	Row    string `yaml:"row"`
	Column string `yaml:"column"`
	From   any    `yaml:"from"`
	To     any    `yaml:"to"`
}

// expectFile is the parsed form of NNNNNN_name.expect.yaml.
type expectFile struct {
	Migration string         `yaml:"migration"`
	Changes   []expectChange `yaml:"changes"`
}

// committedMigrationsDir locates backend/database/migrations (the directory
// that the database package embeds) by walking up to the repo root.
func committedMigrationsDir(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	require.NoError(t, err)
	return filepath.Join(root, "backend", "database", "migrations")
}

// expectFileForMigration returns the expectation file for migration NNNNNN in
// dir (NNNNNN_*.expect.yaml), or nil when that migration declares no data
// transformation. The `migration:` field must name the file it lives in, so
// the self-description cannot drift from the file; a malformed or mismatched
// file is an error, never a silently-dropped declaration.
func expectFileForMigration(t *testing.T, dir string, version uint) (*expectFile, error) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	prefix := fmt.Sprintf("%06d_", version)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".expect.yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- path is not request input: name comes from ReadDir of a committed or test-owned directory
		if err != nil {
			return nil, fmt.Errorf("reading expectation file %s: %w", name, err)
		}
		var ef expectFile
		if err := yaml.Unmarshal(data, &ef); err != nil {
			return nil, fmt.Errorf("parsing expectation file %s: %w", name, err)
		}
		if want := strings.TrimSuffix(name, ".expect.yaml"); ef.Migration != want {
			return nil, fmt.Errorf("%s: the `migration:` field must name its own file (got %q)", name, ef.Migration)
		}
		for _, ch := range ef.Changes {
			if ch.Table == "" || ch.Row == "" || ch.Column == "" {
				return nil, fmt.Errorf("%s: every change must name a table, a row and a column", name)
			}
		}
		return &ef, nil
	}
	return nil, nil
}

// loadExpectationFiles collects every expectation file for the migrations a
// fixture of fromVersion traverses when migrated to toVersion (from+1 .. to).
func loadExpectationFiles(t *testing.T, dir string, fromVersion, toVersion uint) []*expectFile {
	t.Helper()
	var out []*expectFile
	for v := fromVersion + 1; v <= toVersion; v++ {
		ef, err := expectFileForMigration(t, dir, v)
		require.NoError(t, err)
		if ef != nil {
			out = append(out, ef)
		}
	}
	return out
}

// contentSnapshot is a row-ordered, column-scoped view of every table in one
// schema — the unit the semantic suite compares. Only the columns present at
// the snapshot's schema are captured; a column added by a later migration is
// new data in a new column, outside the "nothing changes" guarantee.
type contentSnapshot struct {
	// columns maps table -> ordered column names at the snapshot's schema.
	columns map[string][]string
	// rows maps table -> rowid -> column -> canonical cell rendering.
	rows map[string]map[string]map[string]string
}

// renderCell renders a database value (database/sql scan) or a YAML scalar
// into the canonical string both sides are compared as. Deterministic for a
// given value, so the before/after reads of the same stored bytes always
// render identically.
func renderCell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case []byte:
		return fmt.Sprintf("0x%x", x)
	default:
		return fmt.Sprint(x)
	}
}

// captureContentSnapshot reads every non-derived table of db into a
// contentSnapshot. FTS virtual tables and their shadow tables are excluded
// (their content is rebuilt by triggers from the base rows, which ARE
// captured), as are schema_migrations and sqlite_% internals. Rows are keyed
// by rowid so ordering is irrelevant and the same row is compared across the
// migration; rowids are stable because no migration rebuilds a fixture table
// except audit_events, which preserves ids (rowid == id under AUTOINCREMENT).
func captureContentSnapshot(t *testing.T, db *gorm.DB) *contentSnapshot {
	t.Helper()
	var objects []struct {
		Type string
		Name string
		SQL  string
	}
	require.NoError(t, db.Raw(`
		SELECT type, name, COALESCE(sql, '') AS sql FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations'
		ORDER BY name`).Scan(&objects).Error)

	virtual := map[string]bool{}
	for _, o := range objects {
		if strings.HasPrefix(o.SQL, "CREATE VIRTUAL TABLE") {
			virtual[o.Name] = true
		}
	}

	snap := &contentSnapshot{columns: map[string][]string{}, rows: map[string]map[string]map[string]string{}}
	for _, o := range objects {
		if virtual[o.Name] || isShadowTable(o.Name, virtual) {
			continue
		}
		cols := tableColumnsOf(t, db, o.Name)
		if len(cols) == 0 {
			continue
		}
		snap.columns[o.Name] = cols

		quoted := make([]string, 0, len(cols)+1)
		quoted = append(quoted, "rowid")
		for _, c := range cols {
			quoted = append(quoted, quoteIdentForSnapshot(c))
		}
		rows, err := db.Raw(fmt.Sprintf("SELECT %s FROM %s ORDER BY rowid", strings.Join(quoted, ", "), quoteIdentForSnapshot(o.Name))).Rows()
		require.NoError(t, err)
		rowNames, err := rows.Columns()
		require.NoError(t, err)

		byRow := map[string]map[string]string{}
		for rows.Next() {
			vals := make([]any, len(rowNames))
			ptrs := make([]any, len(rowNames))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			rid := renderCell(vals[0])
			cell := make(map[string]string, len(cols))
			for i := 1; i < len(rowNames); i++ {
				cell[cols[i-1]] = renderCell(vals[i])
			}
			byRow[rid] = cell
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		snap.rows[o.Name] = byRow
	}
	return snap
}

// tableColumnsOf returns a table's column names via PRAGMA table_info.
func tableColumnsOf(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	var rows []struct {
		Name string
	}
	require.NoError(t, db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentForSnapshot(table))).Scan(&rows).Error)
	cols := make([]string, 0, len(rows))
	for _, r := range rows {
		cols = append(cols, r.Name)
	}
	return cols
}

// quoteIdentForSnapshot wraps an identifier in double quotes with embedded
// quotes doubled — safe for the internal table/column names the snapshot
// reads (sqlite_master, not request input).
func quoteIdentForSnapshot(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// isShadowTable reports whether a name is the shadow of an FTS virtual table
// (X_content/_data/_idx/_docsize/_config hold only index bookkeeping).
func isShadowTable(name string, virtual map[string]bool) bool {
	for v := range virtual {
		if strings.HasPrefix(name, v+"_") {
			return true
		}
	}
	return false
}

// cloneSnapshot deep-copies a contentSnapshot so an expected-after state can
// be derived without mutating the before state.
func cloneSnapshot(s *contentSnapshot) *contentSnapshot {
	out := &contentSnapshot{
		columns: make(map[string][]string, len(s.columns)),
		rows:    make(map[string]map[string]map[string]string, len(s.rows)),
	}
	for table, cols := range s.columns {
		out.columns[table] = append([]string(nil), cols...)
	}
	for table, rows := range s.rows {
		out.rows[table] = make(map[string]map[string]string, len(rows))
		for rid, cell := range rows {
			cp := make(map[string]string, len(cell))
			for k, v := range cell {
				cp[k] = v
			}
			out.rows[table][rid] = cp
		}
	}
	return out
}

// expectedAfter builds the expected post-migration content snapshot from the
// before state and the declared changes, verifying each declared change
// actually matched the before state. The verification is the "cannot rot"
// check: a `from` value that never existed means the expectation is stale
// (the migration stopped performing it, or the fixture data moved), and must
// fail rather than pass vacantly.
func expectedAfter(fromTag string, before *contentSnapshot, declared []*expectFile) (*contentSnapshot, error) {
	expected := cloneSnapshot(before)

	for _, ef := range declared {
		for _, ch := range ef.Changes {
			cols, ok := expected.columns[ch.Table]
			if !ok {
				return nil, fmt.Errorf("%s: expect file %s declares table %q which is not part of the %s fixture schema", fromTag, ef.Migration, ch.Table, fromTag)
			}
			found := false
			for _, c := range cols {
				if c == ch.Column {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%s: expect file %s declares column %s.%s which does not exist in the %s fixture schema", fromTag, ef.Migration, ch.Table, ch.Column, fromTag)
			}

			beforeVal, ok := expected.rows[ch.Table][ch.Row][ch.Column]
			if !ok {
				return nil, fmt.Errorf("%s: expect file %s declares a change to %s row %s column %s which does not exist before the migration — the expectation has nothing to act on", fromTag, ef.Migration, ch.Table, ch.Row, ch.Column)
			}
			if renderCell(ch.From) != beforeVal {
				return nil, fmt.Errorf("%s: expect file %s declares %s row %s %s changed from %q but the fixture held %q — the file is stale", fromTag, ef.Migration, ch.Table, ch.Row, ch.Column, renderCell(ch.From), beforeVal)
			}

			expected.rows[ch.Table][ch.Row][ch.Column] = renderCell(ch.To)
		}
	}
	return expected, nil
}

// diffSnapshots returns a sorted, human-readable list of every difference
// between the expected state and the migrated state. It reports missing
// columns (data loss), removed rows (a hard delete of a soft-deleted row),
// added rows, changed cells, and a NEW table that holds rows (a backfill into
// a table the release never had, which must be declared). New empty tables and
// new columns are not differences: the release schema had nothing to preserve.
func diffSnapshots(expected, after *contentSnapshot) []string {
	var diffs []string

	for table, cols := range expected.columns {
		afterRows, hasTable := after.rows[table]
		if !hasTable {
			diffs = append(diffs, fmt.Sprintf("table %s: the whole table is gone", table))
			continue
		}
		afterCols := make(map[string]bool, len(after.columns[table]))
		for _, c := range after.columns[table] {
			afterCols[c] = true
		}
		for _, c := range cols {
			if !afterCols[c] {
				diffs = append(diffs, fmt.Sprintf("table %s: column %s was dropped (its data has nowhere to live)", table, c))
			}
		}

		beforeRows := expected.rows[table]
		for rid, cell := range beforeRows {
			afterCell, ok := afterRows[rid]
			if !ok {
				diffs = append(diffs, fmt.Sprintf("table %s: row %s was removed (hard-deleted what should survive)", table, rid))
				continue
			}
			for col, want := range cell {
				if got := afterCell[col]; got != want {
					diffs = append(diffs, fmt.Sprintf("table %s row %s column %s: %q -> %q", table, rid, col, want, got))
				}
			}
		}
		for rid := range afterRows {
			if _, ok := beforeRows[rid]; !ok {
				diffs = append(diffs, fmt.Sprintf("table %s: row %s was added (a new row in an existing table must be declared)", table, rid))
			}
		}
	}

	for table, rows := range after.rows {
		if _, ok := expected.columns[table]; !ok && len(rows) > 0 {
			diffs = append(diffs, fmt.Sprintf("table %s: a table the release did not have now holds %d rows — this backfill must be declared in an expectation file", table, len(rows)))
		}
	}

	sort.Strings(diffs)
	return diffs
}

// assertContentIdentical asserts the strict default: nothing changed.
func assertContentIdentical(t *testing.T, fromTag string, before, after *contentSnapshot) {
	t.Helper()
	diffs := diffSnapshots(before, after)
	for _, d := range diffs {
		t.Errorf("%s: %s", fromTag, d)
	}
}

// assertContentMatchesExpectations asserts the migrated state equals the
// before state with every declared change applied, and that every declared
// change really happened — both directions, so the file cannot rot.
func assertContentMatchesExpectations(t *testing.T, fromTag string, before, after *contentSnapshot, declared []*expectFile) {
	t.Helper()
	expected, err := expectedAfter(fromTag, before, declared)
	require.NoError(t, err)
	diffs := diffSnapshots(expected, after)
	for _, d := range diffs {
		t.Errorf("%s: %s", fromTag, d)
	}
}

// buildTestSnapshot constructs a contentSnapshot from a compact map form so
// the machinery's unit tests do not need a database.
func buildTestSnapshot(tables map[string]map[string]map[string]string) *contentSnapshot {
	snap := &contentSnapshot{columns: map[string][]string{}, rows: map[string]map[string]map[string]string{}}
	for table, rows := range tables {
		cols := map[string]bool{}
		byRow := map[string]map[string]string{}
		for rid, cell := range rows {
			byRow[rid] = cell
			for c := range cell {
				cols[c] = true
			}
		}
		sorted := make([]string, 0, len(cols))
		for c := range cols {
			sorted = append(sorted, c)
		}
		sort.Strings(sorted)
		snap.columns[table] = sorted
		snap.rows[table] = byRow
	}
	return snap
}

// writeExpectFile writes an expectation file into dir and returns its path.
func writeExpectFile(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

// TestExpectFileLoaderScansByVersionRange pins that the loader collects
// exactly the expectation files in the traversed migration range.
func TestExpectFileLoaderScansByVersionRange(t *testing.T) {
	dir := t.TempDir()
	writeExpectFile(t, dir, "000046_suffix_split.expect.yaml", `
migration: "000046_suffix_split"
changes:
  - table: contacts
    row: "1"
    column: firstname
    from: "Ada"
    to: "Augusta"
`)
	writeExpectFile(t, dir, "000050_later.expect.yaml", `
migration: "000050_later"
changes:
  - table: contacts
    row: "2"
    column: firstname
    from: "Bob"
    to: "Robert"
`)

	// Migrating a fixture from version 44 to 45 traverses 45 only.
	got := loadExpectationFiles(t, dir, 44, 45)
	require.Len(t, got, 0, "no expectation file in the 44..45 range")

	// 44..46 traverses 45 and 46.
	got = loadExpectationFiles(t, dir, 44, 46)
	require.Len(t, got, 1)
	require.Equal(t, "000046_suffix_split", got[0].Migration)

	got = loadExpectationFiles(t, dir, 45, 50)
	require.Len(t, got, 2)
}

// TestExpectFileLoaderRejectsMismatchedSelfDescription pins the anti-rot
// guard at parse time: a file whose `migration:` field does not name the file
// it lives in is rejected, so a copied-and-renamed expectation cannot silently
// claim to describe the wrong migration.
func TestExpectFileLoaderRejectsMismatchedSelfDescription(t *testing.T) {
	dir := t.TempDir()
	writeExpectFile(t, dir, "000047_typo.expect.yaml", `
migration: "000046_suffix_split"
changes: []
`)
	_, err := expectFileForMigration(t, dir, 47)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must name its own file")
}

// TestExpectFileLoaderRejectsIncompleteChange pins that a change missing its
// table/row/column is rejected rather than compared vacuously.
func TestExpectFileLoaderRejectsIncompleteChange(t *testing.T) {
	dir := t.TempDir()
	writeExpectFile(t, dir, "000048_x.expect.yaml", `
migration: "000048_x"
changes:
  - table: contacts
`)
	_, err := expectFileForMigration(t, dir, 48)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must name a table, a row and a column")
}

// TestExpectationMatchingDeclaredChange proves the happy path: a migration
// whose behavior exactly matches its expectation file passes, and only the
// declared cell differs.
func TestExpectationMatchingDeclaredChange(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {
			"1": {"firstname": "Ada", "lastname": "Lovelace"},
			"2": {"firstname": "Bob", "lastname": "Babbage"},
		},
	})
	after := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {
			"1": {"firstname": "Augusta", "lastname": "Lovelace"},
			"2": {"firstname": "Bob", "lastname": "Babbage"},
		},
	})
	declared := []*expectFile{{
		Migration: "000046_suffix_split",
		Changes: []expectChange{
			{Table: "contacts", Row: "1", Column: "firstname", From: "Ada", To: "Augusta"},
		},
	}}
	assertContentMatchesExpectations(t, "fixture", before, after, declared)
}

// TestExpectationStaleFromFails is the "cannot rot" direction: the declared
// `from` value never matched the before state, so the expectation is stale —
// the migration stopped performing it or the fixture moved — and must fail
// rather than pass vacuously.
func TestExpectationStaleFromFails(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Grace"}},
	})
	declared := []*expectFile{{
		Migration: "000046_suffix_split",
		Changes: []expectChange{
			{Table: "contacts", Row: "1", Column: "firstname", From: "Ada", To: "Augusta"},
		},
	}}
	_, err := expectedAfter("fixture", before, declared)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed from \"Ada\" but the fixture held \"Grace\"")
	assert.Contains(t, err.Error(), "the file is stale")
}

// TestExpectationDeclaredChangeNotPerformedFails is the "cannot rot" other
// direction: the migration did not do what the file says. `from` matched, so
// the file is not stale — but the declared `to` never landed, so the diff
// reports the cell.
func TestExpectationDeclaredChangeNotPerformedFails(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Ada"}},
	})
	after := cloneSnapshot(before) // migration did nothing
	declared := []*expectFile{{
		Migration: "000046_suffix_split",
		Changes: []expectChange{
			{Table: "contacts", Row: "1", Column: "firstname", From: "Ada", To: "Augusta"},
		},
	}}
	expected, err := expectedAfter("fixture", before, declared)
	require.NoError(t, err)
	diffs := diffSnapshots(expected, after)
	require.Len(t, diffs, 1)
	assert.Contains(t, diffs[0], "firstname")
	assert.Contains(t, diffs[0], "\"Augusta\" -> \"Ada\"")
}

// TestExpectationUndeclaredChangeFails is the "cannot rot" direction that
// guards the file against understating the migration: a change beyond the
// declaration fails.
func TestExpectationUndeclaredChangeFails(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Ada", "lastname": "Lovelace"}},
	})
	after := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Augusta", "lastname": "Byron"}},
	})
	declared := []*expectFile{{
		Migration: "000046_suffix_split",
		Changes: []expectChange{
			{Table: "contacts", Row: "1", Column: "firstname", From: "Ada", To: "Augusta"},
		},
	}}
	expected, err := expectedAfter("fixture", before, declared)
	require.NoError(t, err)
	diffs := diffSnapshots(expected, after)
	require.Len(t, diffs, 1, "only the undeclared lastname change should be reported")
	assert.Contains(t, diffs[0], "lastname")
	assert.Contains(t, diffs[0], "\"Lovelace\" -> \"Byron\"")
}

// TestExpectationDifferentToValueFails pins that a declared change whose
// to-value the migration does not produce is rejected — behavior drift in the
// other direction.
func TestExpectationDifferentToValueFails(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Ada"}},
	})
	after := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {"1": {"firstname": "Countess"}},
	})
	declared := []*expectFile{{
		Migration: "000046_suffix_split",
		Changes: []expectChange{
			{Table: "contacts", Row: "1", Column: "firstname", From: "Ada", To: "Augusta"},
		},
	}}
	expected, err := expectedAfter("fixture", before, declared)
	require.NoError(t, err)
	diffs := diffSnapshots(expected, after)
	require.Len(t, diffs, 1)
	assert.Contains(t, diffs[0], "firstname")
	assert.Contains(t, diffs[0], "\"Augusta\" -> \"Countess\"")
}

// TestDiffSnapshotsReportsDistinctFailureModes pins each shape the suite must
// name: a dropped column (a rename without a backfill), a removed row (a hard
// delete of a soft-deleted row), and a backfilled NEW table.
func TestDiffSnapshotsReportsDistinctFailureModes(t *testing.T) {
	before := buildTestSnapshot(map[string]map[string]map[string]string{
		"contacts": {
			"1": {"firstname": "Ada", "lastname": "Lovelace"},
			"2": {"firstname": "Bob", "lastname": "Babbage"},
		},
		"audit_events": {"5": {"entity_id": "urn:uuid:1"}},
	})
	after := buildTestSnapshot(map[string]map[string]map[string]string{
		// firstname renamed to name_first WITHOUT a backfill: column dropped.
		"contacts": {
			"1": {"lastname": "Lovelace", "name_first": "Ada"},
			// row 2 hard-deleted entirely.
		},
		"audit_events": {"5": {"entity_id": "urn:uuid:1"}},
		// a brand-new table the migration backfilled.
		"import_links": {"9": {"contact_id": "1"}},
	})

	diffs := diffSnapshots(before, after)
	joined := strings.Join(diffs, "\n")
	assert.Contains(t, joined, "column firstname was dropped", "a column rename without a backfill must name the emptied column")
	assert.Contains(t, joined, "row 2 was removed", "a hard-deleted soft-deleted row must be named")
	assert.Contains(t, joined, "import_links", "a backfill into a new table must be named")
	assert.Contains(t, joined, "this backfill must be declared", "the new-table message must tell the author what to do")
}

// TestRenderCellPinsTheCanonicalForms keeps the value renderings stable so a
// change in how a cell renders cannot silently pass or fail a comparison.
func TestRenderCellPinsTheCanonicalForms(t *testing.T) {
	assert.Equal(t, "NULL", renderCell(nil))
	assert.Equal(t, "42", renderCell(int64(42)))
	assert.Equal(t, "42", renderCell(42))
	assert.Equal(t, "Ada", renderCell("Ada"))
	assert.Equal(t, "0xabcd", renderCell([]byte{0xab, 0xcd}))
	assert.Equal(t, "2026-08-30T12:00:00Z", renderCell(time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)))
}
