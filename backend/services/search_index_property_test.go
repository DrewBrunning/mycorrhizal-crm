// TEST-07 (issue #435) / SEARCH-02 (#462) property: for an arbitrary sequence
// of creates, updates, soft-deletes and hard-deletes, the trigger-maintained
// FTS index and a from-scratch RebuildSearchIndex agree on everything the
// index contract covers, and CheckSearchIndexConsistency reports the
// trigger-maintained index clean.
//
// The incremental index is kept by SQL triggers (migrations
// 000007/000010/000020) — the path a real client exercises; RebuildSearchIndex
// is the derived-from-source fallback. If the two ever disagree (a column the
// trigger indexes but the rebuild does not, a trigger forgetting the
// soft-delete guard), this property finds it with a shrunk counterexample.
//
// # Comparison is modulo the contract (issue #462 action 7)
//
// The AFTER INSERT triggers have no `deleted_at IS NULL` guard (only AFTER
// UPDATE does), so a row inserted already-soft-deleted — a bulk import, a
// hand-written migration — stays in the incremental index, while a rebuild
// (which selects live rows only) will not reproduce it. That asymmetry is
// acceptable under the contract (ADR 0012 INV-D9): the outer query, not index
// contents, is authoritative on deletion state. **Decision: normalise in the
// comparison, do not align the triggers** — restrict the snapshot equality to
// rowids backed by a live base row (snapshotFTSLive). The full incremental
// snapshot is still handed to CheckSearchIndexConsistency, which is contract-
// aware and must call it clean.
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

		// Mutate a random subset through every trigger path:
		//   - a raw UPDATE (AFTER UPDATE re-indexes)
		//   - a soft DELETE (AFTER UPDATE re-insert has the deleted_at guard,
		//     so the row drops out of FTS)
		//   - a hard DELETE (AFTER DELETE removes the FTS row)
		//   - a raw INSERT of an already-soft-deleted contact (AFTER INSERT has
		//     NO guard, so the row lands in the incremental index — the
		//     asymmetry the contract tolerates and the rebuild will not
		//     reproduce)
		if len(contacts) > 0 && drawBool(t, "mutate.update") {
			require.NoError(t, db.Exec("UPDATE contacts SET firstname = ? WHERE id = ?", "Updated-"+string(rune('A'+drawInt(t, "mut.update.suffix", 0, 25))), contacts[0].ID).Error)
		}
		if len(contacts) > 1 && drawBool(t, "mutate.softdelete") {
			require.NoError(t, db.Delete(&contacts[0]).Error)
		}
		if len(contacts) > 2 && drawBool(t, "mutate.harddelete") {
			require.NoError(t, db.Unscoped().Delete(&models.Contact{}, contacts[1].ID).Error)
		}
		if drawBool(t, "mutate.insert_softdeleted") {
			require.NoError(t, db.Exec(
				`INSERT INTO contacts (user_id, firstname, lastname, created_at, updated_at, deleted_at)
				 VALUES (?, 'AlreadyGone', 'Import', ?, ?, ?)`,
				user.ID, time.Now(), time.Now(), time.Now()).Error)
		}

		// The trigger-maintained index is consistent with canonical data
		// under the contract, whatever the sequence above did.
		cons, err := CheckSearchIndexConsistency(db)
		require.NoError(t, err)
		assert.Truef(t, cons.Clean(), "incremental index diverged from canonical data: %v", cons.Divergences)

		beforeLive, err := snapshotFTSLive(db)
		require.NoError(t, err)
		require.NoError(t, RebuildSearchIndex(db))
		afterLive, err := snapshotFTSLive(db)
		require.NoError(t, err)

		for table, b := range beforeLive {
			assert.Equalf(t, b, afterLive[table], "rebuild changed the live-row contents of %s (incremental index and RebuildSearchIndex disagree)", table)
		}

		// A rebuild leaves the index consistent too — no already-soft-deleted
		// leftovers, since it selects live rows only.
		consAfter, err := CheckSearchIndexConsistency(db)
		require.NoError(t, err)
		assert.Truef(t, consAfter.Clean(), "rebuilt index diverged from canonical data: %v", consAfter.Divergences)
	}))
}

// baseTableFor maps an FTS virtual table to the base table it derives from.
var baseTableFor = map[string]string{
	"contacts_fts":   "contacts",
	"notes_fts":      "notes",
	"activities_fts": "activities",
}

// snapshotFTSLive is snapshotFTS restricted to rows whose rowid is a live
// (deleted_at IS NULL) base row — the contract-aware view a rebuild and the
// incremental index must agree on exactly (see the file header).
func snapshotFTSLive(db *gorm.DB) (map[string]string, error) {
	out := make(map[string]string, len(ftsTables))
	for _, table := range ftsTables {
		query := fmt.Sprintf(
			`SELECT f.rowid, f.* FROM %s f
			 JOIN %s b ON b.id = f.rowid AND b.deleted_at IS NULL
			 ORDER BY f.rowid`, table, baseTableFor[table])
		s, err := dumpRows(db, query)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		out[table] = s
	}
	return out, nil
}

// snapshotFTS dumps the three FTS virtual tables as deterministic strings
// (rowid-ordered), keyed by table name.
func snapshotFTS(db *gorm.DB) (map[string]string, error) {
	out := make(map[string]string, len(ftsTables))
	for _, table := range ftsTables {
		s, err := dumpRows(db, "SELECT rowid, * FROM "+table+" ORDER BY rowid")
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		out[table] = s
	}
	return out, nil
}

// dumpRows renders a query result as a deterministic newline-joined string,
// each row rendered the same way (fmt.Sprint over the column values) so NULL
// and "" cannot masquerade as a match.
func dumpRows(db *gorm.DB, query string) (string, error) {
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var parts []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprint(vals...))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, "\n"), nil
}

func drawInt(t *rapid.T, label string, min, max int) int {
	return rapid.IntRange(min, max).Draw(t, label)
}

func drawBool(t *rapid.T, label string) bool {
	return rapid.Bool().Draw(t, label)
}
