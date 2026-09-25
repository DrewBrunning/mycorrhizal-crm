package dbtest_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// schemaDerivingCalls are GORM calls that build tables from struct tags
// instead of the hand-written migration SQL. A test DB built that way agrees
// with the tags by construction, so it cannot see a tag-vs-migration mismatch
// (CLAUDE.md backend trap #1: ContactSyncLink.ETag -> e_tag shipped broken;
// OccasionObligation.Active default:true, PR #1240). Production does not use
// them either (schema is migrations only). Spelled as concatenations so this
// file does not match itself.
var schemaDerivingCalls = []string{
	"." + "AutoMigrate(",
	"Migrator()." + "CreateTable(",
}

// autoMigrateAllowed lists backend files (slash paths relative to backend/)
// that may still make one of those calls, each with a written reason. Keep it
// empty: use dbtest.New(t) for a schema, dbtest.HideTable for a "query against
// a missing table" error path, and a bare gorm.Open(":memory:") only when a
// test needs no tables at all.
var autoMigrateAllowed = map[string]string{}

// TestNoSchemaDerivingCallsOutsideAllowlist is the ratchet that keeps the
// backend on the real migrated schema.
func TestNoSchemaDerivingCallsOutsideAllowlist(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(root, "go.mod"))
	require.NoError(t, err, "expected the backend module root at %s", root)

	found := map[string]bool{}
	var offenders []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "node_modules" || name == "testdata" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- walking this module's own source tree.
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(src), "\n") {
			for _, call := range schemaDerivingCalls {
				if strings.Contains(line, call) {
					found[rel] = true
					if _, ok := autoMigrateAllowed[rel]; !ok {
						offenders = append(offenders, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
					}
				}
			}
		}
		return nil
	})
	require.NoError(t, err)

	sort.Strings(offenders)
	require.Empty(t, offenders,
		"schema-deriving GORM calls build tables from struct tags, not the migrations (CLAUDE.md backend trap #1). "+
			"Use dbtest.New(t) (plus dbtest.HideTable for missing-table error paths), or add the file to "+
			"autoMigrateAllowed in internal/dbtest/automigrate_ratchet_test.go with a reason.")

	for path := range autoMigrateAllowed {
		require.True(t, found[path], "autoMigrateAllowed entry %q no longer makes a schema-deriving call; remove it", path)
	}
}
