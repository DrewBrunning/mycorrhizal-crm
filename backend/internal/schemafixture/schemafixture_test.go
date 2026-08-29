package schemafixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestEachFixtureLoadsAtItsVersion is MIG-01's "each loads into a fresh SQLite
// database without error, reports the expected schema_migrations version and
// is not dirty" (issue #436 verify). A fixture that omitted the schema_migrations
// row would silently present version 0 — this is what pins it present and clean.
func TestEachFixtureLoadsAtItsVersion(t *testing.T) {
	for _, r := range SupportedReleases {
		t.Run(r.Tag, func(t *testing.T) {
			f := Load(t, r)

			version, dirty, ok, err := database.AppliedMigrationVersion(f.DB)
			require.NoError(t, err)
			require.True(t, ok, "fixture must carry a schema_migrations row")
			assert.EqualValues(t, r.Version, version, "fixture must present the release's schema version")
			assert.False(t, dirty, "fixture must not be dirty")
		})
	}
}

// TestFixtureManifestDataRowCounts is MIG-01's "the TEST-02 manifest populates
// each fixture with non-trivial data" (issue #436 verify): every table the
// manifest populates must hold the same rows in the historical fixture as in a
// current-schema population of the same manifest — contacts, relationships,
// custom fields, notes, life events, gifts, files (attachments), and external
// references included, soft-deleted rows and cascades intact.
func TestFixtureManifestDataRowCounts(t *testing.T) {
	reference := referenceCounts(t)

	for _, r := range SupportedReleases {
		t.Run(r.Tag, func(t *testing.T) {
			f := Load(t, r)
			got := tableCounts(t, f.DB)
			for table, want := range reference {
				_, exists := got[table]
				if !exists {
					// Table added after this release — the manifest never
					// populates it, so nothing to assert.
					continue
				}
				assert.EqualValuesf(t, want, got[table],
					"table %s must carry the same rows as a current-schema population of the manifest", table)
			}
		})
	}
}

// referenceCounts builds one current-schema database populated from the
// canonical manifest and returns every table's row count. It is the ground
// truth the historical fixtures must reproduce exactly.
func referenceCounts(t *testing.T) map[string]int64 {
	t.Helper()
	db := dbtest.New(t)
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	_, err = canonicalfixture.Populate(db, m)
	require.NoError(t, err)
	return tableCounts(t, db)
}

// tableCounts returns the live row count of every table in the database
// (COUNT(*) — soft-deleted rows included, matching Unscoped semantics). FTS
// virtual tables and their shadow tables are excluded: their contents are
// derived data (rebuilt by triggers) and not comparable across schemas.
func tableCounts(t *testing.T, db *gorm.DB) map[string]int64 {
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

	out := make(map[string]int64, len(objects))
	virtual := map[string]bool{}
	for _, o := range objects {
		if strings.HasPrefix(o.SQL, "CREATE VIRTUAL TABLE") {
			virtual[o.Name] = true
		}
	}
	for _, o := range objects {
		if virtual[o.Name] || isShadowOfVirtual(o.Name, virtual) {
			continue
		}
		var n int64
		require.NoError(t, db.Raw("SELECT count(*) FROM \""+strings.ReplaceAll(o.Name, `"`, `""`)+"\"").Scan(&n).Error)
		out[o.Name] = n
	}
	return out
}

// TestFixtureContainsManifestTrapData spot-checks that the copy into the
// historical schema preserved value-level detail the row-count check cannot
// see: the soft-deleted contact stays soft-deleted alongside julie's recreated
// row sharing its vcard_uid, and confirmed relationship edges keep their shape.
func TestFixtureContainsManifestTrapData(t *testing.T) {
	for _, r := range SupportedReleases {
		t.Run(r.Tag, func(t *testing.T) {
			f := Load(t, r)

			gina := f.Dataset.Contacts["gina"]
			require.NotNil(t, gina)

			var live, tombstoned int64
			require.NoError(t, f.DB.Raw("SELECT count(*) FROM contacts WHERE vcard_uid = ? AND deleted_at IS NULL", gina.VCardUID).Scan(&live).Error)
			require.NoError(t, f.DB.Raw("SELECT count(*) FROM contacts WHERE vcard_uid = ? AND deleted_at IS NOT NULL", gina.VCardUID).Scan(&tombstoned).Error)
			assert.EqualValues(t, 1, tombstoned, "gina must still be soft-deleted in the historical fixture")
			assert.EqualValues(t, 1, live, "julie's recreated contact must share the vcard_uid live")

			var edgeCount int64
			require.NoError(t, f.DB.Raw(`
				SELECT count(*) FROM relationship_edges
				WHERE status = 'confirmed'`).Scan(&edgeCount).Error)
			assert.Positive(t, edgeCount, "fixture must carry confirmed relationship edges")
		})
	}
}

// TestSchemaDumpsReproduceCurrentChain is the frozen-and-append-only guard
// (issue #436 action 1 + 4): the migration chain is never edited retroactively,
// so regenerating a release's dump from the CURRENT embedded migrations must
// produce a byte-identical file. A stale dump, or an edit to an old migration
// file, fails here. Regenerate with `cd backend && go run ./cmd/genschema`.
func TestSchemaDumpsReproduceCurrentChain(t *testing.T) {
	dir, err := findSchemaDir()
	require.NoError(t, err)

	for _, r := range SupportedReleases {
		t.Run(r.Tag, func(t *testing.T) {
			generated, err := GenerateDump(r.Version, r.Tag)
			require.NoError(t, err)
			committed, err := os.ReadFile(filepath.Join(dir, DumpFile(r)))
			require.NoError(t, err)
			assert.Equal(t, string(committed), generated,
				"committed schema dump drifted from the current migration chain — run `go run ./cmd/genschema` from backend/")
		})
	}
}

// TestEverySupportedReleaseHasADump checks the fixture set's internal
// completeness: every entry in SupportedReleases has a committed dump whose
// schema_migrations row matches the registry's version. The "new release
// without a dump fails CI" gate at tag-push time is the docker-publish.yml job
// that enumerates release tags against this same registry.
func TestEverySupportedReleaseHasADump(t *testing.T) {
	dir, err := findSchemaDir()
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	committed := map[string]bool{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			committed[e.Name()] = true
		}
	}

	for _, r := range SupportedReleases {
		require.Truef(t, committed[DumpFile(r)], "release %s is in SupportedReleases but has no committed schema dump (run `go run ./cmd/genschema` from backend/)", r.Tag)
	}
}

// TestFilterShadowTables pins the dump-filtering rule: virtual tables and FTS
// triggers survive (they share the `<virtual-table>_<suffix>` naming), shadow
// tables are dropped, and unrelated tables are untouched. The trigger case is
// the load-bearing one — dropping FTS triggers made search silently empty
// after a fixture load.
func TestFilterShadowTables(t *testing.T) {
	objs := []schemaObject{
		{Type: "table", Name: "contacts", SQL: "CREATE TABLE contacts (id INTEGER)"},
		{Type: "table", Name: "contacts_fts", SQL: "CREATE VIRTUAL TABLE contacts_fts USING fts5(...)"},
		{Type: "table", Name: "contacts_fts_data", SQL: "CREATE TABLE 'contacts_fts_data'(...)"},
		{Type: "table", Name: "contacts_fts_content", SQL: "CREATE TABLE 'contacts_fts_content'(...)"},
		{Type: "trigger", Name: "contacts_fts_ai", SQL: "CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN END"},
		{Type: "trigger", Name: "contacts_fts_au", SQL: "CREATE TRIGGER contacts_fts_au AFTER UPDATE ON contacts BEGIN END"},
		{Type: "index", Name: "idx_contacts_user", SQL: "CREATE INDEX idx_contacts_user ON contacts(user_id)"},
	}

	got := filterShadowTables(objs)
	names := map[string]bool{}
	for _, o := range got {
		names[o.Name] = true
	}

	assert.True(t, names["contacts"], "base table must survive")
	assert.True(t, names["contacts_fts"], "virtual table must survive")
	assert.False(t, names["contacts_fts_data"], "shadow table must be dropped")
	assert.False(t, names["contacts_fts_content"], "shadow table must be dropped")
	assert.True(t, names["contacts_fts_ai"], "FTS trigger must survive (shares the shadow naming)")
	assert.True(t, names["contacts_fts_au"], "FTS trigger must survive")
	assert.True(t, names["idx_contacts_user"], "index must survive")
}

// TestDumpHeaderIsDeterministic guards against a nondeterministic dump (e.g. a
// timestamp creeping into the header), which would make the reproducibility
// test vacuous.
func TestDumpHeaderIsDeterministic(t *testing.T) {
	a, err := GenerateDump(FloorVersion, "v0.6.0")
	require.NoError(t, err)
	b, err := GenerateDump(FloorVersion, "v0.6.0")
	require.NoError(t, err)
	assert.Equal(t, a, b, "regenerating the same dump must be byte-identical")
}

// TestGenerateDumpRefusesUnknownVersion ensures the generator fails loudly for
// a version the chain cannot reach, so a typo'd registry entry surfaces at
// generation time rather than producing a truncated dump.
func TestGenerateDumpRefusesUnknownVersion(t *testing.T) {
	_, err := GenerateDump(9999, "v9.9.9")
	require.Error(t, err)
}
