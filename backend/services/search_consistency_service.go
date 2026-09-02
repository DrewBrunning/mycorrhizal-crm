package services

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// FTS index consistency validation (SEARCH-02, issue #462; ADR 0012 INV-D9).
// This is the *detection* half of the derived-index story — it reports whether
// the trigger-maintained FTS5 index currently matches canonical data. The
// *repair* is RebuildSearchIndex (SEARCH-01, issue #461); this function never
// writes.
//
// # The index contract (what a divergence is, and is not)
//
// The FTS index is derived data maintained by SQL triggers (migrations
// 000007/000010/000020). Its contract is deliberately narrower than "mirrors
// the base table", so a naive base-vs-index diff would produce constant false
// positives:
//
//   - The index is NOT authoritative on deletion or archive state. A
//     soft-deleted or archived base row may or may not sit in the index
//     depending on which write path touched it — the AFTER UPDATE trigger
//     drops a row on soft-delete, but the AFTER INSERT trigger has no
//     deleted_at guard, so a row inserted already-soft-deleted (a bulk import,
//     a hand-written migration) stays. Both are acceptable: search correctness
//     for those rows is guaranteed by the outer query's own `archived` filter
//     and GORM's soft-delete scope, never by index contents
//     (controllers/contact_controller.go, the applyContactSearch subquery).
//     So this check only ever compares the index against **live**
//     (deleted_at IS NULL) base rows, and never reports a soft-deleted or
//     archived row that is present in the index.
//
// The real divergence classes, all of which a trigger-bypassing write can
// produce and none of which the contract tolerates:
//
//   - FTSDivergenceMissingFromIndex — a live base row with no index row. The
//     class that actually breaks search.
//   - FTSDivergenceOrphanInIndex — an index row whose base row does not exist
//     at all (hard-deleted). A soft-deleted base row is NOT an orphan.
//   - FTSDivergenceContentMismatch — a row present in both whose indexed
//     column values differ from the current base values. The class a
//     migration that updates a base table directly produces.
//   - FTSDivergenceScopeMismatch — a row present in both whose indexed
//     user_id differs from the base user_id. A scoping problem as well as a
//     correctness one.
const (
	FTSDivergenceMissingFromIndex = "missing_from_index"
	FTSDivergenceOrphanInIndex    = "orphan_in_index"
	FTSDivergenceContentMismatch  = "content_mismatch"
	FTSDivergenceScopeMismatch    = "scope_mismatch"
)

// FTSDivergence is one specific disagreement between an FTS index and
// canonical data: which index, which row, which direction, and (for a content
// mismatch) which columns. It never carries a field *value* — only column
// names — so it is safe to log and to surface on an admin diagnostics
// response.
type FTSDivergence struct {
	Index   string   `json:"index"`
	RowID   int64    `json:"rowid"`
	Class   string   `json:"class"`
	Columns []string `json:"columns,omitempty"`
}

// FTSConsistencyResult is the outcome of CheckSearchIndexConsistency.
type FTSConsistencyResult struct {
	// Divergences is the (possibly capped) list of specific disagreements,
	// ordered by index then class then rowid.
	Divergences []FTSDivergence `json:"divergences"`
	// Truncated is true when at least one (index, class) pair had more
	// divergences than the per-pair cap and Divergences holds only a sample.
	Truncated bool `json:"truncated"`
}

// Clean reports whether the index matches canonical data under the contract.
func (r FTSConsistencyResult) Clean() bool { return len(r.Divergences) == 0 }

// Summary is a short, low-cardinality description for a log line or an
// operational-check-result detail string — counts per (index, class), never a
// row id list of unbounded length.
func (r FTSConsistencyResult) Summary() string {
	if r.Clean() {
		return "search index consistent with canonical data"
	}
	counts := map[string]int{}
	order := []string{}
	for _, d := range r.Divergences {
		k := d.Index + "/" + d.Class
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	parts := make([]string, 0, len(order))
	for _, k := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	s := strings.Join(parts, " ")
	if r.Truncated {
		s += " (sampled)"
	}
	return s
}

// ftsDivergenceCapPerClass bounds how many rows of any one (index, class) pair
// CheckSearchIndexConsistency returns. A catastrophically desynchronised index
// (e.g. every row) should send the operator to a rebuild, not stream millions
// of ids through a diagnostics response.
const ftsDivergenceCapPerClass = 100

// ftsConsistencySpec describes one FTS virtual table and the base table it is
// derived from. contentCols are the columns present in BOTH that a rebuild /
// the triggers copy verbatim — compared NULL-safely with SQLite `IS NOT`.
type ftsConsistencySpec struct {
	index       string
	base        string
	contentCols []string
}

// ftsConsistencySpecs is the full set. The column lists mirror
// RebuildSearchIndex's INSERT ... SELECT and the post-000020 trigger bodies —
// if a future migration indexes a new column, add it here too.
var ftsConsistencySpecs = []ftsConsistencySpec{
	{"contacts_fts", "contacts", []string{
		"firstname", "lastname", "nickname", "email", "phone", "org",
		"addresses_flat", "phones_normalized",
	}},
	{"notes_fts", "notes", []string{"content"}},
	{"activities_fts", "activities", []string{"title", "description", "location"}},
}

// CheckSearchIndexConsistency validates all three FTS indexes against
// canonical data under the contract documented above, read-only. It is cheap —
// every query is an indexed join on the rowid/PK — so it is safe to run from
// the scheduled integrity job and from the on-demand diagnostics sweep.
func CheckSearchIndexConsistency(db *gorm.DB) (FTSConsistencyResult, error) {
	var res FTSConsistencyResult

	for _, spec := range ftsConsistencySpecs {
		// The three rowid-only classes share one shape: run a bounded query,
		// tag each returned rowid with the class.
		rowidChecks := []struct {
			class string
			query string
		}{
			{FTSDivergenceMissingFromIndex, fmt.Sprintf(
				`SELECT b.id FROM %[2]s b LEFT JOIN %[1]s f ON f.rowid = b.id
				 WHERE b.deleted_at IS NULL AND f.rowid IS NULL
				 ORDER BY b.id LIMIT ?`, spec.index, spec.base)},
			// A soft-deleted base row still exists, so it is deliberately not
			// an orphan (the contract).
			{FTSDivergenceOrphanInIndex, fmt.Sprintf(
				`SELECT f.rowid FROM %[1]s f LEFT JOIN %[2]s b ON b.id = f.rowid
				 WHERE b.id IS NULL
				 ORDER BY f.rowid LIMIT ?`, spec.index, spec.base)},
			{FTSDivergenceScopeMismatch, fmt.Sprintf(
				`SELECT f.rowid FROM %[1]s f JOIN %[2]s b ON b.id = f.rowid
				 WHERE b.deleted_at IS NULL AND f.user_id IS NOT b.user_id
				 ORDER BY f.rowid LIMIT ?`, spec.index, spec.base)},
		}
		for _, rc := range rowidChecks {
			var ids []int64
			if err := db.Raw(rc.query, ftsDivergenceCapPerClass+1).Scan(&ids).Error; err != nil {
				return res, fmt.Errorf("fts consistency (%s %s): %w", spec.index, rc.class, err)
			}
			if len(ids) > ftsDivergenceCapPerClass {
				ids, res.Truncated = ids[:ftsDivergenceCapPerClass], true
			}
			for _, id := range ids {
				res.Divergences = append(res.Divergences, FTSDivergence{Index: spec.index, RowID: id, Class: rc.class})
			}
		}

		// Content mismatch: rows present in both (with a live base row) whose
		// indexed columns differ. The query selects rowid plus one boolean
		// per column so the report can name which columns drifted, never a
		// value.
		content, capped, err := ftsContentMismatches(db, spec)
		if err != nil {
			return res, fmt.Errorf("fts consistency (%s %s): %w", spec.index, FTSDivergenceContentMismatch, err)
		}
		res.Truncated = res.Truncated || capped
		res.Divergences = append(res.Divergences, content...)
	}

	return res, nil
}

// ftsContentMismatches runs the per-column comparison for one index and
// returns a content_mismatch divergence per drifted row, with the drifted
// column names.
func ftsContentMismatches(db *gorm.DB, spec ftsConsistencySpec) ([]FTSDivergence, bool, error) {
	selects := make([]string, len(spec.contentCols))
	predicates := make([]string, len(spec.contentCols))
	for i, col := range spec.contentCols {
		selects[i] = fmt.Sprintf("(f.%[1]s IS NOT b.%[1]s) AS m_%[1]s", col)
		predicates[i] = fmt.Sprintf("f.%[1]s IS NOT b.%[1]s", col)
	}
	query := fmt.Sprintf(
		`SELECT f.rowid AS rowid, %s
		 FROM %s f JOIN %s b ON b.id = f.rowid
		 WHERE b.deleted_at IS NULL AND (%s)
		 ORDER BY f.rowid LIMIT ?`,
		strings.Join(selects, ", "), spec.index, spec.base, strings.Join(predicates, " OR "))

	var rows []map[string]any
	if err := db.Raw(query, ftsDivergenceCapPerClass+1).Scan(&rows).Error; err != nil {
		return nil, false, err
	}

	capped := false
	if len(rows) > ftsDivergenceCapPerClass {
		rows, capped = rows[:ftsDivergenceCapPerClass], true
	}

	out := make([]FTSDivergence, 0, len(rows))
	for _, row := range rows {
		var cols []string
		for _, col := range spec.contentCols {
			// A GORM map-scan surfaces every SQLite integer (the rowid, and
			// each `x IS NOT y` 0/1 flag) as int64; a missing key or other
			// type reads as the zero value, i.e. "no mismatch".
			if flag, _ := row["m_"+col].(int64); flag != 0 {
				cols = append(cols, col)
			}
		}
		rowid, _ := row["rowid"].(int64)
		out = append(out, FTSDivergence{
			Index: spec.index, RowID: rowid, Class: FTSDivergenceContentMismatch, Columns: cols,
		})
	}
	return out, capped, nil
}
