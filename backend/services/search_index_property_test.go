// TEST-07 (issue #435) / SEARCH-02 (#462) property: rebuilding the FTS index
// from the base tables produces an index equivalent to the incrementally
// maintained one. The incremental index is kept by SQL triggers on
// INSERT/UPDATE/DELETE (migrations 000007/000010/000020) — the mutation path
// a real client exercises; RebuildSearchIndex is the derived-from-source
// fallback. If the triggers and the rebuild ever disagree (a column the
// trigger indexes but the rebuild does not, a trigger forgetting the
// soft-delete guard), this property finds it with a shrunk counterexample.
//
// Per check it builds a fresh migrated database (the shared contactgen
// helper), populates generated contacts/notes/activities through the same
// ApplyRecordToContact path the API uses, mutates a random subset via the
// trigger paths (raw UPDATE + soft DELETE), snapshots the three FTS virtual
// tables, rebuilds, and asserts the snapshots are identical.
package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/contactgen"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// ftsTables are the three FTS5 virtual tables RebuildSearchIndex maintains.
var ftsTables = []string{"contacts_fts", "notes_fts", "activities_fts"}

// TestSearchIndex_RebuildMatchesIncremental is the load-bearing property.
func TestSearchIndex_RebuildMatchesIncremental(t *testing.T) {
	t.Run("rebuild", rapid.MakeCheck(func(t *rapid.T) {
		db, _, err := contactgen.MigratedDB(t)
		require.NoError(t, err)

		user, err := contactgen.NewUser(db, "search-prop")
		require.NoError(t, err)
		other, err := contactgen.NewUser(db, "search-prop-other")
		require.NoError(t, err)

		// Two users' contacts: the per-row user_id scoping must survive a
		// rebuild unchanged.
		recs := contactgen.Records(t, drawInt(t, "n", 0, 6))
		contacts, err := contactgen.Populate(db, user.ID, recs)
		require.NoError(t, err)
		recsOther := contactgen.Records(t, drawInt(t, "n2", 0, 3))
		_, err = contactgen.Populate(db, other.ID, recsOther)
		require.NoError(t, err)

		// Notes and activities (the other two indexed tables). A note needs a
		// contact when one exists; an unfiled note exercises the nil-contact
		// path.
		for i, c := range contacts {
			note := models.Note{UserID: user.ID, Content: fmt.Sprintf("generated note %d", i), Date: time.Now()}
			if len(contacts) > 0 {
				note.ContactID = &c.ID
			}
			require.NoError(t, db.Create(&note).Error)
		}
		if len(contacts) > 0 {
			require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "unfiled generated note", Date: time.Now()}).Error)
		}
		for i := range contacts {
			require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: fmt.Sprintf("generated activity %d", i), Description: "desc", Date: time.Now()}).Error)
		}

		// Mutate a random subset through the trigger paths: a raw UPDATE (the
		// AFTER UPDATE trigger re-indexes) and a soft DELETE (the re-insert
		// carries a deleted_at IS NULL guard, dropping the row from FTS).
		if len(contacts) > 0 && drawBool(t, "mutate.update") {
			require.NoError(t, db.Exec("UPDATE contacts SET firstname = ? WHERE id = ?", "Updated-"+string(rune('A'+drawInt(t, "mut.update.suffix", 0, 25))), contacts[0].ID).Error)
		}
		if len(contacts) > 1 && drawBool(t, "mutate.delete") {
			require.NoError(t, db.Delete(&contacts[0]).Error)
		}

		before, err := snapshotFTS(db)
		require.NoError(t, err)
		require.NoError(t, RebuildSearchIndex(db))
		after, err := snapshotFTS(db)
		require.NoError(t, err)

		for table, b := range before {
			assert.Equalf(t, b, after[table], "rebuild changed the contents of %s (incremental index and RebuildSearchIndex disagree)", table)
		}
	}))
}

// snapshotFTS dumps the three FTS virtual tables as deterministic strings
// (rowid-ordered), keyed by table name.
func snapshotFTS(db *gorm.DB) (map[string]string, error) {
	out := make(map[string]string, len(ftsTables))
	for _, table := range ftsTables {
		var parts []string
		rows, err := db.Raw("SELECT rowid, * FROM " + table + " ORDER BY rowid").Rows()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("columns of %s: %w", table, err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scanning %s: %w", table, err)
			}
			parts = append(parts, fmt.Sprint(vals...))
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("iterating %s: %w", table, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("closing %s: %w", table, err)
		}
		out[table] = strings.Join(parts, "\n")
	}
	return out, nil
}

func drawInt(t *rapid.T, label string, min, max int) int {
	return rapid.IntRange(min, max).Draw(t, label)
}

func drawBool(t *rapid.T, label string) bool {
	return rapid.Bool().Draw(t, label)
}
