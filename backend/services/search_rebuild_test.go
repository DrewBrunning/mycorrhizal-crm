// SEARCH-01 (issue #461): the FTS rebuild as a first-class operational
// capability. These tests pin the ticket's "How to verify" list:
//
//   - a rebuild reproduces canonical data exactly — for all three indexes,
//     checked against a direct base-table scan (ftsDivergence);
//   - a rebuild interrupted partway leaves the previously-good index intact,
//     never a partial one (the single-transaction guarantee);
//   - concurrent rebuilds, and a rebuild racing ordinary writes, converge on
//     a correct index and never corrupt it;
//   - the per-index row counts the rebuild reports are the live-row counts.
//
// The negative control (TestFtsDivergence_DetectsEachClass) is the
// "hand-verify per CLAUDE.md" step encoded: it proves ftsDivergence actually
// fails when the index is wrong, so a green equivalence assertion means
// something.
package services

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// scanKeyed runs query (first column = rowid) and returns rowid -> a
// deterministic string of the remaining columns, rendered the same way on
// both sides of a comparison so NULL vs "" can never masquerade as a match.
func scanKeyed(t *testing.T, db *gorm.DB, query string) map[int64]string {
	t.Helper()
	rows, err := db.Raw(query).Rows()
	require.NoError(t, err)
	defer rows.Close()

	cols, err := rows.Columns()
	require.NoError(t, err)

	out := make(map[int64]string)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, rows.Scan(ptrs...))
		var rid int64
		switch v := vals[0].(type) {
		case int64:
			rid = v
		default:
			t.Fatalf("scanKeyed: first column of %q is not an integer rowid (got %T)", query, vals[0])
		}
		out[rid] = fmt.Sprint(vals[1:]...)
	}
	require.NoError(t, rows.Err())
	return out
}

// ftsDivergence compares each FTS virtual table against the live
// (deleted_at IS NULL) base rows a rebuild derives it from, and returns one
// line per mismatch. An empty result means the index matches canonical data
// exactly — the equivalence the ticket's verification calls for.
//
// It is a test oracle for a freshly-rebuilt index, not the shipped SEARCH-02
// consistency check (#462): it deliberately treats any soft-deleted row left
// in the index as divergence, which is correct for validating rebuild output
// (a rebuild inserts live rows only) but is NOT the trigger-maintained
// index's contract.
func ftsDivergence(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var diffs []string

	cases := []struct {
		name      string
		ftsQuery  string
		baseQuery string
	}{
		{
			"contacts_fts",
			`SELECT rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts_fts`,
			`SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts WHERE deleted_at IS NULL`,
		},
		{
			"notes_fts",
			`SELECT rowid, user_id, content FROM notes_fts`,
			`SELECT id, user_id, content FROM notes WHERE deleted_at IS NULL`,
		},
		{
			"activities_fts",
			`SELECT rowid, user_id, title, description, location FROM activities_fts`,
			`SELECT id, user_id, title, description, location FROM activities WHERE deleted_at IS NULL`,
		},
	}

	for _, c := range cases {
		fts := scanKeyed(t, db, c.ftsQuery)
		base := scanKeyed(t, db, c.baseQuery)
		for rid, bv := range base {
			fv, ok := fts[rid]
			if !ok {
				diffs = append(diffs, fmt.Sprintf("%s: base row %d missing from index", c.name, rid))
				continue
			}
			if fv != bv {
				diffs = append(diffs, fmt.Sprintf("%s: row %d indexed content stale (index=%q base=%q)", c.name, rid, fv, bv))
			}
		}
		for rid := range fts {
			if _, ok := base[rid]; !ok {
				diffs = append(diffs, fmt.Sprintf("%s: index row %d has no live base row", c.name, rid))
			}
		}
	}
	return diffs
}

// rebuildTestUser makes a user with a unique-enough name for these tests.
func rebuildTestUser(t *testing.T, db *gorm.DB, tag string) models.User {
	t.Helper()
	u := models.User{Username: "rebuild-" + tag, Password: "password123!A", Email: "rebuild-" + tag + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// seedRebuildCorpus creates n contacts (each with a note and an activity) for
// userID and returns the contacts. Every third contact is soft-deleted, so
// the corpus exercises the live/deleted split a rebuild must honour.
func seedRebuildCorpus(t *testing.T, db *gorm.DB, userID uint, n int) (live, deleted int) {
	t.Helper()
	for i := 0; i < n; i++ {
		c := models.Contact{
			UserID:    userID,
			Firstname: fmt.Sprintf("First%d", i),
			Lastname:  fmt.Sprintf("Last%d", i),
			Email:     fmt.Sprintf("person%d@example.com", i),
		}
		require.NoError(t, db.Create(&c).Error)
		require.NoError(t, db.Create(&models.Note{UserID: userID, ContactID: &c.ID, Content: fmt.Sprintf("note body %d about the project", i), Date: time.Now()}).Error)
		require.NoError(t, db.Create(&models.Activity{UserID: userID, Title: fmt.Sprintf("activity %d", i), Description: "call", Date: time.Now()}).Error)

		if i%3 == 0 {
			require.NoError(t, db.Delete(&c).Error)
			deleted++
		} else {
			live++
		}
	}
	return live, deleted
}

// TestRebuildSearchIndexReport_CountsAreLiveRowCounts pins recommended
// action 3/6: the report covers all three indexes and its numbers are the
// live-row counts (soft-deleted rows excluded).
func TestRebuildSearchIndexReport_CountsAreLiveRowCounts(t *testing.T) {
	db := dbtest.New(t)
	user := rebuildTestUser(t, db, "counts")
	liveContacts, _ := seedRebuildCorpus(t, db, user.ID, 9)

	stats, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)

	var wantNotes, wantActs int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM notes WHERE deleted_at IS NULL`).Scan(&wantNotes).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM activities WHERE deleted_at IS NULL`).Scan(&wantActs).Error)

	assert.Equal(t, int64(liveContacts), stats.Contacts, "only live contacts are indexed")
	assert.Equal(t, wantNotes, stats.Notes)
	assert.Equal(t, wantActs, stats.Activities)
	assert.Equal(t, stats.Contacts+stats.Notes+stats.Activities, stats.Total())
	assert.Empty(t, ftsDivergence(t, db), "a fresh rebuild matches canonical data exactly")
}

// TestRebuildSearchIndex_ReproducesCanonicalDataAfterCorruption is the
// ticket's headline verification: deliberately corrupt every FTS table in
// every divergence class, rebuild, and confirm the index matches a direct
// base-table scan exactly.
func TestRebuildSearchIndex_ReproducesCanonicalDataAfterCorruption(t *testing.T) {
	db := dbtest.New(t)
	userA := rebuildTestUser(t, db, "corruptA")
	userB := rebuildTestUser(t, db, "corruptB")
	seedRebuildCorpus(t, db, userA.ID, 7)
	seedRebuildCorpus(t, db, userB.ID, 5)

	// Class 1: rows dropped from the index entirely.
	require.NoError(t, db.Exec(`DELETE FROM contacts_fts WHERE rowid IN (SELECT rowid FROM contacts_fts LIMIT 3)`).Error)
	require.NoError(t, db.Exec(`DELETE FROM notes_fts`).Error)
	// Class 2: an orphan index row with no base row at all.
	require.NoError(t, db.Exec(`INSERT INTO activities_fts(rowid, user_id, title, description, location) VALUES (999999, ?, 'ghost', '', '')`, userA.ID).Error)
	// Class 3: stale indexed content (the trigger-bypassing-migration class).
	require.NoError(t, db.Exec(`UPDATE contacts_fts SET firstname = 'STALE' WHERE rowid IN (SELECT rowid FROM contacts_fts LIMIT 2)`).Error)
	require.NoError(t, db.Exec(`UPDATE activities_fts SET title = 'STALE' WHERE rowid IN (SELECT rowid FROM activities_fts LIMIT 2)`).Error)

	require.NotEmpty(t, ftsDivergence(t, db), "sanity: the corruption is visible before the rebuild")

	_, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)

	assert.Empty(t, ftsDivergence(t, db), "after a rebuild every index matches canonical data exactly")

	// And search actually works for a known row on each side.
	res, err := Search(db, userA.ID, "First1", 0, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Contacts, "a known contact is findable again after the rebuild")
}

// TestFtsDivergence_DetectsEachClass is the negative control for the oracle
// above — the CLAUDE.md "break it and confirm the check fails" step, encoded.
// If ftsDivergence ever stops noticing a corruption class, the equivalence
// assertions elsewhere in this file become worthless, so this guards them.
func TestFtsDivergence_DetectsEachClass(t *testing.T) {
	newCorpus := func(t *testing.T) (*gorm.DB, models.User) {
		db := dbtest.New(t)
		u := rebuildTestUser(t, db, "neg")
		seedRebuildCorpus(t, db, u.ID, 6)
		_, err := RebuildSearchIndexReport(db)
		require.NoError(t, err)
		require.Empty(t, ftsDivergence(t, db), "clean after rebuild")
		return db, u
	}

	t.Run("missing base row", func(t *testing.T) {
		db, _ := newCorpus(t)
		require.NoError(t, db.Exec(`DELETE FROM contacts_fts WHERE rowid = (SELECT min(rowid) FROM contacts_fts)`).Error)
		assert.NotEmpty(t, ftsDivergence(t, db))
	})
	t.Run("orphan index row", func(t *testing.T) {
		db, u := newCorpus(t)
		require.NoError(t, db.Exec(`INSERT INTO notes_fts(rowid, user_id, content) VALUES (888888, ?, 'orphan')`, u.ID).Error)
		assert.NotEmpty(t, ftsDivergence(t, db))
	})
	t.Run("stale content", func(t *testing.T) {
		db, _ := newCorpus(t)
		require.NoError(t, db.Exec(`UPDATE activities_fts SET description = 'drifted' WHERE rowid = (SELECT min(rowid) FROM activities_fts)`).Error)
		assert.NotEmpty(t, ftsDivergence(t, db))
	})
	t.Run("user_id mismatch", func(t *testing.T) {
		db, _ := newCorpus(t)
		require.NoError(t, db.Exec(`UPDATE contacts_fts SET user_id = user_id + 1 WHERE rowid = (SELECT min(rowid) FROM contacts_fts)`).Error)
		assert.NotEmpty(t, ftsDivergence(t, db), "a scoping drift is a divergence too")
	})
}

// TestRebuildSearchIndex_InterruptionLeavesOldIndexIntact pins recommended
// action 2 / the "interrupted partway" verification. A failure injected after
// two of the three INSERTs have run must roll the whole rebuild back, leaving
// the previously-good index exactly as it was — never a partial one.
func TestRebuildSearchIndex_InterruptionLeavesOldIndexIntact(t *testing.T) {
	db := dbtest.New(t)
	user := rebuildTestUser(t, db, "interrupt")
	seedRebuildCorpus(t, db, user.ID, 8)

	// Start from a known-good index.
	_, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)
	require.Empty(t, ftsDivergence(t, db))
	good := scanKeyed(t, db, `SELECT rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts_fts`)
	goodNotes := scanKeyed(t, db, `SELECT rowid, user_id, content FROM notes_fts`)

	// Inject a mid-transaction failure: error out the third INSERT
	// (activities_fts), after contacts_fts and notes_fts have already been
	// cleared and repopulated inside the open transaction.
	const cbName = "rebuild_test_fail_activities_fts"
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(cbName, func(d *gorm.DB) {
		if strings.Contains(d.Statement.SQL.String(), "INSERT INTO activities_fts") {
			_ = d.AddError(fmt.Errorf("injected failure"))
		}
	}))

	_, err = RebuildSearchIndexReport(db)
	require.Error(t, err, "the injected failure must surface")

	require.NoError(t, db.Callback().Raw().Remove(cbName))

	// The transaction rolled back: the old index is byte-for-byte intact and
	// still matches canonical data. Nothing partial.
	assert.Equal(t, good, scanKeyed(t, db, `SELECT rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts_fts`),
		"contacts_fts must be unchanged after a rolled-back rebuild")
	assert.Equal(t, goodNotes, scanKeyed(t, db, `SELECT rowid, user_id, content FROM notes_fts`),
		"notes_fts must be unchanged after a rolled-back rebuild")
	assert.Empty(t, ftsDivergence(t, db), "the surviving index still matches canonical data")

	// And a real rebuild still works once the fault is cleared.
	_, err = RebuildSearchIndexReport(db)
	require.NoError(t, err)
	assert.Empty(t, ftsDivergence(t, db))
}

// TestRebuildSearchIndex_FailureInClearPhaseRollsBack covers the other
// failure edge: a fault during the initial DELETE phase (here, a missing FTS
// table) also aborts the whole rebuild with the previous index intact.
func TestRebuildSearchIndex_FailureInClearPhaseRollsBack(t *testing.T) {
	db := dbtest.New(t)
	user := rebuildTestUser(t, db, "clearfail")
	seedRebuildCorpus(t, db, user.ID, 5)

	_, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)
	good := scanKeyed(t, db, `SELECT rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts_fts`)
	require.NotEmpty(t, good)

	// notes_fts vanishes (a botched migration, a manual DROP) — the rebuild's
	// `DELETE FROM notes_fts` now errors before any INSERT runs.
	require.NoError(t, db.Exec(`DROP TABLE notes_fts`).Error)

	_, err = RebuildSearchIndexReport(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "clear")

	assert.Equal(t, good, scanKeyed(t, db, `SELECT rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized FROM contacts_fts`),
		"contacts_fts is untouched when the clear phase fails")
}

// TestRebuildSearchIndexExclusive_SkipsWhenAlreadyRunning pins recommended
// action 4: a second rebuild while one is in progress in this process is
// skipped (ErrJobSkipped), not queued or run concurrently. The in-process
// mutex is held directly here so the race is deterministic.
func TestRebuildSearchIndexExclusive_SkipsWhenAlreadyRunning(t *testing.T) {
	db := dbtest.New(t)
	user := rebuildTestUser(t, db, "exclusive")
	seedRebuildCorpus(t, db, user.ID, 3)

	searchIndexRebuildMu.Lock()
	_, err := RebuildSearchIndexExclusive(db)
	searchIndexRebuildMu.Unlock()

	require.ErrorIs(t, err, ErrJobSkipped, "a rebuild already in progress is reported as skipped")

	// The guard releases: a subsequent rebuild runs normally.
	_, err = RebuildSearchIndexExclusive(db)
	require.NoError(t, err)
	assert.Empty(t, ftsDivergence(t, db))
}

// TestRebuildSearchIndex_ConcurrentRebuildsAndWritesConverge pins recommended
// action 4's core: many rebuilds at once, plus ordinary create/soft-delete
// traffic racing them, must converge on a correct index and never corrupt the
// database.
func TestRebuildSearchIndex_ConcurrentRebuildsAndWritesConverge(t *testing.T) {
	db := dbtest.New(t)
	user := rebuildTestUser(t, db, "concurrent")
	seedRebuildCorpus(t, db, user.ID, 12)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var ran, skipped int
	var badErr error

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := RebuildSearchIndexExclusive(db)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ran++
			case err == ErrJobSkipped:
				skipped++
			default:
				badErr = err
			}
		}()
	}
	// Also race the unguarded primitive against itself: two rebuild
	// transactions in flight at once must be serialised by SQLite
	// (_txlock=immediate) and each converge on the same correct index —
	// the in-process mutex is not what makes concurrent rebuilds safe.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := RebuildSearchIndexReport(db); err != nil {
				mu.Lock()
				badErr = err
				mu.Unlock()
			}
		}()
	}
	// Ordinary write traffic racing the rebuilds — the triggers keep the
	// index live while the rebuilds re-derive it.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := models.Contact{UserID: user.ID, Firstname: fmt.Sprintf("Race%d", i), Lastname: "Runner"}
			if err := db.Create(&c).Error; err != nil {
				mu.Lock()
				badErr = err
				mu.Unlock()
				return
			}
			if i%2 == 0 {
				_ = db.Delete(&c).Error
			}
		}(i)
	}
	wg.Wait()

	require.NoError(t, badErr, "no rebuild or write may error under contention")
	assert.Positive(t, ran, "at least one concurrent rebuild ran")
	t.Logf("concurrent rebuilds: ran=%d skipped=%d", ran, skipped)

	// A final settling rebuild, then the index must match canonical data and
	// the database must pass an integrity check.
	_, err := RebuildSearchIndexExclusive(db)
	require.NoError(t, err)
	assert.Empty(t, ftsDivergence(t, db), "the index converges on canonical data")

	var integ string
	require.NoError(t, db.Raw(`PRAGMA integrity_check`).Scan(&integ).Error)
	assert.Equal(t, "ok", integ, "no rebuild/write interleaving corrupted the database")
}
