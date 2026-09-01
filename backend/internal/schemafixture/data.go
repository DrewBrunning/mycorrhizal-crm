package schemafixture

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// tableData is the extracted data of one table: its column names (current
// schema) and one row of values per record, in SQLite's own serialized form.
type tableData struct {
	Table   string
	Columns []string
	Rows    [][]any
}

// extractData reads every populated table of the current-schema source
// database as tableData, skipping objects that must not be copied into a
// historical fixture:
//
//   - schema_migrations (the historical dump carries its own row; a second row
//     would break golang-migrate's LIMIT 1 read);
//   - sqlite_% internals (sqlite_sequence is re-derived by AUTOINCREMENT on
//     insert);
//   - FTS virtual tables and their shadow tables (the historical schema's FTS
//     triggers rebuild the index from the copied base rows).
//
// Value serialization round-trips through database/sql, so JSON text, dates,
// and blobs are copied losslessly.
func extractData(db *gorm.DB) (map[string]tableData, error) {
	type masterRow struct {
		Type string
		Name string
		SQL  string
	}
	var master []masterRow
	if err := db.Raw(`SELECT type, name, COALESCE(sql, '') AS sql FROM sqlite_master`).Scan(&master).Error; err != nil {
		return nil, fmt.Errorf("schemafixture: reading sqlite_master: %w", err) // # pragma: no cover — a freshly-migrated scratch DB always answers sqlite_master
	}

	virtual := map[string]bool{}
	tableNames := map[string]bool{}
	for _, m := range master {
		if m.Type != "table" {
			continue
		}
		if strings.HasPrefix(m.SQL, "CREATE VIRTUAL TABLE") {
			virtual[m.Name] = true
		} else if m.Name != "schema_migrations" && !strings.HasPrefix(m.Name, "sqlite_") {
			tableNames[m.Name] = true
		}
	}
	for v := range virtual {
		// Shadow tables (e.g. contacts_fts_data) are auto-created by the
		// virtual table and must not be copied.
		for name := range tableNames {
			if strings.HasPrefix(name, v+"_") {
				delete(tableNames, name)
			}
		}
	}

	out := make(map[string]tableData, len(tableNames))
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err // # pragma: no cover — DB() fails only on a closed handle, which the loader never passes
	}
	for name := range tableNames {
		colNames, err := tableColumns(sqlDB, name)
		if err != nil {
			return nil, err // # pragma: no cover — a real sqlite_master table always answers PRAGMA table_info
		}
		rows, err := readAllRows(sqlDB, name, colNames)
		if err != nil {
			return nil, err // # pragma: no cover — a real table with PRAGMA-confirmed columns always selects
		}
		out[name] = tableData{Table: name, Columns: colNames, Rows: rows}
	}
	return out, nil
}

// tableColumns returns a table's column names via PRAGMA table_info, keeping
// the name from sqlite_master (internal, not request input).
func tableColumns(conn *sql.DB, table string) ([]string, error) {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("schemafixture: reading columns of %s: %w", table, err) // # pragma: no cover — a real table always answers PRAGMA
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("schemafixture: scanning column of %s: %w", table, err) // # pragma: no cover — PRAGMA rows always scan into six fixed columns
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// readAllRows reads every row of a table into database/sql value slots,
// preserving column order.
func readAllRows(conn *sql.DB, table string, cols []string) ([][]any, error) {
	colList := quoteIdents(cols)
	rows, err := conn.Query(fmt.Sprintf("SELECT %s FROM %s", colList, quoteIdent(table)))
	if err != nil {
		return nil, fmt.Errorf("schemafixture: selecting from %s: %w", table, err) // # pragma: no cover — a real table always selects
	}
	defer rows.Close()

	var out [][]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("schemafixture: scanning %s: %w", table, err) // # pragma: no cover — database/sql returns typed values that always scan into any
		}
		out = append(out, values)
	}
	return out, rows.Err()
}

// copyData copies every extracted table into the historical-schema connection,
// intersecting each table's columns against the columns that actually exist at
// that version (queried from the live schema). Tables or columns absent from
// the historical schema — added by migrations after the release — are
// dropped; they were empty in the manifest dataset anyway, and any data they
// hold has no home in a fixture of that release.
func copyData(conn *sql.DB, data map[string]tableData) error {
	present, err := presentColumns(conn)
	if err != nil {
		return err // # pragma: no cover — a schema dump just applied always answers sqlite_master
	}

	// Deterministic order: copying in a stable order keeps a failure message
	// reproducible and (with FK enforcement off) sidesteps dependency order.
	for _, name := range sortedKeys(data) {
		td := data[name]
		presentCols, ok := present[name]
		if !ok {
			continue
		}
		selected := make([]string, 0, len(td.Columns))
		idx := make([]int, 0, len(td.Columns))
		for i, c := range td.Columns {
			if presentCols[c] {
				selected = append(selected, c)
				idx = append(idx, i)
			}
		}
		if len(selected) == 0 || len(td.Rows) == 0 {
			continue
		}

		var b strings.Builder
		fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES ",
			quoteIdent(name), quoteIdents(selected))
		for r, row := range td.Rows {
			if r > 0 {
				b.WriteString(",")
			}
			values := make([]string, 0, len(idx))
			for _, i := range idx {
				values = append(values, literal(row[i]))
			}
			fmt.Fprintf(&b, "(%s)", strings.Join(values, ","))
		}
		if _, err := conn.Exec(b.String()); err != nil {
			return fmt.Errorf("schemafixture: copying %s into historical schema: %w", name, err) // # pragma: no cover — an INSERT against columns just read from this schema cannot fail
		}
	}
	return nil
}

// presentColumns returns the set of tables and columns that exist in the
// historical schema, from PRAGMA table_info.
func presentColumns(conn *sql.DB) (map[string]map[string]bool, error) {
	var tables []string
	rows, err := conn.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err // # pragma: no cover — a schema dump just applied always answers sqlite_master
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err // # pragma: no cover — a single TEXT column always scans
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		return nil, err // # pragma: no cover — closing a completed rows set does not fail
	}
	if err := rows.Err(); err != nil {
		return nil, err // # pragma: no cover — iteration errors surface during Next, not after Close
	}

	out := make(map[string]map[string]bool, len(tables))
	for _, name := range tables {
		cols, err := tableColumns(conn, name)
		if err != nil {
			return nil, err // # pragma: no cover — a real table always answers PRAGMA
		}
		set := make(map[string]bool, len(cols))
		for _, c := range cols {
			set[c] = true
		}
		out[name] = set
	}
	return out, nil
}

func sortedKeys(m map[string]tableData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quoteIdent wraps an identifier in double quotes with embedded quotes
// doubled — safe for the internal table/column names this loader handles.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func quoteIdents(ss []string) string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = quoteIdent(s)
	}
	return strings.Join(out, ", ")
}

// literal renders a scanned value as a SQL literal for the INSERT. Values
// come from database/sql: nil -> NULL, int64/float64 -> bare number, []byte
// -> blob literal, string -> quoted text, time.Time -> RFC3339Nano text.
//
// The time.Time case is load-bearing, not cosmetic: the SQLite driver returns
// a time.Time for a DATETIME column, and Go's default %v rendering
// ("2026-09-01 10:22:45.81 -0500 -0500") is a format GORM's driver cannot scan
// back into a time.Time field ("unsupported Scan ... string into *time.Time").
// Every migration-written row in production stores RFC3339Nano UTC, so a
// fixture that matches it can be driven through the real GORM models and HTTP
// stack after an upgrade (DEPLOY-02, issue #451) rather than only through raw
// SQL.
func literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case []byte:
		return "X'" + fmt.Sprintf("%x", x) + "'"
	case time.Time:
		return "'" + x.UTC().Format(time.RFC3339Nano) + "'"
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", x), "'", "''") + "'"
	}
}
