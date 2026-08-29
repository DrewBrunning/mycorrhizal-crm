package contactgen

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestPopulate_CreatesContactsAndRoundTrips proves the DB helper persists
// generated records through the same path the REST API uses (trap #2) and
// that the authoritative read (RecordForContact, trap #3) returns the same
// neutral Record — the property suites' databases are therefore shaped exactly
// like the application's.
func TestPopulate_CreatesContactsAndRoundTrips(t *testing.T) {
	db := dbtest.New(t)
	user, err := NewUser(db, "populate")
	require.NoError(t, err)

	var recs []*contactmodel.Record
	t.Run("generate", rapid.MakeCheck(func(t *rapid.T) {
		recs = append(recs, Record(t))
	}))
	ensureUniqueUIDs(recs)

	contacts, err := Populate(db, user.ID, recs)
	require.NoError(t, err)
	require.Len(t, contacts, len(recs))

	for i, c := range contacts {
		var reloaded models.Contact
		require.NoError(t, db.First(&reloaded, c.ID).Error)
		got := models.RecordForContact(&reloaded, "", nil)
		require.NotNil(t, got, "RecordForContact must never return nil for a stored contact")

		want := canonicalize(t, recs[i])
		gotRec := canonicalize(t, got)
		assert.Equal(t, want.Card, gotRec.Card, "contact %d: card did not round-trip", i)
		assert.Equal(t, want.Envelope, gotRec.Envelope, "contact %d: crm envelope did not round-trip", i)
		assert.Equal(t, want.Passthrough, gotRec.Passthrough, "contact %d: passthrough did not round-trip", i)
		assert.Equal(t, reloaded.VCardUID, gotRec.Card.UID, "contact %d: VCardUID and Card.UID drifted", i)
	}
}

// TestPopulate_DuplicateUIDFails pins the error path: two generated records
// sharing a UID must be rejected by the partial unique VCardUID index rather
// than silently producing duplicate keys (which is exactly what the property
// suites' distinct-UID contract prevents).
func TestPopulate_DuplicateUIDFails(t *testing.T) {
	db := dbtest.New(t)
	user, err := NewUser(db, "populate-dup")
	require.NoError(t, err)

	rec := &contactmodel.Record{Card: contactmodel.Card{UID: "urn:uuid:duplicate"}}
	_, err = Populate(db, user.ID, []*contactmodel.Record{rec, rec})
	require.Error(t, err, "a duplicate VCardUID must fail the unique index")
}

// TestNewUser_CreatesScopedUser pins the user helper.
func TestNewUser_CreatesScopedUser(t *testing.T) {
	db := dbtest.New(t)
	u, err := NewUser(db, "scoped")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", u.Username).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// TestNewUser_DuplicateLabelFails pins the error path: a second user with the
// same label is rejected by the unique username index, which is what keeps
// generated users scoped.
func TestNewUser_DuplicateLabelFails(t *testing.T) {
	db := dbtest.New(t)
	_, err := NewUser(db, "dup")
	require.NoError(t, err)

	_, err = NewUser(db, "dup")
	require.Error(t, err, "a duplicate generated username must fail the unique index")
}

// TestCopyFile_MissingSourceFails pins copyFile's error path (used by
// MigratedDB).
func TestCopyFile_MissingSourceFails(t *testing.T) {
	err := copyFile(filepath.Join(t.TempDir(), "does-not-exist"), filepath.Join(t.TempDir(), "out.db"))
	require.Error(t, err)
}

// TestMigratedDB_IsFullyMigrated proves the DB helper hands back a real
// migrated schema (trap #1) that accepts application writes.
func TestMigratedDB_IsFullyMigrated(t *testing.T) {
	db, path, err := MigratedDB(t)
	require.NoError(t, err)
	require.NotEmpty(t, path)

	u, err := NewUser(db, "migrated")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", u.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "a write must persist in the migrated template copy")
}

// ensureUniqueUIDs makes a batch of generated records safe to persist: every
// card carries a non-empty UID and no two collide (the partial unique
// VCardUID index), so a check that drew records with empty UIDs (a legitimate
// generated shape) still populates cleanly.
func ensureUniqueUIDs(recs []*contactmodel.Record) {
	seen := map[string]bool{}
	next := 0
	for _, r := range recs {
		if r.Card.UID == "" || seen[r.Card.UID] {
			next++
			r.Card.UID = fmt.Sprintf("urn:uuid:test-%d", next)
		}
		seen[r.Card.UID] = true
	}
}

func canonicalize(t *testing.T, rec *contactmodel.Record) *contactmodel.Record {
	t.Helper()
	data, err := json.Marshal(rec)
	require.NoError(t, err)
	var out contactmodel.Record
	require.NoError(t, json.Unmarshal(data, &out))
	return &out
}
