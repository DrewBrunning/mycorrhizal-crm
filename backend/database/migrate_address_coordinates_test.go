package database

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationBackfillsAddressCoordinates covers 000073 (issue #1440):
// a coordinate / sensitivity that lives only on the neutral card entry is
// copied down onto the index-paired flat address (so GET /contacts/map sees
// it), a flat value that is already present wins, an unpaired (length-
// mismatched) card is left alone, untouched rows stay byte-for-byte identical,
// the migration is idempotent, and its down is a documented no-op that never
// destroys the backfilled data.
func TestMigrationBackfillsAddressCoordinates(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "address-coords.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(72)) // everything before 000073

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'coord', 'x', 'coord@example.com')")
	require.NoError(t, err)

	const stamp = "2020-01-02 03:04:05"
	insert := func(name, addresses, card string, deleted bool) int64 {
		t.Helper()
		var del any
		if deleted {
			del = "2021-01-01 00:00:00"
		}
		res, err := sqlDB.Exec(
			"INSERT INTO contacts (created_at, updated_at, deleted_at, firstname, user_id, addresses, card) VALUES (?, ?, ?, ?, 1, ?, ?)",
			stamp, stamp, del, name, addresses, card)
		require.NoError(t, err)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}

	// Paired, card-only coordinate + sensitivity: both flow down, the
	// unprojected 'building' component and every flat key survive.
	cardOnly := insert("cardonly",
		`[{"id":"a-1","type":"home","street":"1 Main St"}]`,
		`{"addresses":[{"id":"a-1","components":[{"kind":"name","value":"1 Main St"},{"kind":"building","value":"The Tower"}],"coordinates":"geo:48.2,16.3","sensitivity":"private"}]}`, false)
	// Paired, coordinate only: sensitivity is never invented.
	coordinateOnly := insert("coordinateonly",
		`[{"id":"c-1","street":"2 Side St"}]`,
		`{"addresses":[{"id":"c-1","components":[{"kind":"name","value":"2 Side St"}],"coordinates":"geo:1,1"}]}`, false)
	// Paired, sensitivity only: coordinates is never invented.
	sensitivityOnly := insert("sensitivityonly",
		`[{"id":"s-1","street":"3 Oak Ave"}]`,
		`{"addresses":[{"id":"s-1","components":[{"kind":"name","value":"3 Oak Ave"}],"sensitivity":"secret"}]}`, false)
	// Paired, flat already carries both: user data wins, nothing changes.
	alreadySet := insert("alreadyset",
		`[{"id":"u-1","street":"4 Elm St","coordinates":"geo:7,7","sensitivity":"normal"}]`,
		`{"addresses":[{"id":"u-1","components":[{"kind":"name","value":"4 Elm St"}],"coordinates":"geo:8,8","sensitivity":"private"}]}`, false)
	// Paired, card has nothing to copy: byte-for-byte untouched.
	cardNoMap := insert("cardnomap",
		`[{"id":"n-1","street":"6 Fir Ln"}]`,
		`{"addresses":[{"id":"n-1","components":[{"kind":"name","value":"6 Fir Ln"}]}]}`, false)
	// Length mismatch: card has two addresses, flat one -> unpaired, untouched.
	mismatched := insert("mismatch",
		`[{"id":"m-1","street":"5 Pine Rd"}]`,
		`{"addresses":[{"id":"m-1","components":[{"kind":"name","value":"5 Pine Rd"}],"coordinates":"geo:9,9"},{"components":[{"kind":"name","value":"extra"}],"coordinates":"geo:8,8"}]}`, false)
	// Soft-deleted rows are backfilled too (the map excludes them, but undo
	// and a later restore must find the data).
	softDeleted := insert("deleted",
		`[{"id":"d-1","street":"8 Birch Way"}]`,
		`{"addresses":[{"id":"d-1","components":[{"kind":"name","value":"8 Birch Way"}],"coordinates":"geo:2,2"}]}`, true)
	// A card with no addresses, an empty flat array and invalid JSON are all
	// byte-for-byte untouched.
	cardEmpty := insert("cardempty", `[{"id":"e-1","street":"x"}]`, `{}`, false)
	emptyArr := insert("empty", `[]`, `{"addresses":[]}`, false)
	invalid := insert("invalid", `not json`, `{}`, false)

	read := func(id int64) (addresses, card, updatedAt string) {
		t.Helper()
		require.NoError(t, sqlDB.QueryRow("SELECT addresses, card, updated_at FROM contacts WHERE id = ?", id).Scan(&addresses, &card, &updatedAt))
		return
	}
	flatOf := func(id int64) []map[string]any {
		t.Helper()
		a, _, _ := read(id)
		var out []map[string]any
		require.NoError(t, json.Unmarshal([]byte(a), &out))
		return out
	}
	_, cardBefore, _ := read(cardOnly)

	require.NoError(t, m.Steps(1)) // 000073

	// Card-only coordinate + sensitivity flow down; the card copy and every
	// other flat key are preserved.
	co := flatOf(cardOnly)
	require.Len(t, co, 1)
	assert.Equal(t, "geo:48.2,16.3", co[0]["coordinates"])
	assert.Equal(t, "private", co[0]["sensitivity"])
	assert.Equal(t, "a-1", co[0]["id"])
	assert.Equal(t, "home", co[0]["type"])
	assert.Equal(t, "1 Main St", co[0]["street"])
	_, cardAfter, _ := read(cardOnly)
	assert.Equal(t, cardBefore, cardAfter, "the card copy is not touched")

	// Coordinate-only / sensitivity-only never invent the other field.
	cco := flatOf(coordinateOnly)
	require.Len(t, cco, 1)
	assert.Equal(t, "geo:1,1", cco[0]["coordinates"])
	assert.NotContains(t, cco[0], "sensitivity", "sensitivity must not be invented from nothing")
	sso := flatOf(sensitivityOnly)
	require.Len(t, sso, 1)
	assert.Equal(t, "secret", sso[0]["sensitivity"])
	assert.NotContains(t, sso[0], "coordinates", "coordinates must not be invented from nothing")

	// A flat value that is already present wins; nothing is overwritten.
	as := flatOf(alreadySet)
	require.Len(t, as, 1)
	assert.Equal(t, "geo:7,7", as[0]["coordinates"])
	assert.Equal(t, "normal", as[0]["sensitivity"])

	// Nothing to copy: untouched.
	cn := flatOf(cardNoMap)
	require.Len(t, cn, 1)
	assert.NotContains(t, cn[0], "coordinates")
	assert.NotContains(t, cn[0], "sensitivity")

	// Length mismatch: the card is never guessed at; the flat entry stays bare.
	mm := flatOf(mismatched)
	require.Len(t, mm, 1)
	assert.NotContains(t, mm[0], "coordinates", "an un-paired card must not be guessed at")
	assert.NotContains(t, mm[0], "sensitivity")

	// Soft-deleted rows are backfilled.
	assert.Equal(t, "geo:2,2", flatOf(softDeleted)[0]["coordinates"])

	// Empty array / no card addresses / invalid JSON are byte-for-byte untouched.
	a, _, _ := read(cardEmpty)
	assert.Equal(t, `[{"id":"e-1","street":"x"}]`, a)
	a, _, _ = read(emptyArr)
	assert.Equal(t, `[]`, a)
	a, _, _ = read(invalid)
	assert.Equal(t, `not json`, a)

	// A schema backfill is not a user edit: updated_at never moves.
	for _, id := range []int64{cardOnly, coordinateOnly, sensitivityOnly, alreadySet, cardNoMap, mismatched, softDeleted, cardEmpty, emptyArr, invalid} {
		_, _, u := read(id)
		assert.Contains(t, u, "2020-01-02", "contact %d: updated_at must not change", id)
	}

	// Idempotent: re-running the up script changes nothing.
	before := flatOf(cardOnly)
	script, err := os.ReadFile(filepath.Join("migrations", "000073_address_coordinates_backfill.up.sql"))
	require.NoError(t, err)
	_, err = sqlDB.Exec(string(script))
	require.NoError(t, err)
	assert.Equal(t, before, flatOf(cardOnly))
	assert.Equal(t, "geo:1,1", flatOf(coordinateOnly)[0]["coordinates"])

	// Down is a documented no-op: it must NOT un-backfill (that would be
	// data loss), so the coordinate is still there afterwards.
	require.NoError(t, m.Steps(-1))
	assert.Equal(t, "geo:48.2,16.3", flatOf(cardOnly)[0]["coordinates"], "down must not clear the backfilled coordinate")
	assert.Equal(t, "private", flatOf(cardOnly)[0]["sensitivity"])
}
