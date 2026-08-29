package schemafixture

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiteralCoversEverySQLValueType pins the value-serialization switch used
// to build the data-copy INSERTs. Every database/sql value class must render
// to a valid SQL literal — NULL, INTEGER, REAL, bool (defensive; SQLite has no
// bool), BLOB, TEXT, and anything unexpected (time.Time is the realistic
// "default" case a driver can return).
func TestLiteralCoversEverySQLValueType(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "NULL"},
		{int64(42), "42"},
		{int64(-7), "-7"},
		{float64(3.5), "3.5"},
		{true, "1"},
		{false, "0"},
		{[]byte{0x41, 0x00, 0xFF}, "X'4100ff'"},
		{[]byte{}, "X''"},
		{"it's", "'it''s'"},
		{"plain", "'plain'"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, literal(c.in), "literal(%v)", c.in)
	}

	// The default case (anything that is not one of the above) stringifies;
	// time.Time is the realistic driver-returned type to land there.
	when := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "'2026-08-04 12:00:00 +0000 UTC'", literal(when))
}

// TestQuoteIdentRoundTrips tests the identifier quoting used on internal
// sqlite_master-derived names: embedded double quotes are doubled, and the
// result survives a round trip through SQLite as an identifier.
func TestQuoteIdentRoundTrips(t *testing.T) {
	assert.Equal(t, `"contacts"`, quoteIdent("contacts"))
	assert.Equal(t, `"weird""name"`, quoteIdent(`weird"name`))
	assert.Equal(t, `"a", "b"`, quoteIdents([]string{"a", "b"}))
	assert.Equal(t, `""`, quoteIdents([]string{""}))
}

// TestSortedKeysIsDeterministic pins the stable copy order copyData relies on.
func TestSortedKeysIsDeterministic(t *testing.T) {
	m := map[string]tableData{
		"z": {Table: "z"},
		"a": {Table: "a"},
		"m": {Table: "m"},
	}
	assert.Equal(t, []string{"a", "m", "z"}, sortedKeys(m))
}

// TestDumpFileNamesMatchConvention pins the v0.6.N.sql naming the loader,
// completeness test, and docker-publish gate all depend on.
func TestDumpFileNamesMatchConvention(t *testing.T) {
	assert.Equal(t, "v0.6.0.sql", DumpFile(Release{Tag: "v0.6.0", Version: 31}))
	assert.Equal(t, "v0.6.3.sql", DumpFile(Release{Tag: "v0.6.3", Version: 44}))
}

// TestIsShadowOfVirtual pins the shadow-table prefix predicate: a virtual
// table is not its own shadow, a distinct table sharing the prefix is, and an
// unrelated name is not. The predicate is prefix-based — filterShadowTables
// applies it only to type='table' rows, so an FTS trigger (which shares the
// naming) is dropped from filtering at the caller, not here.
func TestIsShadowOfVirtual(t *testing.T) {
	virtual := map[string]bool{"contacts_fts": true}
	assert.False(t, isShadowOfVirtual("contacts_fts", virtual))
	assert.True(t, isShadowOfVirtual("contacts_fts_data", virtual))
	assert.True(t, isShadowOfVirtual("contacts_fts_config", virtual))
	assert.False(t, isShadowOfVirtual("contacts", virtual))
	assert.False(t, isShadowOfVirtual("users", virtual))
}

// TestReadDumpMissingRelease is a direct unit test of readDump's error path:
// a release with no committed dump fails loudly rather than returning empty
// bytes (a fixture silently built from nothing would be a worse failure).
func TestReadDumpMissingRelease(t *testing.T) {
	_, err := readDump(Release{Tag: "v0.6.999", Version: 999})
	require.Error(t, err)
}

// TestFindRepoRootFromNonRepoDir exercises the walk-up failure path: from a
// directory that is not under the repository, findRepoRoot must error rather
// than loop forever.
func TestFindRepoRootFromNonRepoDir(t *testing.T) {
	empty := t.TempDir()
	t.Chdir(empty)
	_, err := findRepoRoot()
	require.Error(t, err)
}

// TestFindSchemaDirFromNonRepoDir mirrors the above for findSchemaDir.
func TestFindSchemaDirFromNonRepoDir(t *testing.T) {
	empty := t.TempDir()
	t.Chdir(empty)
	_, err := findSchemaDir()
	require.Error(t, err)
}
