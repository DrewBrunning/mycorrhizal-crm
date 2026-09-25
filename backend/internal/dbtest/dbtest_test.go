package dbtest_test

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New yields a handle on the real hand-written migrated schema, not an
// AutoMigrate derivation (CLAUDE.md backend trap #1). The canonical tells:
// ContactSyncLink's column is `etag`, not GORM's derived `e_tag`, and
// HouseholdMember's is `member_vcard_uid`, not `member_v_card_uid`.
func TestNew_YieldsRealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	require.NoError(t, db.Exec("SELECT etag FROM contact_sync_links WHERE 1=0").Error)
	require.NoError(t, db.Exec("SELECT member_vcard_uid FROM household_members WHERE 1=0").Error)

	// schema_migrations exists and is at the latest embedded version.
	var version uint
	require.NoError(t, db.Raw("SELECT version FROM schema_migrations LIMIT 1").Scan(&version).Error)
	assert.NotZero(t, version)
}

// Each New is a separate file copy: a write in one is invisible to another.
func TestNew_IsolatedPerCall(t *testing.T) {
	a := dbtest.New(t)
	b := dbtest.New(t)

	require.NoError(t, a.Create(&models.User{Username: "iso-a", Email: "iso-a@example.com", Password: "x"}).Error)

	var countB int64
	require.NoError(t, b.Model(&models.User{}).Count(&countB).Error)
	assert.Equal(t, int64(0), countB, "second handle must not see the first handle's write")
}

// NewAt writes the database where the caller asked, for path-on-disk tests.
func TestNewAt_MaterialisesFileAtPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "explicit.db")
	db := dbtest.NewAt(t, path)

	_, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.User{Username: "at", Email: "at@example.com", Password: "x"}).Error)
}

// HideTable renames the table out from under its own name, so a query
// against it fails with "no such table" -- this package's own error-path
// seam, used by controllers/services tests but (per CLAUDE.md's per-package
// coverage model) not otherwise exercised from within this package itself.
func TestHideTable_QueryAgainstHiddenTableFails(t *testing.T) {
	db := dbtest.New(t)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Count(&count).Error, "sanity: the table is queryable before hiding it")

	dbtest.HideTable(t, db, "users")

	err := db.Model(&models.User{}).Count(&count).Error
	require.Error(t, err, "the table must be unreachable under its own name after HideTable")
	assert.Contains(t, err.Error(), "no such table")
}
