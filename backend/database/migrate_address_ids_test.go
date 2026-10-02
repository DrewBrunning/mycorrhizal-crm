package database

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationBackfillsAddressIDs covers 000071 (issue #694, ADR 0031): every
// existing flat address gets a stable ID, the same ID lands on the index-paired
// card entry (so the T75 merge keeps seeing them as one projection), existing
// IDs and unprojected card components survive, and untouched rows stay
// byte-for-byte identical.
func TestMigrationBackfillsAddressIDs(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "address-ids.db")
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)
	require.NoError(t, m.Steps(70)) // everything before 000071

	_, err = sqlDB.Exec("INSERT INTO users (created_at, updated_at, username, password, email) VALUES (datetime('now'), datetime('now'), 'adr', 'x', 'adr@example.com')")
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

	flatOnly := insert("flatonly",
		`[{"type":"home","street":"1 Main St","city":"Springfield"},{"type":"work","street":"2 Side St"}]`, `{}`, false)
	paired := insert("paired",
		`[{"type":"home","street":"3 Oak Ave"}]`,
		`{"addresses":[{"components":[{"kind":"name","value":"3 Oak Ave"},{"kind":"building","value":"Tower B"}],"full":"3 Oak Ave"}],"name":{"full":"x"}}`, false)
	pairedWithID := insert("pairedid",
		`[{"type":"home","street":"4 Elm St"}]`,
		`{"addresses":[{"id":"imported-key","components":[{"kind":"name","value":"4 Elm St"}]}]}`, false)
	mismatched := insert("mismatch",
		`[{"type":"home","street":"5 Pine Rd"}]`,
		`{"addresses":[{"components":[{"kind":"name","value":"5 Pine Rd"}]},{"components":[{"kind":"name","value":"extra"}]}]}`, false)
	alreadyHas := insert("hasid",
		`[{"id":"keep-me","street":"6 Fir Ln"},{"street":"7 Ash Ct"}]`, `{}`, false)
	softDeleted := insert("deleted", `[{"street":"8 Birch Way"}]`, `{}`, true)
	emptyArr := insert("empty", `[]`, `{}`, false)
	invalid := insert("invalid", `not json`, `{}`, false)

	require.NoError(t, m.Steps(1)) // 000071

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
	cardAddrsOf := func(id int64) []map[string]any {
		t.Helper()
		_, c, _ := read(id)
		var card struct {
			Addresses []map[string]any `json:"addresses"`
		}
		require.NoError(t, json.Unmarshal([]byte(c), &card))
		return card.Addresses
	}
	uuidRE := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	// Flat-only: every element gets a distinct v4 UUID; other keys and order kept.
	fo := flatOf(flatOnly)
	require.Len(t, fo, 2)
	assert.Regexp(t, uuidRE, fo[0]["id"])
	assert.Regexp(t, uuidRE, fo[1]["id"])
	assert.NotEqual(t, fo[0]["id"], fo[1]["id"])
	assert.Equal(t, "1 Main St", fo[0]["street"])
	assert.Equal(t, "Springfield", fo[0]["city"])
	assert.Equal(t, "2 Side St", fo[1]["street"])
	_, c, _ := read(flatOnly)
	assert.JSONEq(t, `{}`, c, "a card with no addresses is left alone")

	// Paired: the SAME id on both copies, and the unprojected 'building'
	// component is still there.
	pf, pc := flatOf(paired), cardAddrsOf(paired)
	require.Len(t, pf, 1)
	require.Len(t, pc, 1)
	assert.Regexp(t, uuidRE, pf[0]["id"])
	assert.Equal(t, pf[0]["id"], pc[0]["id"], "flat and card copies must carry the same ID")
	assert.Contains(t, pc[0]["components"], map[string]any{"kind": "building", "value": "Tower B"})
	_, pcard, _ := read(paired)
	assert.Contains(t, pcard, `"name":{"full":"x"}`, "unrelated card members survive")

	// Paired with an existing card ID: the flat entry adopts it.
	assert.Equal(t, "imported-key", flatOf(pairedWithID)[0]["id"])
	assert.Equal(t, "imported-key", cardAddrsOf(pairedWithID)[0]["id"])

	// Mismatched lengths: flat stamped, card untouched.
	assert.Regexp(t, uuidRE, flatOf(mismatched)[0]["id"])
	for _, a := range cardAddrsOf(mismatched) {
		assert.NotContains(t, a, "id", "an un-paired card must not be guessed at")
	}

	// An element that already has an ID keeps it; its sibling gets one.
	ah := flatOf(alreadyHas)
	assert.Equal(t, "keep-me", ah[0]["id"])
	assert.Regexp(t, uuidRE, ah[1]["id"])

	// Soft-deleted rows are stamped too (undo must find an ID).
	assert.Regexp(t, uuidRE, flatOf(softDeleted)[0]["id"])

	// Empty array and invalid JSON are byte-for-byte untouched.
	a, _, _ := read(emptyArr)
	assert.Equal(t, `[]`, a)
	a, _, _ = read(invalid)
	assert.Equal(t, `not json`, a)

	// A schema backfill is not a user edit: updated_at never moves.
	for _, id := range []int64{flatOnly, paired, pairedWithID, mismatched, alreadyHas, softDeleted, emptyArr, invalid} {
		_, _, u := read(id)
		assert.Contains(t, u, "2020-01-02", "contact %d: updated_at must not change", id)
	}

	// The temp scratch table is gone.
	var n int
	require.NoError(t, sqlDB.QueryRow("SELECT COUNT(*) FROM sqlite_temp_master WHERE name = '_migration_address_ids'").Scan(&n))
	assert.Zero(t, n)

	// Idempotent: re-running the up script changes no ID.
	before := flatOf(flatOnly)
	script, err := os.ReadFile(filepath.Join("migrations", "000071_address_ids.up.sql"))
	require.NoError(t, err)
	_, err = sqlDB.Exec(string(script))
	require.NoError(t, err)
	assert.Equal(t, before, flatOf(flatOnly))
	assert.Equal(t, pf[0]["id"], flatOf(paired)[0]["id"])

	// Down strips the three keys from the flat copy and nothing else.
	_, err = sqlDB.Exec(`UPDATE contacts SET addresses = json_set(addresses, '$[0].coordinates', 'geo:1,2', '$[0].sensitivity', 'private') WHERE id = ?`, flatOnly)
	require.NoError(t, err)
	require.NoError(t, m.Steps(-1))
	down := flatOf(flatOnly)
	assert.Equal(t, map[string]any{"type": "home", "street": "1 Main St", "city": "Springfield"}, down[0])
	assert.Equal(t, map[string]any{"type": "work", "street": "2 Side St"}, down[1])
	a, _, _ = read(invalid)
	assert.Equal(t, `not json`, a, "down leaves unparseable rows alone")
	a, _, _ = read(emptyArr)
	assert.Equal(t, `[]`, a)
}
