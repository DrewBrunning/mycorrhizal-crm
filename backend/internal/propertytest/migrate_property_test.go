// TEST-07 (issue #435) / MIG-03 (#438) property: migrate(migrate(db)) is
// non-destructive — re-running the migration chain over a database that
// already has content preserves that content exactly.
//
// Per check it builds a fresh migrated database (contactgen.MigratedDB,
// which is itself produced by database.InitDB), populates generated contacts
// through the same ApplyRecordToContact path the API uses, snapshots every
// table's rows, runs MigrateUp over the file, reopens it, and asserts the
// fingerprint is unchanged. A future migration that is not idempotent on its
// own re-run (a backfill without a guard, a trigger re-created with a
// different shape) fails here with a shrunk counterexample before it ever
// runs against a production database.
// Package propertytest holds the TEST-07 (issue #435) generative properties
// that cannot live in their subject packages. The migration property is one:
// the database package's own test files cannot import internal/contactgen
// (contactgen's DB helpers import database, so an in-package test would form
// an import cycle). A test-only package is the standard escape hatch, and it
// is discovered by `go test ./...` exactly like any other package.
//
// These properties create a fresh migrated database per check, so they are
// the expensive end of the TEST-07 suite; their iteration budget (RAPID_CHECKS,
// tiered 200 PR / 1000 push / 8000 nightly in unit-tests.yml) is what bounds
// the whole nightly run's wall-clock.
package propertytest

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/contactgen"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// TestMigrateIsNonDestructive is the load-bearing migration property.
func TestMigrateIsNonDestructive(t *testing.T) {
	t.Parallel()
	t.Run("rerun", rapid.MakeCheck(func(t *rapid.T) {
		db, path, err := contactgen.MigratedDB(t)
		require.NoError(t, err)

		user, err := contactgen.NewUser(db, "migrate-prop")
		require.NoError(t, err)
		recs := contactgen.Records(t, drawInt(t, "n", 0, 5))
		_, err = contactgen.Populate(db, user.ID, recs)
		require.NoError(t, err)

		before, err := contentFingerprint(db)
		require.NoError(t, err)

		// Close the application connection, then re-run migrations against the
		// file the way a deploy would against a stopped database.
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())

		require.NoError(t, database.MigrateUp(path), "re-running the migration chain must succeed")

		reopened, err := database.OpenMigratedFile(path)
		require.NoError(t, err)
		defer func() {
			if sqlDB, err := reopened.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()

		after, err := contentFingerprint(reopened)
		require.NoError(t, err)
		require.Equal(t, before, after, "re-running migrations must not change any table's content")
	}))
}

// contentFingerprint renders every user table's rows (rowid-ordered) into a
// stable hash, so two databases with identical content hash identically.
// Table names come from sqlite_master, not user input.
func contentFingerprint(db *gorm.DB) (string, error) {
	var tables []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name").Scan(&tables).Error; err != nil {
		return "", fmt.Errorf("fingerprint: listing tables: %w", err)
	}

	h := sha256.New()
	for _, table := range tables {
		if isFTSShadowTable(table) {
			// FTS5's internal bookkeeping tables (X_config/_data/_idx/...) have
			// no rowid and no stable representation; the main FTS table is
			// fingerprinted and reflects the same indexed content.
			continue
		}
		fmt.Fprintf(h, "== %s ==\n", table)
		rows, err := db.Raw("SELECT * FROM \"" + table + "\" ORDER BY rowid").Rows()
		if err != nil {
			return "", fmt.Errorf("fingerprint: reading %s: %w", table, err)
		}
		cols, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("fingerprint: columns of %s: %w", table, err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return "", fmt.Errorf("fingerprint: scanning %s: %w", table, err)
			}
			fmt.Fprintln(h, fmt.Sprint(vals...))
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("fingerprint: iterating %s: %w", table, err)
		}
		if err := rows.Close(); err != nil {
			return "", fmt.Errorf("fingerprint: closing %s: %w", table, err)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func drawInt(t *rapid.T, label string, min, max int) int {
	return rapid.IntRange(min, max).Draw(t, label)
}

func drawBool(t *rapid.T, label string) bool {
	return rapid.Bool().Draw(t, label)
}

// isFTSShadowTable reports whether a table name is an FTS5 shadow table
// (X_config, X_data, X_idx, X_content, X_docsize), which has no rowid and is
// SQLite internal bookkeeping rather than application data.
func isFTSShadowTable(name string) bool {
	for _, suffix := range []string{"_config", "_data", "_idx", "_content", "_docsize"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
